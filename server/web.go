package main

import (
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed web
var webFS embed.FS

const version = "1.1.0"

func clientIP(r *http.Request) string {
	if r.Context().Value(viaTunnelKey) != nil { // relayed by our hub: it tells us who the client is
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func reqToken(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func authed(r *http.Request) (devID, name string, ok bool) {
	return validateToken(reqToken(r), clientIP(r))
}

// ------------------------------------------------------------ brute-force protection

type failRec struct {
	n     int
	until time.Time
}

var (
	failMu sync.Mutex
	fails  = map[string]*failRec{}
)

func lockedOut(ip string) time.Duration {
	failMu.Lock()
	defer failMu.Unlock()
	if f, ok := fails[ip]; ok && time.Now().Before(f.until) {
		return time.Until(f.until)
	}
	return 0
}

func noteFail(ip string) {
	failMu.Lock()
	defer failMu.Unlock()
	f := fails[ip]
	if f == nil {
		f = &failRec{}
		fails[ip] = f
	}
	f.n++
	if f.n >= 5 {
		d := time.Duration(30<<min(f.n-5, 6)) * time.Second // 30s, 60s, 2m ... up to 32m
		f.until = time.Now().Add(d)
	}
}

func clearFail(ip string) {
	failMu.Lock()
	delete(fails, ip)
	failMu.Unlock()
}

// ------------------------------------------------------------ server

var webMux *http.ServeMux

func startWebServer(cert tls.Certificate) error {
	mux := http.NewServeMux()
	webMux = mux
	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		c := getConfig()
		writeJSON(w, 200, map[string]any{"app": "kaddudesk", "name": c.Name, "version": version,
			"fp": certFingerprint, "paused": c.Paused, "macs": macAddrs(), "id": deviceID()})
	})
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		if _, name, ok := authed(r); ok {
			writeJSON(w, 200, map[string]any{"ok": true, "name": name})
			return
		}
		writeJSON(w, 401, map[string]any{"ok": false})
	})
	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		if id, _, ok := authed(r); ok && id != "" {
			removeDevice(id)
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("/ws", handleWS)
	mux.HandleFunc("/api/files", handleFiles)
	mux.HandleFunc("/api/download", handleDownload)
	mux.HandleFunc("/api/upload", handleUpload)

	c := getConfig()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.Port),
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
		ReadHeaderTimeout: 15 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	go func() {
		if err := srv.ServeTLS(tcpNoDelay{ln}, "", ""); err != nil {
			logf("https server: %v", err)
		}
	}()
	// optional plain-HTTP listener for local testing; loopback only (localhost is a secure context)
	if a := *flagDevHTTP; a != "" {
		if h, _, err := net.SplitHostPort(a); err == nil && (h == "127.0.0.1" || h == "localhost") {
			if dl, err := net.Listen("tcp", a); err == nil {
				logf("dev http on %s", a)
				go http.Serve(tcpNoDelay{dl}, mux)
			}
		}
	}
	return nil
}

// tcpNoDelay makes sure Nagle is off (Go default, but be explicit) and buffers are roomy.
type tcpNoDelay struct{ net.Listener }

