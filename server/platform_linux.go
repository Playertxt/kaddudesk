package main

import (
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func isAdmin() bool { return os.Geteuid() == 0 }

func hiddenCmd(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }

func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland")
}

func xDisplay() string {
	if d := os.Getenv("DISPLAY"); d != "" {
		return d
	}
	return ":0"
}

func firstExisting(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// ------------------------------------------------------------ monitors

var monRe = regexp.MustCompile(`(\d+)/\d+x(\d+)/\d+\+(-?\d+)\+(-?\d+)\s+(\S+)`)

// listMonitors uses xrandr on X11, /sys/class/drm otherwise (Wayland / KMS capture).
func listMonitors() []Monitor {
	var out []Monitor
	if !isWayland() || os.Getenv("DISPLAY") != "" {
		if p := firstExisting("xrandr"); p != "" {
			cmd := exec.Command(p, "--listmonitors")
			if b, err := cmd.Output(); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					m := monRe.FindStringSubmatch(l)
					if m == nil {
						continue
					}
					w, _ := strconv.Atoi(m[1])
					h, _ := strconv.Atoi(m[2])
					x, _ := strconv.Atoi(m[3])
					y, _ := strconv.Atoi(m[4])
					out = append(out, Monitor{Index: len(out), Name: m[5], X: int32(x), Y: int32(y), W: int32(w), H: int32(h),
						Primary: strings.Contains(l, "*")})
				}
			}
		}
	}
	if len(out) == 0 { // KMS: first connected connector's preferred mode
		conns, _ := filepath.Glob("/sys/class/drm/card*-*")
		for _, c := range conns {
			st, _ := os.ReadFile(filepath.Join(c, "status"))
			if strings.TrimSpace(string(st)) != "connected" {
				continue
			}
			modes, _ := os.ReadFile(filepath.Join(c, "modes"))
			first := strings.SplitN(strings.TrimSpace(string(modes)), "\n", 2)[0]
			var w, h int
			if _, err := fmtSscanf(first, &w, &h); err == nil && w > 0 {
				out = append(out, Monitor{Index: len(out), Name: filepath.Base(c), W: int32(w), H: int32(h), Primary: len(out) == 0})
				break // kmsgrab captures one plane; expose one screen
			}
		}
	}
	if len(out) == 0 {
		out = []Monitor{{Index: 0, Name: "Screen", W: 1920, H: 1080, Primary: true}}
	}
	vsMu.Lock()
	vsMons = out
	vsMu.Unlock()
	return out
}

func fmtSscanf(s string, w, h *int) (int, error) {
	p := strings.SplitN(s, "x", 2)
	if len(p) != 2 {
		return 0, errors.New("bad mode")
	}
	var err error
	*w, err = strconv.Atoi(p[0])
	if err != nil {
		return 0, err
	}
	*h, err = strconv.Atoi(strings.TrimRightFunc(p[1], func(r rune) bool { return r < '0' || r > '9' }))
	return 2, err
}

var (
	vsMu   sync.Mutex
	vsMons []Monitor
)

// virtualScreen is the bounding box of all monitors (what the absolute pointer spans).
func virtualScreen() (x, y, w, h int32) {
	vsMu.Lock()
	ms := vsMons
	vsMu.Unlock()
	if len(ms) == 0 {
		ms = listMonitors()
	}
	minX, minY, maxX, maxY := ms[0].X, ms[0].Y, ms[0].X+ms[0].W, ms[0].Y+ms[0].H
	for _, m := range ms[1:] {
		minX, minY = min(minX, m.X), min(minY, m.Y)
		maxX, maxY = max(maxX, m.X+m.W), max(maxY, m.Y+m.H)
	}
	return minX, minY, maxX - minX, maxY - minY
}

// ------------------------------------------------------------ clipboard (wl-clipboard / xclip / xsel)

var (
	clipMu   sync.Mutex
	clipLast string
	clipSeq  uint32
	clipAt   time.Time
)

func readClipboard() (string, error) {
	var cmd *exec.Cmd
	switch {
	case isWayland() && firstExisting("wl-paste") != "":
		cmd = exec.Command("wl-paste", "--no-newline", "--type", "text")
	case firstExisting("xclip") != "":
		cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	case firstExisting("xsel") != "":
		cmd = exec.Command("xsel", "--clipboard", "--output")
	default:
		return "", errors.New("install wl-clipboard (Wayland) or xclip (X11) for clipboard sync")
	}
	b, err := cmd.Output()
	if err != nil {
		return "", nil // empty / non-text clipboard
	}
	return string(b), nil
}

