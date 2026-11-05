package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed admin/index.html
var adminHTML []byte

var adminKey string

// The admin panel listens on 127.0.0.1 only. Requests must carry the per-install key
// (blocks other websites from driving it via CSRF) and a localhost Host header
// (blocks DNS rebinding).
func startAdmin() error {
	keyFile := filepath.Join(dataDir, "adminkey")
	if b, err := os.ReadFile(keyFile); err == nil && len(b) >= 32 {
		adminKey = strings.TrimSpace(string(b))
	} else {
		adminKey = randomHex(24)
		os.WriteFile(keyFile, []byte(adminKey), 0o600)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(adminHTML)
	})
	api := func(path string, fn func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			k := r.Header.Get("X-KD-Key")
			if k == "" {
				k = r.URL.Query().Get("k")
			}
			if subtle.ConstantTimeCompare([]byte(k), []byte(adminKey)) != 1 {
				http.Error(w, "forbidden", 403)
				return
			}
			fn(w, r)
		})
	}
	api("/admin/state", adminState)
	api("/admin/settings", adminSettings)
	api("/admin/pin", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Pin string }
		json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req)
		pin := strings.TrimSpace(req.Pin)
		if pin == "" {
			pin = randomPin(6)
		}
		if err := setPin(pin); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "pin": pin})
	})
	api("/admin/device/remove", func(w http.ResponseWriter, r *http.Request) {
		removeDevice(r.URL.Query().Get("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/client/kick", func(w http.ResponseWriter, r *http.Request) {
		kickClient(r.URL.Query().Get("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/kickall", func(w http.ResponseWriter, r *http.Request) {
		kickAll()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/qr.png", func(w http.ResponseWriter, r *http.Request) {
		png, err := qrcode.Encode(r.URL.Query().Get("u"), qrcode.Medium, 320)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})
	api("/admin/autostart", func(w http.ResponseWriter, r *http.Request) {
		on := r.URL.Query().Get("on") == "1"
		if err := setAutostart(on); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "autostart": autostartEnabled()})
	})
	api("/admin/firewall", func(w http.ResponseWriter, r *http.Request) {
		err := addFirewallRule(r.URL.Query().Get("public") == "1")
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/reprobe", func(w http.ResponseWriter, r *http.Request) {
		resetEncoderProbe()
		video.mu.Lock()
		video.monitors = listMonitors()
		video.mu.Unlock()
		video.requestKeyframe()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/service", func(w http.ResponseWriter, r *http.Request) {
		on := r.URL.Query().Get("on") == "1"
		if on {
			if agentMode {
				writeJSON(w, 200, map[string]any{"ok": true})
				return
			}
			if err := installService(); err != nil {
				writeJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "restarting": true})
			// hand over: this user-mode copy exits, the service starts the SYSTEM agent
			go func() {
				time.Sleep(400 * time.Millisecond)
				kickAll()
				startService()
				os.Exit(0)
			}()
			return
		}
		// turn off: reply first (in agent mode stopping the service ends this process)
		writeJSON(w, 200, map[string]any{"ok": true, "restarting": true})
		go func() {
			time.Sleep(400 * time.Millisecond)
			kickAll()
			stopAndDeleteService(true)
			os.Exit(0)
		}()
	})
	api("/admin/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(logRing.String()))
	})
	api("/admin/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); quit() }()
	})

	addr := fmt.Sprintf("127.0.0.1:%d", getConfig().AdminPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: hostGuard(mux), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return nil
}

func hostGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", 403)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func adminURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/#k=%s", getConfig().AdminPort, adminKey)
}

func adminState(w http.ResponseWriter, r *http.Request) {
	c := getConfig()
	video.mu.Lock()
	enc, fps, kbps, mons := video.encoder, video.statsFPS, video.statsKbps, video.monitors
	lastErr := video.lastErr
	video.mu.Unlock()
	var urls []string
	for _, ip := range localIPs() {
		urls = append(urls, fmt.Sprintf("https://%s:%d/", ip, c.Port))
	}
	devs := make([]map[string]any, 0, len(c.Devices))
	for _, d := range c.Devices {
		devs = append(devs, map[string]any{"id": d.ID, "name": d.Name, "created": d.Created, "lastSeen": d.LastSeen, "lastIP": d.LastIP})
	}
	writeJSON(w, 200, map[string]any{
		"name": c.Name, "port": c.Port, "pin": c.PinPlain, "fp": prettyFingerprint(), "fpFull": certFingerprint,
		"urls": urls, "clients": clientList(), "devices": devs, "monitors": mons,
		"encoder": enc, "fps": fps, "kbps": kbps, "lastError": lastErr, "ffmpeg": ffmpegPath,
		"admin": isAdmin(), "autostart": autostartEnabled(), "service": agentMode, "serviceInstalled": serviceInstalled(), "version": version, "desktop": inputDesktopName(),
		"settings": map[string]any{
			"requireApproval": c.RequireApproval, "allowControl": c.AllowControl, "allowFiles": c.AllowFiles,
			"allowAudio": c.AllowAudio, "allowClipboard": c.AllowClipboard, "allowPower": c.AllowPower,
			"defaultFps": c.DefaultFPS, "defaultBitrate": c.DefaultBitrate, "encoder": c.Encoder,
			"uploadDir": uploadDir(), "name": c.Name, "paused": c.Paused, "port": c.Port,
		},
	})
}

