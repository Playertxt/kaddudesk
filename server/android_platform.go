//go:build kdandroid

package main

// Android: the KadduDesk app (Java) captures the screen with MediaProjection, encodes it
// with the phone's hardware H.264 encoder and injects input with an AccessibilityService.
// This core talks to it over a loopback "bridge" socket:
//   app → core: [len u32][type u8][payload]  type 1 = video frame (flags u8 + Annex-B), 3 = JSON
//   core → app: same framing, JSON only (encoder config, touches, keys, text, buttons)

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tailscale.com/net/dnscache"
)

var (
	flagBridge    = flag.Int("bridge", 0, "Android: loopback port for the app's screen/input bridge")
	flagDNS       = flag.String("dns", "", "Android: comma-separated DNS servers")
	flagUploadDir = flag.String("uploaddir", "", "Android: where received files are saved")
	flagIPs       = flag.String("ips", "", "Android: this phone's IPv4 addresses (comma-separated)")
)

var trayIcon []byte

type abridge struct {
	mu      sync.Mutex
	wmu     sync.Mutex
	conn    net.Conn
	w, h    int
	sharing bool
	access  bool
	ips     []string
	pending map[string]chan bool
	gen     atomic.Int64
	noticed time.Time
}

var ab = &abridge{w: 1080, h: 2340, pending: map[string]chan bool{}}

func init() {
	platformVideoRun = androidVideoRun
	servers := []string{"8.8.8.8:53", "1.1.1.1:53"}
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		list := servers
		if *flagDNS != "" {
			list = nil
			for _, s := range strings.Split(*flagDNS, ",") {
				if s = strings.TrimSpace(s); s != "" {
					if !strings.Contains(s, ":") || strings.Count(s, ":") > 1 && !strings.HasPrefix(s, "[") {
						s = net.JoinHostPort(s, "53")
					}
					list = append(list, s)
				}
			}
			list = append(list, servers...)
		}
		var d net.Dialer
		var last error
		for _, s := range list {
			c, err := d.DialContext(ctx, "udp", s)
			if err == nil {
				return c, nil
			}
			last = err
		}
		return nil, last
	}}
	// Tailscale's control client makes its own resolver that reads /etc/resolv.conf,
	// which Android doesn't have, so "Anywhere" could never find Tailscale's server.
	dnscache.Get().Forward = net.DefaultResolver
}

// ------------------------------------------------------------ bridge

func startBridge() {
	if *flagBridge <= 0 {
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *flagBridge))
	if err != nil {
		logf("bridge: %v", err)
		return
	}
	tok := os.Getenv("KD_BRIDGE_TOKEN")
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveBridge(c, tok)
		}
	}()
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:4])
	if n > 16<<20 {
		return 0, nil, errors.New("frame too big")
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return hdr[4], b, err
}

func serveBridge(c net.Conn, tok string) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	t, b, err := readFrame(c)
	var hello struct{ T, Token string }
	if err != nil || t != 3 || json.Unmarshal(b, &hello) != nil || hello.T != "hello" || tok == "" || hello.Token != tok {
		return
	}
	c.SetReadDeadline(time.Time{})
	ab.mu.Lock()
	if ab.conn != nil {
		ab.conn.Close()
	}
	ab.conn = c
	ab.mu.Unlock()
	logf("bridge: app connected")
	if videoHasViewers() {
		video.requestKeyframe() // (re)send encoder settings to the new connection
	}
	for {
		t, b, err := readFrame(c)
		if err != nil {
			break
		}
		switch t {
		case 1:
			if len(b) > 1 {
				video.deliver(b[1:], b[0]&1 == 1)
			}
		case 3:
			handleBridgeJSON(b)
		}
	}
	ab.mu.Lock()
	if ab.conn == c {
		ab.conn = nil
		ab.sharing = false
	}
	ab.mu.Unlock()
	logf("bridge: app disconnected")
}

