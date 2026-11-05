package main

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	id       string
	name     string
	deviceID string
	ip       string
	since    time.Time
	conn     *websocket.Conn
	out      chan []byte // JSON control messages
	closed   chan struct{}
	once     sync.Once
	cfg      VideoCfg
	wantAud  atomic.Bool
	audCh    chan []byte
	audMu    sync.Mutex
	vsub     *videoSub
	keysDown map[string]bool
	viewOnly bool
	btnDown  map[int]bool
	sentKB   atomic.Int64
}

var (
	clientsMu sync.Mutex
	clients   = map[*Client]struct{}{}
)

func clientList() []map[string]any {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	var out []map[string]any
	for c := range clients {
		out = append(out, map[string]any{"id": c.id, "name": c.name, "ip": c.ip, "since": c.since, "device": c.deviceID})
	}
	return out
}

func clientCount() int {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	return len(clients)
}

func kickClient(id string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for c := range clients {
		if c.id == id {
			c.close()
		}
	}
}

func kickDevice(devID string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for c := range clients {
		if c.deviceID == devID {
			c.close()
		}
	}
}

func kickAll() {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for c := range clients {
		c.close()
	}
}

func broadcastJSON(v any, filter func(*Client) bool) {
	b, _ := json.Marshal(v)
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for c := range clients {
		if filter == nil || filter(c) {
			c.send(b)
		}
	}
}

func (c *Client) send(b []byte) {
	select {
	case c.out <- b:
	default:
	}
}

func (c *Client) sendJSON(v any) {
	b, _ := json.Marshal(v)
	c.send(b)
}

func (c *Client) close() {
	c.once.Do(func() {
		close(c.closed)
		c.conn.Close()
	})
}

// ------------------------------------------------------------ input thread

// All input runs on one locked OS thread so SetThreadDesktop (desktop following) sticks.
var inputCh = make(chan func(), 512)

func inputLoop() {
	runtime.LockOSThread()
	syncInputDesktop()
	for fn := range inputCh {
		fn()
	}
}

func doInput(fn func()) {
	select {
	case inputCh <- fn:
	default:
	}
}

// ------------------------------------------------------------ session

type inMsg struct {
	T   string    `json:"t"`
	X   float64   `json:"x"`
	Y   float64   `json:"y"`
	DX  float64   `json:"dx"`
	DY  float64   `json:"dy"`
	B   int       `json:"b"`
	D   bool      `json:"d"`
	C   string    `json:"c"`
	S   string    `json:"s"`
	A   string    `json:"a"`
	K   []string  `json:"k"`
	On  bool      `json:"on"`
	TS  float64   `json:"ts"`
	Cfg *VideoCfg `json:"cfg"`
}

func runSession(conn *websocket.Conn, name, deviceID, ip string, viewOnly bool) {
	c := &Client{
		id: randomHex(4), name: name, deviceID: deviceID, ip: ip, since: time.Now(),
		conn: conn, out: make(chan []byte, 256), closed: make(chan struct{}),
		keysDown: map[string]bool{}, btnDown: map[int]bool{}, viewOnly: viewOnly,
	}
	conf := getConfig()
	c.cfg = VideoCfg{Monitor: 0, FPS: conf.DefaultFPS, Bitrate: conf.DefaultBitrate}
	clientsMu.Lock()
	clients[c] = struct{}{}
	clientsMu.Unlock()
	logf("client connected: %s (%s)", name, ip)
	updateTray()

	defer func() {
		c.close()
		clientsMu.Lock()
		delete(clients, c)
		clientsMu.Unlock()
		video.unsubscribe(c.vsub)
		c.setAudio(false)
		c.releaseAll()
		logf("client disconnected: %s (%s)", name, ip)
		updateTray()
	}()

	video.mu.Lock()
	mons := video.monitors
	video.mu.Unlock()
	c.sendJSON(map[string]any{
		"t": "hello", "name": conf.Name, "version": version, "monitors": mons, "cfg": c.cfg,
		"features": map[string]bool{
			"control": conf.AllowControl && !viewOnly, "files": conf.AllowFiles && !viewOnly, "audio": conf.AllowAudio,
			"clipboard": conf.AllowClipboard && !viewOnly, "power": conf.AllowPower && !viewOnly,
		},
		"macs": macAddrs(), "admin": isAdmin(), "fp": certFingerprint, "you": c.id,
		"service": agentMode, "desk": inputDesktopName(),
	})
	c.vsub = video.subscribe()
	if !video.configure(c.cfg) {
		video.requestKeyframe()
	}
	if conf.AllowClipboard {
		if s, err := getClipboardText(); err == nil && s != "" && len(s) < 1<<20 {
			c.sendJSON(map[string]any{"t": "clip", "s": s})
		}
	}

	go c.writer()
	conn.SetReadLimit(4 << 20)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m inMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		c.handle(&m)
	}
}