func adminSettings(w http.ResponseWriter, r *http.Request) {
	var s map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&s); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	b := func(k string, dst *bool) {
		if v, ok := s[k].(bool); ok {
			*dst = v
		}
	}
	n := func(k string, dst *int, lo, hi int) {
		if v, ok := s[k].(float64); ok {
			*dst = clamp(int(v), lo, hi)
		}
	}
	str := func(k string, dst *string) {
		if v, ok := s[k].(string); ok && strings.TrimSpace(v) != "" {
			*dst = strings.TrimSpace(v)
		}
	}
	cfgMu.Lock()
	oldEnc := cfg.Encoder
	b("requireApproval", &cfg.RequireApproval)
	b("allowControl", &cfg.AllowControl)
	b("allowFiles", &cfg.AllowFiles)
	b("allowAudio", &cfg.AllowAudio)
	b("allowClipboard", &cfg.AllowClipboard)
	b("allowPower", &cfg.AllowPower)
	b("paused", &cfg.Paused)
	n("defaultFps", &cfg.DefaultFPS, 5, 144)
	n("defaultBitrate", &cfg.DefaultBitrate, 300, 150000)
	n("port", &cfg.Port, 1024, 65535)
	str("encoder", &cfg.Encoder)
	str("uploadDir", &cfg.UploadDir)
	str("name", &cfg.Name)
	paused := cfg.Paused
	newEnc := cfg.Encoder
	saveConfigLocked()
	cfgMu.Unlock()
	if paused {
		kickAll()
	}
	if newEnc != oldEnc {
		resetEncoderProbe()
		video.requestKeyframe()
	}
	updateTray()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ------------------------------------------------------------ autostart (Task Scheduler, elevated, no UAC prompt)

const taskName = "KadduDesk Server"

func autostartEnabled() bool {
	return hiddenCmd("schtasks.exe", "/Query", "/TN", taskName).Run() == nil
}

func setAutostart(on bool) error {
	if !on {
		out, err := hiddenCmd("schtasks.exe", "/Delete", "/F", "/TN", taskName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	exe, _ := os.Executable()
	xml := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Starts KadduDesk remote desktop server at logon</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><Delay>PT5S</Delay></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + xmlEscape(exe) + `</Command><Arguments>-minimized</Arguments></Exec></Actions>
</Task>`
	tmp := filepath.Join(os.TempDir(), "kaddudesk-task.xml")
	// schtasks wants UTF-16 for the declared encoding
	u := []byte{0xFF, 0xFE}
	for _, r := range xml {
		u = append(u, byte(r), byte(r>>8))
	}
	if err := os.WriteFile(tmp, u, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := hiddenCmd("schtasks.exe", "/Create", "/F", "/TN", taskName, "/XML", tmp).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// ------------------------------------------------------------ firewall

const fwRule = "KadduDesk Server"

func addFirewallRule(public bool) error {
	exe, _ := os.Executable()
	return addFirewallRuleFor(exe, public)
}

func addFirewallRuleFor(exe string, public bool) error {
	profile := "private,domain"
	if public {
		profile = "any"
	}
	hiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name="+fwRule).Run()
	out, err := hiddenCmd("netsh", "advfirewall", "firewall", "add", "rule", "name="+fwRule, "dir=in",
		"action=allow", "program="+exe, "enable=yes", "profile="+profile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	logf("firewall rule added (profile=%s)", profile)
	return nil
}

// ------------------------------------------------------------ log ring buffer

type ring struct {
	mu    sync.Mutex
	lines []string
	f     *os.File
}

var logRing = &ring{}

func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, strings.TrimRight(string(p), "\n"))
	if len(r.lines) > 400 {
		r.lines = r.lines[len(r.lines)-400:]
	}
	if r.f != nil {
		r.f.Write(p)
	}
	return len(p), nil
}

func (r *ring) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}