func handleBridgeJSON(b []byte) {
	var m struct {
		T       string   `json:"t"`
		W       int      `json:"w"`
		H       int      `json:"h"`
		Sharing bool     `json:"sharing"`
		Access  bool     `json:"access"`
		IPs     []string `json:"ips"`
		ID      string   `json:"id"`
		OK      bool     `json:"ok"`
	}
	if json.Unmarshal(b, &m) != nil {
		return
	}
	switch m.T {
	case "screen":
		ab.mu.Lock()
		changed := ab.w != m.W || ab.h != m.H
		if m.W > 0 && m.H > 0 {
			ab.w, ab.h = m.W, m.H
		}
		ab.mu.Unlock()
		if changed {
			video.mu.Lock()
			video.monitors = listMonitors()
			video.mu.Unlock()
			if videoHasViewers() {
				video.requestKeyframe()
			}
			broadcastJSON(map[string]any{"t": "hello-monitors", "monitors": listMonitors()}, nil)
		}
	case "state":
		ab.mu.Lock()
		was := ab.sharing
		ab.sharing, ab.access = m.Sharing, m.Access
		ab.mu.Unlock()
		if m.Sharing && !was && videoHasViewers() {
			video.requestKeyframe()
		}
	case "net":
		ab.mu.Lock()
		ab.ips = m.IPs
		ab.mu.Unlock()
	case "answer":
		ab.mu.Lock()
		ch := ab.pending[m.ID]
		delete(ab.pending, m.ID)
		ab.mu.Unlock()
		if ch != nil {
			ch <- m.OK
		}
	case "noaccess":
		noticeOnce("To control this phone, turn on KadduDesk under Accessibility on the phone (KadduDesk → This device → Remote control).")
	}
}

func noticeOnce(s string) {
	ab.mu.Lock()
	if time.Since(ab.noticed) < 20*time.Second {
		ab.mu.Unlock()
		return
	}
	ab.noticed = time.Now()
	ab.mu.Unlock()
	broadcastJSON(map[string]any{"t": "notice", "s": s}, nil)
}

func bsend(m map[string]any) bool {
	ab.mu.Lock()
	c := ab.conn
	ab.mu.Unlock()
	if c == nil {
		return false
	}
	b, _ := json.Marshal(m)
	var hdr [5]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(b)))
	hdr[4] = 3
	ab.wmu.Lock()
	defer ab.wmu.Unlock()
	c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write(append(hdr[:], b...)); err != nil {
		return false
	}
	return true
}

// ------------------------------------------------------------ video

func androidVideoRun(ctx context.Context, v *videoPipeline) {
	g := ab.gen.Add(1)
	v.mu.Lock()
	w, h := v.outputSize()
	fps, br := v.cfg.FPS, v.cfg.Bitrate
	v.mu.Unlock()
	if fps <= 0 {
		fps = 30
	}
	if fps > 60 {
		fps = 60
	}
	if br <= 0 {
		br = 8000
	}
	ab.mu.Lock()
	sharing := ab.sharing
	ab.mu.Unlock()
	if !bsend(map[string]any{"t": "vcfg", "gen": g, "fps": fps, "kbps": br, "w": w, "h": h}) || !sharing {
		noticeOnce("This phone isn't sharing its screen right now. On the phone, open KadduDesk → This device → Start sharing.")
	}
	<-ctx.Done()
	bsend(map[string]any{"t": "vstop", "gen": g})
}

// ------------------------------------------------------------ monitors / input

func listMonitors() []Monitor {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return []Monitor{{Index: 0, Name: "Phone screen", W: int32(ab.w), H: int32(ab.h), Primary: true, GPU: "MediaCodec"}}
}

var (
	curX, curY int32
	btnDown    = -1
	lastMove   time.Time
	shiftDown  bool
)

func inputInit() error                        { return nil }
func syncInputDesktop() bool                  { return true }
func inputDesktopName() string                { return "Default" }
func initCursors()                            {}
func getCursor() (string, int32, int32, bool) { return "default", curX, curY, true }