func (c *Client) writer() {
	ping := time.NewTicker(1 * time.Second)
	defer ping.Stop()
	waitKey := false
	write := func(mt int, b []byte) bool {
		c.conn.SetWriteDeadline(time.Now().Add(6 * time.Second))
		if err := c.conn.WriteMessage(mt, b); err != nil {
			c.close()
			return false
		}
		c.sentKB.Add(int64(len(b)))
		return true
	}
	hdr := make([]byte, 14)
	for {
		// control + audio first: they are tiny and latency-critical
		select {
		case b := <-c.out:
			if !write(websocket.TextMessage, b) {
				return
			}
			continue
		default:
		}
		aud := c.audioChan()
		select {
		case <-c.closed:
			return
		case b := <-c.out:
			if !write(websocket.TextMessage, b) {
				return
			}
		case p := <-aud:
			msg := make([]byte, 9+len(p))
			msg[0] = 2
			binary.BigEndian.PutUint64(msg[1:], uint64(time.Now().UnixMicro()))
			copy(msg[9:], p)
			if !write(websocket.BinaryMessage, msg) {
				return
			}
		case f := <-c.vsub.ch:
			if c.vsub.dropped.Swap(false) {
				// we fell behind: flush queue and resync at a fresh keyframe
				for len(c.vsub.ch) > 0 {
					<-c.vsub.ch
				}
				waitKey = true
				video.requestKeyframe()
				c.sendJSON(map[string]any{"t": "congestion"})
				continue
			}
			if waitKey {
				if !f.Key {
					continue
				}
				waitKey = false
			}
			hdr[0] = 1
			hdr[1] = 0
			if f.Key {
				hdr[1] = 1
			}
			binary.BigEndian.PutUint64(hdr[2:], uint64(f.TS))
			binary.BigEndian.PutUint32(hdr[10:], f.Seq)
			c.conn.SetWriteDeadline(time.Now().Add(6 * time.Second))
			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				c.close()
				return
			}
			w.Write(hdr)
			w.Write(f.Data)
			if err := w.Close(); err != nil {
				c.close()
				return
			}
			c.sentKB.Add(int64(len(f.Data)))
		case <-ping.C:
			video.mu.Lock()
			w, h := video.outputSize()
			st := map[string]any{"t": "st", "fps": video.statsFPS, "kbps": math.Round(video.statsKbps),
				"enc": video.encoder, "zc": video.zeroCopy, "w": w, "h": h, "viewers": clientCount(),
				"desk": inputDesktopName()}
			video.mu.Unlock()
			b, _ := json.Marshal(st)
			if !write(websocket.TextMessage, b) {
				return
			}
		}
	}
}

func (c *Client) audioChan() chan []byte {
	c.audMu.Lock()
	defer c.audMu.Unlock()
	return c.audCh // nil channel blocks forever = disabled
}

func (c *Client) setAudio(on bool) {
	c.audMu.Lock()
	defer c.audMu.Unlock()
	if on && c.audCh == nil {
		c.audCh = audio.subscribe()
	} else if !on && c.audCh != nil {
		audio.unsubscribe(c.audCh)
		c.audCh = nil
	}
}

func (c *Client) monitorRect() Monitor {
	video.mu.Lock()
	defer video.mu.Unlock()
	return video.monitor()
}