func getClipboardText() (string, error) {
	clipMu.Lock()
	defer clipMu.Unlock()
	if time.Since(clipAt) < 300*time.Millisecond {
		return clipLast, nil
	}
	s, err := readClipboard()
	if err == nil {
		clipLast, clipAt = s, time.Now()
	}
	return s, err
}

func setClipboardText(s string) error {
	var cmd *exec.Cmd
	switch {
	case isWayland() && firstExisting("wl-copy") != "":
		cmd = exec.Command("wl-copy")
	case firstExisting("xclip") != "":
		cmd = exec.Command("xclip", "-selection", "clipboard", "-i")
	case firstExisting("xsel") != "":
		cmd = exec.Command("xsel", "--clipboard", "--input")
	default:
		return errors.New("no clipboard tool")
	}
	cmd.Stdin = strings.NewReader(s)
	if err := cmd.Start(); err != nil { // wl-copy/xclip stay alive to serve the selection
		return err
	}
	go cmd.Wait()
	clipMu.Lock()
	clipLast, clipAt = s, time.Now()
	clipMu.Unlock()
	return nil
}

var clipHash [32]byte

// clipboardSeq changes whenever the clipboard text changes (polled by clipboardLoop).
func clipboardSeq() uint32 {
	s, err := readClipboard()
	if err != nil {
		return clipSeq
	}
	clipMu.Lock()
	defer clipMu.Unlock()
	if h := sha256.Sum256([]byte(s)); h != clipHash {
		clipHash = h
		clipSeq++
		clipLast, clipAt = s, time.Now()
	}
	return clipSeq
}

// ------------------------------------------------------------ power / apps

func runFirst(cands ...[]string) error {
	for _, c := range cands {
		if p := firstExisting(c[0]); p != "" {
			return exec.Command(p, c[1:]...).Start()
		}
	}
	return errors.New("no suitable program installed")
}

func powerAction(a string) error {
	home, _ := os.UserHomeDir()
	switch a {
	case "lock":
		return runFirst([]string{"loginctl", "lock-session"}, []string{"xdg-screensaver", "lock"})
	case "sleep":
		return runFirst([]string{"systemctl", "suspend"})
	case "restart":
		return runFirst([]string{"systemctl", "reboot"})
	case "shutdown":
		return runFirst([]string{"systemctl", "poweroff"})
	case "signout":
		if id := os.Getenv("XDG_SESSION_ID"); id != "" {
			return runFirst([]string{"loginctl", "terminate-session", id})
		}
		return runFirst([]string{"gnome-session-quit", "--logout", "--no-prompt"}, []string{"qdbus", "org.kde.ksmserver", "/KSMServer", "logout", "0", "0", "0"})
	case "taskmgr":
		return runFirst([]string{"gnome-system-monitor"}, []string{"plasma-systemmonitor"}, []string{"ksysguard"},
			[]string{"xfce4-taskmanager"}, []string{"mate-system-monitor"}, []string{"x-terminal-emulator", "-e", "top"})
	case "explorer":
		return runFirst([]string{"xdg-open", home})
	case "cmd":
		return runFirst([]string{"x-terminal-emulator"}, []string{"gnome-terminal"}, []string{"konsole"},
			[]string{"xfce4-terminal"}, []string{"kitty"}, []string{"alacritty"}, []string{"xterm"})
	case "settings":
		return runFirst([]string{"gnome-control-center"}, []string{"systemsettings"}, []string{"xfce4-settings-manager"},
			[]string{"cinnamon-settings"}, []string{"mate-control-center"})
	}
	return errors.New("unknown action")
}

// askApproval shows a Yes/No dialog (zenity or kdialog). Without either it refuses.
func askApproval(title, msg string, timeoutMs uint32) bool {
	secs := strconv.Itoa(int(timeoutMs / 1000))
	if p := firstExisting("zenity"); p != "" {
		return exec.Command(p, "--question", "--title", title, "--text", msg, "--timeout", secs).Run() == nil
	}
	if p := firstExisting("kdialog"); p != "" {
		return exec.Command(p, "--title", title, "--yesno", msg).Run() == nil
	}
	logf("approval needed but zenity/kdialog not installed — refusing")
	return false
}