func clampXY(x, y int32) (int32, int32) {
	ab.mu.Lock()
	w, h := int32(ab.w), int32(ab.h)
	ab.mu.Unlock()
	return max(0, min(w-1, x)), max(0, min(h-1, y))
}

func inputMoveAbs(x, y int32) {
	curX, curY = clampXY(x, y)
	if btnDown >= 0 && time.Since(lastMove) > 16*time.Millisecond {
		lastMove = time.Now()
		bsend(map[string]any{"t": "move", "x": curX, "y": curY})
	}
}

func inputMoveRel(dx, dy int32) { inputMoveAbs(curX+dx, curY+dy) }

func inputButton(b int, d bool) {
	if d {
		btnDown = b
		bsend(map[string]any{"t": "down", "x": curX, "y": curY, "b": b})
	} else {
		bsend(map[string]any{"t": "up", "x": curX, "y": curY, "b": b})
		btnDown = -1
	}
}

func inputWheel(dx, dy int32) {
	bsend(map[string]any{"t": "scroll", "x": curX, "y": curY, "dx": dx, "dy": dy})
}

var androidKeyGlobal = map[string]string{"Escape": "back", "BrowserBack": "back", "MetaLeft": "home", "MetaRight": "home",
	"ContextMenu": "recents", "AudioVolumeMute": "", "F5": ""}

func keyByCode(code string, down bool) {
	if code == "ShiftLeft" || code == "ShiftRight" {
		shiftDown = down
		return
	}
	if !down {
		return
	}
	if g, ok := androidKeyGlobal[code]; ok {
		if g != "" {
			bsend(map[string]any{"t": "global", "a": g})
		}
		return
	}
	switch code {
	case "Backspace", "Delete", "Enter", "NumpadEnter", "Tab":
		bsend(map[string]any{"t": "key", "k": code})
		return
	}
	if r := codeToChar(code, shiftDown); r != "" {
		bsend(map[string]any{"t": "text", "s": r})
	}
}

// codeToChar turns physical keys from a computer keyboard into text (US layout).
func codeToChar(code string, shift bool) string {
	if strings.HasPrefix(code, "Key") && len(code) == 4 {
		c := code[3:]
		if !shift {
			c = strings.ToLower(c)
		}
		return c
	}
	if strings.HasPrefix(code, "Digit") && len(code) == 6 {
		if shift {
			return string(")!@#$%^&*("[code[5]-'0'])
		}
		return code[5:]
	}
	m := map[string][2]string{"Space": {" ", " "}, "Minus": {"-", "_"}, "Equal": {"=", "+"}, "BracketLeft": {"[", "{"},
		"BracketRight": {"]", "}"}, "Semicolon": {";", ":"}, "Quote": {"'", "\""}, "Comma": {",", "<"}, "Period": {".", ">"},
		"Slash": {"/", "?"}, "Backslash": {"\\", "|"}, "Backquote": {"`", "~"}}
	if v, ok := m[code]; ok {
		if shift {
			return v[1]
		}
		return v[0]
	}
	return ""
}

func typeText(s string) { bsend(map[string]any{"t": "text", "s": s}) }

func powerAction(a string) error {
	switch a {
	case "back", "home", "recents", "notifications", "quicksettings", "lock", "screenshot", "power":
		bsend(map[string]any{"t": "global", "a": a})
		return nil
	case "taskmgr":
		bsend(map[string]any{"t": "global", "a": "recents"})
		return nil
	}
	return errors.New("not available on Android")
}

// ------------------------------------------------------------ clipboard / approval / misc

func getClipboardText() (string, error) { return "", nil }
func setClipboardText(s string) error {
	bsend(map[string]any{"t": "clip", "s": s})
	return nil
}
func clipboardSeq() uint32 { return 0 }