func (c *Client) handle(m *inMsg) {
	conf := getConfig()
	ctl := conf.AllowControl && !c.viewOnly
	switch m.T {
	case "ping":
		c.sendJSON(map[string]any{"t": "pong", "ts": m.TS})
	case "cfg":
		if m.Cfg != nil {
			c.cfg = *m.Cfg
			video.configure(c.cfg)
			video.mu.Lock()
			cur := video.cfg
			video.mu.Unlock()
			broadcastJSON(map[string]any{"t": "cfg", "cfg": cur}, nil)
		}
	case "kf":
		video.requestKeyframe()
	case "audio":
		c.setAudio(m.On && conf.AllowAudio)
	case "mm": // absolute move, x/y normalised 0..1 within the streamed monitor
		if !ctl {
			return
		}
		mon := c.monitorRect()
		x := mon.X + int32(math.Round(clampF(m.X)*float64(mon.W-1)))
		y := mon.Y + int32(math.Round(clampF(m.Y)*float64(mon.H-1)))
		doInput(func() { inputMoveAbs(x, y) })
	case "mr": // relative move (pointer lock / games)
		if !ctl {
			return
		}
		dx, dy := int32(m.DX), int32(m.DY)
		doInput(func() { inputMoveRel(dx, dy) })
	case "mb":
		if !ctl || m.B < 0 || m.B > 4 {
			return
		}
		c.btnDown[m.B] = m.D
		b, d := m.B, m.D
		doInput(func() { inputButton(b, d) })
	case "wh":
		if !ctl {
			return
		}
		dx, dy := int32(m.DX), int32(m.DY)
		doInput(func() { inputWheel(dx, dy) })
	case "k":
		if !ctl {
			return
		}
		if m.D {
			c.keysDown[m.C] = true
		} else {
			delete(c.keysDown, m.C)
		}
		code, d := m.C, m.D
		doInput(func() { keyByCode(code, d) })
	case "txt":
		if !ctl || len(m.S) > 10000 {
			return
		}
		s := m.S
		doInput(func() { typeText(s) })
	case "combo":
		if !ctl || len(m.K) > 6 {
			return
		}
		keys := m.K
		if len(keys) == 3 && keys[0] == "ControlLeft" && keys[1] == "AltLeft" && keys[2] == "Delete" {
			if sendSAS() {
				return
			}
			c.sendJSON(map[string]any{"t": "notice", "s": "Real Ctrl+Alt+Del needs Service mode (control panel, Server section). Opened Task Manager instead."})
			powerAction("taskmgr")
			return
		}
		doInput(func() {
			for _, k := range keys {
				keyByCode(k, true)
			}
			for i := len(keys) - 1; i >= 0; i-- {
				keyByCode(keys[i], false)
			}
		})
	case "clip":
		if !conf.AllowClipboard || c.viewOnly || len(m.S) > 1<<20 {
			return
		}
		if err := setClipboardText(m.S); err == nil {
			lastClipSeq.Store(clipboardSeq())
			broadcastJSON(map[string]any{"t": "clip", "s": m.S}, func(o *Client) bool { return o != c })
		}
	case "power":
		if !conf.AllowPower || c.viewOnly {
			c.sendJSON(map[string]any{"t": "notice", "s": "Power actions are disabled on this PC"})
			return
		}
		logf("power action %q from %s", m.A, c.name)
		if err := powerAction(m.A); err != nil {
			c.sendJSON(map[string]any{"t": "notice", "s": err.Error()})
		}
	case "release":
		c.releaseAll()
	}
}

// releaseAll lifts every key/button this client holds, so nothing stays stuck on disconnect.
func (c *Client) releaseAll() {
	keys := c.keysDown
	btns := c.btnDown
	c.keysDown, c.btnDown = map[string]bool{}, map[int]bool{}
	doInput(func() {
		for k := range keys {
			keyByCode(k, false)
		}
		for b, d := range btns {
			if d {
				inputButton(b, false)
			}
		}
	})
}