func (l tcpNoDelay) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if tc, ok := c.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetWriteBuffer(4 << 20)
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(15 * time.Second)
	}
	return c, err
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "POST only"})
		return
	}
	ip := clientIP(r)
	if d := lockedOut(ip); d > 0 {
		writeJSON(w, 429, map[string]any{"error": fmt.Sprintf("Too many wrong PINs. Try again in %d s.", int(d.Seconds())+1)})
		return
	}
	var req struct {
		Pin      string `json:"pin"`
		Remember bool   `json:"remember"`
		Device   string `json:"device"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad request"})
		return
	}
	if !checkPin(strings.TrimSpace(req.Pin)) {
		noteFail(ip)
		logf("wrong PIN from %s", ip)
		writeJSON(w, 401, map[string]any{"error": "Wrong PIN"})
		return
	}
	clearFail(ip)
	name := strings.TrimSpace(req.Device)
	if name == "" {
		name = "Device"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	if getConfig().RequireApproval {
		if !askApproval("KadduDesk - connection request",
			fmt.Sprintf("\"%s\" (%s) wants to view and control this PC.\n\nAllow?", name, ip), 30000) {
			writeJSON(w, 403, map[string]any{"error": "The PC owner declined the connection"})
			return
		}
	}
	var tok, id string
	if req.Remember {
		id, tok = addDevice(name, ip)
	} else {
		tok = newSessionToken(name)
	}
	logf("login ok: %s (%s) remember=%v", name, ip, req.Remember)
	writeJSON(w, 200, map[string]any{"token": tok, "device": id})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:    16 << 10,
	WriteBufferSize:   256 << 10,
	EnableCompression: false,
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if o == "" {
			return true
		}
		u, err := url.Parse(o)
		return err == nil && u.Host == r.Host
	},
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	devID, name, ok := authed(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if getConfig().Paused {
		http.Error(w, "sharing paused", 503)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	runSession(conn, name, devID, clientIP(r))
}

// ------------------------------------------------------------ file transfer

type fileEntry struct {
	Name string    `json:"name"`
	Path string    `json:"path"`
	Dir  bool      `json:"dir"`
	Size int64     `json:"size"`
	Mod  time.Time `json:"mod"`
}

func filesAllowed(w http.ResponseWriter, r *http.Request) bool {
	if _, _, ok := authed(r); !ok {
		writeJSON(w, 401, map[string]any{"error": "unauthorized"})
		return false
	}
	if !getConfig().AllowFiles {
		writeJSON(w, 403, map[string]any{"error": "File transfer is disabled on this PC"})
		return false
	}
	return true
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	if !filesAllowed(w, r) {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		home := userHome()
		var ents []fileEntry
		for _, q := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Videos", "Music"} {
			fp := filepath.Join(home, q)
			if st, err := os.Stat(fp); err == nil && st.IsDir() {
				ents = append(ents, fileEntry{Name: q, Path: fp, Dir: true})
			}
		}
		ents = append(ents, rootEntries()...)
		writeJSON(w, 200, map[string]any{"path": "", "parent": "", "entries": ents, "uploadDir": uploadDir()})
		return
	}
	p = filepath.Clean(p)
	list, err := os.ReadDir(p)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	ents := make([]fileEntry, 0, len(list))
	for _, e := range list {
		info, err := e.Info()
		if err != nil {
			continue
		}
		ents = append(ents, fileEntry{Name: e.Name(), Path: filepath.Join(p, e.Name()), Dir: e.IsDir(), Size: info.Size(), Mod: info.ModTime()})
	}
	sort.Slice(ents, func(i, j int) bool {
		if ents[i].Dir != ents[j].Dir {
			return ents[i].Dir
		}
		return strings.ToLower(ents[i].Name) < strings.ToLower(ents[j].Name)
	})
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	writeJSON(w, 200, map[string]any{"path": p, "parent": parent, "entries": ents, "uploadDir": uploadDir()})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if !filesAllowed(w, r) {
		return
	}
	p := filepath.Clean(r.URL.Query().Get("path"))
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "not a file", 400)
		return
	}
	logf("download %s", p)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(st.Name()))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "POST only"})
		return
	}
	if !filesAllowed(w, r) {
		return
	}
	name := filepath.Base(strings.ReplaceAll(r.URL.Query().Get("name"), "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "upload.bin"
	}
	dir := uploadDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	dst := filepath.Join(dir, name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
	f, err := os.Create(dst)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	n, err := io.Copy(f, r.Body)
	f.Close()
	if err != nil {
		os.Remove(dst)
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	logf("upload saved %s (%d bytes)", dst, n)
	writeJSON(w, 200, map[string]any{"ok": true, "path": dst, "size": n})
}

// ------------------------------------------------------------ LAN discovery

const discoveryPort = 47810

func discoveryLoop() {
	pc, err := net.ListenUDP("udp4", &net.UDPAddr{Port: discoveryPort})
	if err != nil {
		logf("discovery disabled: %v", err)
		return
	}
	buf := make([]byte, 512)
	for {
		n, addr, err := pc.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(string(buf[:n]), "KADDUDESK?") {
			continue
		}
		c := getConfig()
		m := map[string]any{"app": "kaddudesk", "name": c.Name, "port": c.Port,
			"fp": certFingerprint, "macs": macAddrs(), "version": version, "id": deviceID()}
		if hubRunning() {
			m["hubPort"] = c.HubPort
		}
		resp, _ := json.Marshal(m)
		pc.WriteToUDP(resp, addr)
	}
}