func askApproval(title, msg string, timeoutMs uint32) bool {
	id := randomHex(6)
	ch := make(chan bool, 1)
	ab.mu.Lock()
	ab.pending[id] = ch
	ab.mu.Unlock()
	if !bsend(map[string]any{"t": "ask", "id": id, "title": title, "msg": msg}) {
		return false
	}
	select {
	case ok := <-ch:
		return ok
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return false
	}
}

func isAdmin() bool                                   { return false }
func hiddenCmd(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
func platformName() string                            { return "Android" }
func trayAvailable() bool                             { return false }
func platformEarly() bool                             { return false }
func selectDataDir()                                  { initDataDir() }
func initDataDir() {
	if *flagDataDir != "" {
		initDataDirAt(*flagDataDir)
		return
	}
	initDataDirAt(filepath.Join(os.TempDir(), "kaddudesk"))
}
func initDataDirAt(dir string) { dataDir = dir; os.MkdirAll(dataDir, 0o700) }
func alreadyRunning() bool     { return false }
func acquireHostLock() bool    { return true }
func platformStart()           { startBridge() }
func platformQuit()            {}
func openBrowser(u string)     { bsend(map[string]any{"t": "open", "url": u}) }
func fatal(f string, a ...any) {
	logf("FATAL: "+f, a...)
	fmt.Fprintf(os.Stderr, "KadduDesk: "+f+"\n", a...)
	os.Exit(1)
}
func sendSAS() bool { return false }
func rootEntries() []fileEntry {
	return []fileEntry{{Name: "KadduDesk files", Path: uploadDir(), Dir: true}}
}
func userHome() string { return uploadDir() }
func uploadDir() string {
	if d := getConfig().UploadDir; d != "" {
		return d
	}
	if *flagUploadDir != "" {
		os.MkdirAll(*flagUploadDir, 0o755)
		return *flagUploadDir
	}
	return filepath.Join(dataDir, "received")
}
func installService() error           { return errNotSupported }
func startService() error             { return errNotSupported }
func stopAndDeleteService(bool) error { return nil }
func serviceInstalled() bool          { return false }
func serviceRunning() bool            { return false }
func requestSAS() bool                { return false }
func autostartEnabled() bool          { return false }
func setAutostart(bool) error         { return errNotSupported }
func addFirewallRule(bool) error      { return nil }
func addHubFirewallRule()             {}
func launchHostElevated() error       { return errNotSupported }
func defaultAppPaths() []string       { return nil }
func notify(title, msg string)        { bsend(map[string]any{"t": "notify", "title": title, "msg": msg}) }
func runAsUser(string) error          { return errNotSupported }
func platformIPs() []string {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	if len(ab.ips) == 0 && *flagIPs != "" {
		return strings.Split(*flagIPs, ",")
	}
	return append([]string(nil), ab.ips...)
}
func extraBroadcasts() []string {
	var out []string
	for _, ip := range platformIPs() {
		if p := strings.Split(ip, "."); len(p) == 4 {
			out = append(out, p[0]+"."+p[1]+"."+p[2]+".255")
		}
	}
	return out
}

// ------------------------------------------------------------ media (no ffmpeg on Android)

func findFFmpeg() error { ffmpegPath = "(MediaCodec)"; return nil }
func ffStart(ctx context.Context, args []string, desktop string) (*ffProc, error) {
	return nil, errNotSupported
}

var encoderOrder = []string{"mediacodec"}

func encoderCandidates([]string) []encChoice                                 { return nil }
func fallbackChoice() encChoice                                              { return encChoice{enc: "mediacodec"} }
func (v *videoPipeline) buildArgs(string, bool, bool, string, bool) []string { return nil }

// phone sound capture isn't wired up yet: stay idle instead of retrying
func (a *audioPipeline) runOnce(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func videoHasViewers() bool {
	video.mu.Lock()
	defer video.mu.Unlock()
	return len(video.subs) > 0
}