func clampF(v float64) float64 {
	if v < 0 || math.IsNaN(v) {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ------------------------------------------------------------ cursor + clipboard watchers

var lastClipSeq atomic.Uint32

var cursorDeskChanged atomic.Bool

// desktopLoop notices when Windows switches to the lock screen / UAC / sign-in desktop
// and back. In service mode capture, input and cursor follow it.
func desktopLoop() {
	last := inputDesktopName()
	video.setDesktop(last)
	t := time.NewTicker(250 * time.Millisecond)
	for range t.C {
		d := inputDesktopName()
		if d == last {
			continue
		}
		logf("input desktop: %s -> %s", last, d)
		last = d
		video.setDesktop(d)
		doInput(func() { syncInputDesktop() })
		cursorDeskChanged.Store(true)
		broadcastJSON(map[string]any{"t": "desk", "s": d, "service": agentMode}, nil)
	}
}

func cursorLoop() {
	runtime.LockOSThread() // so SetThreadDesktop sticks for GetCursorInfo
	syncInputDesktop()
	t := time.NewTicker(8 * time.Millisecond) // 125 Hz
	lastShape := ""
	var lx, ly int32 = -1, -1
	idle := 0
	for range t.C {
		if cursorDeskChanged.Swap(false) {
			syncInputDesktop()
		}
		if clientCount() == 0 {
			lastShape, lx, ly = "", -1, -1
			continue
		}
		shape, x, y, _ := getCursor()
		if shape != lastShape {
			lastShape = shape
			broadcastJSON(map[string]any{"t": "cur", "s": shape}, nil)
		}
		if x != lx || y != ly || idle > 60 {
			lx, ly, idle = x, y, 0
			video.mu.Lock()
			mon := video.monitor()
			video.mu.Unlock()
			if mon.W > 0 && mon.H > 0 {
				broadcastJSON(map[string]any{"t": "cp",
					"x": float64(x-mon.X) / float64(mon.W), "y": float64(y-mon.Y) / float64(mon.H)}, nil)
			}
		} else {
			idle++
		}
	}
}

func clipboardLoop() {
	lastClipSeq.Store(clipboardSeq())
	t := time.NewTicker(400 * time.Millisecond)
	for range t.C {
		if clientCount() == 0 || !getConfig().AllowClipboard {
			continue
		}
		s := clipboardSeq()
		if s == lastClipSeq.Load() {
			continue
		}
		lastClipSeq.Store(s)
		txt, err := getClipboardText()
		if err != nil || txt == "" || len(txt) > 1<<20 {
			continue
		}
		broadcastJSON(map[string]any{"t": "clip", "s": txt}, nil)
	}
}

// ------------------------------------------------------------ misc

type macInfo struct {
	MAC   string `json:"mac"`
	Bcast string `json:"bcast"`
	IP    string `json:"ip"`
}

func macAddrs() []macInfo {
	var out []macInfo
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || len(i.HardwareAddr) != 6 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			ip := n.IP.To4()
			b := make(net.IP, 4)
			for k := 0; k < 4; k++ {
				b[k] = ip[k] | ^n.Mask[len(n.Mask)-4+k]
			}
			out = append(out, macInfo{MAC: i.HardwareAddr.String(), Bcast: b.String(), IP: ip.String()})
		}
	}
	return out
}

// localIPs lists this PC's IPv4 addresses, typical home-LAN ranges first
// (VirtualBox/VPN adapters last), so the panel's first address is the one phones can reach.
func localIPs() []string {
	rank := func(ip string) int {
		switch {
		case strings.HasPrefix(ip, "192.168.56."), strings.HasPrefix(ip, "169.254."):
			return 4 // VirtualBox host-only / link-local
		case strings.HasPrefix(ip, "192.168."):
			return 0
		case strings.HasPrefix(ip, "10."):
			return 1
		case strings.HasPrefix(ip, "172."):
			return 2
		default:
			return 3 // VPNs (Tailscale 100.x, Radmin 26.x ...) – still useful remotely
		}
	}
	var out []string
	for _, m := range macAddrs() {
		out = append(out, m.IP)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

func logf(f string, a ...any) { log.Printf(f, a...) }
