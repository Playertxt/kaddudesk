package main

// The KadduDesk app core. It runs inside the KadduDesk app on every OS (Electron on
// Windows/Linux, the Android app) and:
//   - serves the app's own UI (bundled, never from the internet) on a private loopback port
//   - finds devices (LAN discovery, hubs, "anywhere" mesh peers) and pins their certificates
//   - carries every session to the chosen device: directly, through a hub, or through the mesh
//   - shows and changes this device's own sharing settings
// The loopback UI is locked to the app with a per-launch secret cookie and a Host check.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed appui
var appUIFS embed.FS

var (
	flagParentWatch = flag.Bool("parentwatch", false, "exit when stdin closes (the app shell owns this process)")
	flagDevName     = flag.String("devname", "", "this device's display name (Android)")
)

type AppDevice struct {
	Key   string    `json:"key"`
	Kind  string    `json:"kind"`  // "pc" or "hub"
	Route string    `json:"route"` // "lan" or "mesh"
	Addr  string    `json:"addr"`  // host:port
	FP    string    `json:"fp"`
	Name  string    `json:"name"`
	OS    string    `json:"os"`
	ID    string    `json:"id"`
	Macs  []macInfo `json:"macs,omitempty"`
	Last  time.Time `json:"last"`
}

type appCookie struct {
	Value   string    `json:"value"`
	Expires time.Time `json:"expires"`
}

type appConfigT struct {
	Port        int                  `json:"port"`
	Devices     []AppDevice          `json:"devices"`
	MeshEnabled bool                 `json:"meshEnabled"`
	HubCookies  map[string]appCookie `json:"hubCookies"`
}

type nearbyDev struct {
	Addr    string    `json:"addr"`
	Host    string    `json:"host"`
	Port    int       `json:"port"`
	HubPort int       `json:"hubPort"`
	Name    string    `json:"name"`
	OS      string    `json:"os"`
	FP      string    `json:"fp"`
	ID      string    `json:"id"`
	Macs    []macInfo `json:"macs"`
	Seen    time.Time `json:"-"`
}

var app = struct {
	mu        sync.Mutex
	cfg       appConfigT
	secret    string
	port      int
	nearby    map[string]*nearbyDev
	sessCk    map[string]string // non-persistent hub cookies
	proxies   map[string]*httputil.ReverseProxy
	inproc    bool   // the host runs inside this process (Linux/Android)
	ownFP     string // this device's certificate, to hide ourselves from lists
	extCache  time.Time
	extOK     bool
	startErr  string
	startedAt time.Time
}{nearby: map[string]*nearbyDev{}, sessCk: map[string]string{}, proxies: map[string]*httputil.ReverseProxy{}}

func appDir() string { return filepath.Join(dataDir, "app") }

func appCfgName() string {
	if *flagDevName != "" {
		return *flagDevName
	}
	h, _ := os.Hostname()
	return h
}

func loadAppConfig() {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.cfg = appConfigT{HubCookies: map[string]appCookie{}}
	if b, err := os.ReadFile(filepath.Join(appDir(), "app.json")); err == nil {
		json.Unmarshal(b, &app.cfg)
	}
	if app.cfg.HubCookies == nil {
		app.cfg.HubCookies = map[string]appCookie{}
	}
}

func saveAppConfigLocked() {
	b, _ := json.MarshalIndent(app.cfg, "", " ")
	p := filepath.Join(appDir(), "app.json")
	if os.WriteFile(p+".tmp", b, 0o600) == nil {
		os.Rename(p+".tmp", p)
	}
}

// runApp is the entry point of `-app`.
func runApp() {
	os.MkdirAll(appDir(), 0o700)
	setupLog(filepath.Join(appDir(), "app.log"))
	loadAppConfig()
	app.startedAt = time.Now()
	logf("KadduDesk app core %s on %s", version, platformName())

	// Linux/Android: the app itself makes this device shareable (no separate server needed)
	if runtime.GOOS != "windows" && acquireHostLock() {
		if err := startHost(); err != nil {
			app.startErr = err.Error()
			logf("host: %v", err)
		} else {
			app.inproc = true
		}
	}
	app.ownFP = localHostFingerprint()
	if !app.inproc {
		app.mu.Lock()
		on := app.cfg.MeshEnabled
		app.mu.Unlock()
		if on && !externalHostHasMesh() {
			go startMesh(true)
		}
	}

	ln, port := listenAppPort()
	app.port = port
	app.secret = randomHex(16)
	fmt.Printf("KDAPP http://127.0.0.1:%d/app/?k=%s\n", port, app.secret)
	os.Stdout.Sync()
	if *flagParentWatch {
		go func() { io.Copy(io.Discard, os.Stdin); logf("app shell closed — exiting"); os.Exit(0) }()
	}
	go appDiscoveryLoop()
	srv := &http.Server{Handler: http.HandlerFunc(appHandler), ReadHeaderTimeout: 20 * time.Second}
	logf("app UI on 127.0.0.1:%d", port)
	srv.Serve(ln)
}

// listenAppPort keeps the same port across launches (the UI's saved logins live per origin).
func listenAppPort() (net.Listener, int) {
	app.mu.Lock()
	want := app.cfg.Port
	app.mu.Unlock()
	if *flagAppPort > 0 {
		want = *flagAppPort
	}
	try := func(p int) net.Listener {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			return nil
		}
		return ln
	}
	if want > 0 {
		if ln := try(want); ln != nil {
			return ln, want
		}
	}
	for i := 0; i < 50; i++ {
		p := 47900 + mrand.IntN(1000)
		if ln := try(p); ln != nil {
			app.mu.Lock()
			app.cfg.Port = p
			saveAppConfigLocked()
			app.mu.Unlock()
			return ln, p
		}
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func localHostFingerprint() string {
	if certFingerprint != "" {
		return certFingerprint
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "cert.pem"))
	if err != nil {
		return ""
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return ""
	}
	s := sha256.Sum256(blk.Bytes)
	return strings.ToUpper(hex.EncodeToString(s[:]))
}

// ------------------------------------------------------------ HTTP entry

func appHandler(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.Host)
	if host != "127.0.0.1" && host != "localhost" {
		http.Error(w, "forbidden", 403)
		return
	}
	if k := r.URL.Query().Get("k"); k != "" {
		if subtle.ConstantTimeCompare([]byte(k), []byte(app.secret)) == 1 {
			http.SetCookie(w, &http.Cookie{Name: "kdapp", Value: app.secret, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/app/", http.StatusFound)
			return
		}
	}
	ck, err := r.Cookie("kdapp")
	if err != nil || subtle.ConstantTimeCompare([]byte(ck.Value), []byte(app.secret)) != 1 {
		http.Error(w, "Open KadduDesk from its app icon.", 403)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p := r.URL.Path
	switch {
	case p == "/" || p == "/app":
		http.Redirect(w, r, "/app/", http.StatusFound)
	case strings.HasPrefix(p, "/app/api/"):
		appAPI(w, r, strings.TrimPrefix(p, "/app/api/"))
	case strings.HasPrefix(p, "/app/s/"):
		appSession(w, r, strings.TrimPrefix(p, "/app/s/"))
	case strings.HasPrefix(p, "/app/hubui/"):
		appHubUI(w, r, strings.TrimPrefix(p, "/app/hubui/"))
	case strings.HasPrefix(p, "/app/local/"):
		appLocal(w, r, strings.TrimPrefix(p, "/app/local/"))
	case strings.HasPrefix(p, "/app/"):
		serveAppStatic(w, r, strings.TrimPrefix(p, "/app/"))
	default:
		http.NotFound(w, r)
	}
}

func serveAppStatic(w http.ResponseWriter, r *http.Request, name string) {
	if name == "" {
		name = "index.html"
	}
	switch name {
	case "bridge.js", "style.css", "icon.png":
		sub, _ := fs.Sub(webFS, "web")
		http.ServeFileFS(w, r, sub, name)
		return
	}
	sub, _ := fs.Sub(appUIFS, "appui")
	if _, err := fs.Stat(sub, name); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-src 'self'; connect-src 'self'")
	http.ServeFileFS(w, r, sub, name)
}

// serveSessionStatic serves the remote-session screen (bundled in the app).
func serveSessionStatic(w http.ResponseWriter, r *http.Request, name string) {
	if name == "" {
		name = "index.html"
	}
	sub, _ := fs.Sub(webFS, "web")
	if _, err := fs.Stat(sub, name); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, sub, name)
}

// ------------------------------------------------------------ app API

func appAPI(w http.ResponseWriter, r *http.Request, p string) {
	post := r.Method == http.MethodPost
	var body map[string]any
	if post {
		json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body)
	}
	str := func(k string) string { s, _ := body[k].(string); return strings.TrimSpace(s) }
	switch p {
	case "state":
		mode := "off"
		if app.inproc {
			mode = "inproc"
		} else if externalHostOK() {
			mode = "external"
		}
		writeJSON(w, 200, map[string]any{"platform": platformName(), "version": version, "name": appCfgName(),
			"host": mode, "hostError": app.startErr, "canStartHost": runtime.GOOS == "windows" || !app.inproc,
			"meshMode": appMeshMode()})
	case "devices":
		writeJSON(w, 200, appDeviceList())
	case "probe":
		route, addr := str("route"), normAddr(str("addr"), 47800)
		if route == "" {
			route = "lan"
		}
		fp, info, err := appProbe(route, addr)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": "Can't reach " + addr + ": " + friendlyNetErr(err)})
			return
		}
		app.mu.Lock()
		var known string
		for _, d := range app.cfg.Devices {
			if d.Addr == addr && d.Route == route {
				known = d.FP
			}
		}
		app.mu.Unlock()
		writeJSON(w, 200, map[string]any{"fp": fp, "info": info, "known": known, "addr": addr, "route": route})
	case "trust":
		route, addr, fp := str("route"), normAddr(str("addr"), 47800), strings.ToUpper(str("fp"))
		if len(fp) != 64 {
			writeJSON(w, 400, map[string]any{"error": "bad fingerprint"})
			return
		}
		info, _ := body["info"].(map[string]any)
		d := AppDevice{Kind: "pc", Route: route, Addr: addr, FP: fp, Last: time.Now()}
		if info != nil {
			if info["app"] == "kaddudesk-hub" {
				d.Kind = "hub"
			}
			d.Name, _ = info["name"].(string)
			d.OS, _ = info["os"].(string)
			d.ID, _ = info["id"].(string)
			if ms, ok := info["macs"].([]any); ok {
				b, _ := json.Marshal(ms)
				json.Unmarshal(b, &d.Macs)
			}
		}
		if d.Name == "" {
			d.Name = addr
		}
		app.mu.Lock()
		found := false
		for i := range app.cfg.Devices {
			x := &app.cfg.Devices[i]
			if x.Addr == addr && x.Route == route && x.Kind == d.Kind {
				d.Key = x.Key
				*x = d
				found = true
			}
		}
		if !found {
			d.Key = randomHex(5)
			app.cfg.Devices = append(app.cfg.Devices, d)
		}
		delete(app.proxies, d.Key)
		saveAppConfigLocked()
		app.mu.Unlock()
		writeJSON(w, 200, map[string]any{"key": d.Key, "kind": d.Kind})
	case "remove":
		k := str("key")
		app.mu.Lock()
		out := app.cfg.Devices[:0]
		for _, d := range app.cfg.Devices {
			if d.Key != k {
				out = append(out, d)
			}
		}
		app.cfg.Devices = out
		delete(app.cfg.HubCookies, k)
		saveAppConfigLocked()
		app.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	case "rename":
		k, n := str("key"), str("name")
		app.mu.Lock()
		for i := range app.cfg.Devices {
			if app.cfg.Devices[i].Key == k && n != "" {
				app.cfg.Devices[i].Name = n
			}
		}
		saveAppConfigLocked()
		app.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	case "wake":
		d := savedDevice(str("key"))
		if d == nil || len(d.Macs) == 0 {
			writeJSON(w, 400, map[string]any{"error": "No Wake-on-LAN info for this device yet — connect to it once while it's on."})
			return
		}
		for _, m := range d.Macs {
			sendWOL(m.MAC, m.Bcast)
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case "mesh/status":
		writeJSON(w, 200, appMeshStatus())
	case "mesh/enable":
		on, _ := body["on"].(bool)
		if err := appMeshEnable(on); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case "mesh/login":
		u, err := appMeshCall("login")
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"url": u})
	case "mesh/logout":
		if _, err := appMeshCall("logout"); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case "local/start":
		if err := appStartSharing(); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		writeJSON(w, 404, map[string]any{"error": "unknown"})
	}
}

func normAddr(a string, defPort int) string {
	a = strings.TrimSpace(a)
	a = strings.TrimPrefix(strings.TrimPrefix(a, "https://"), "http://")
	a = strings.TrimSuffix(a, "/")
	if _, _, err := net.SplitHostPort(a); err != nil {
		if strings.Count(a, ":") > 1 && !strings.HasPrefix(a, "[") {
			a = "[" + a + "]"
		}
		a = fmt.Sprintf("%s:%d", a, defPort)
	}
	return a
}

func friendlyNetErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "refused"):
		return "nothing is listening there (is KadduDesk running on it?)"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline"):
		return "no answer (different network or firewall?)"
	case strings.Contains(s, "no such host"):
		return "unknown name"
	}
	return s
}

func savedDevice(key string) *AppDevice {
	app.mu.Lock()
	defer app.mu.Unlock()
	for i := range app.cfg.Devices {
		if app.cfg.Devices[i].Key == key {
			d := app.cfg.Devices[i]
			return &d
		}
	}
	return nil
}

// ------------------------------------------------------------ device list

type devOut struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Route   string `json:"route"`
	Addr    string `json:"addr"`
	Name    string `json:"name"`
	OS      string `json:"os"`
	ID      string `json:"id"`
	Pretty  string `json:"pretty"`
	Online  bool   `json:"online"`
	Saved   bool   `json:"saved"`
	Wake    bool   `json:"wake"`
	Section string `json:"section"` // saved | nearby | anywhere
}

func appDeviceList() map[string]any {
	if app.ownFP == "" { // sharing may have been turned on after the app started
		app.ownFP = localHostFingerprint()
	}
	app.mu.Lock()
	saved := append([]AppDevice(nil), app.cfg.Devices...)
	var near []*nearbyDev
	for _, n := range app.nearby {
		if time.Since(n.Seen) < 15*time.Second && (app.ownFP == "" || n.FP != app.ownFP) {
			c := *n
			near = append(near, &c)
		}
	}
	app.mu.Unlock()
	peers := appMeshPeers()
	var out []devOut
	have := map[string]bool{}
	for _, d := range saved {
		online := false
		for _, n := range near {
			if n.FP == d.FP || n.Addr == d.Addr {
				online = true
			}
		}
		for _, p := range peers {
			if d.Route == "mesh" && strings.HasPrefix(d.Addr, p.IP+":") && p.Online {
				online = true
			}
		}
		out = append(out, devOut{Key: d.Key, Kind: d.Kind, Route: d.Route, Addr: d.Addr, Name: d.Name, OS: d.OS, ID: d.ID,
			Pretty: prettyID(d.ID), Online: online, Saved: true, Wake: len(d.Macs) > 0, Section: "saved"})
		have[d.Route+"|"+d.Addr+"|"+d.Kind] = true
	}
	sort.Slice(near, func(i, j int) bool { return near[i].Name < near[j].Name })
	for _, n := range near {
		if !have["lan|"+n.Addr+"|pc"] {
			out = append(out, devOut{Key: "lan:" + n.Addr, Kind: "pc", Route: "lan", Addr: n.Addr, Name: n.Name, OS: n.OS, ID: n.ID,
				Pretty: prettyID(n.ID), Online: true, Section: "nearby"})
		}
		if n.HubPort > 0 {
			ha := net.JoinHostPort(n.Host, itoa(n.HubPort))
			if !have["lan|"+ha+"|hub"] {
				out = append(out, devOut{Key: "lan:" + ha, Kind: "hub", Route: "lan", Addr: ha, Name: n.Name, OS: n.OS, Online: true, Section: "nearby"})
			}
		}
	}
	for _, p := range peers {
		if p.App {
			continue
		}
		a := p.IP + ":47800"
		if have["mesh|"+a+"|pc"] {
			continue
		}
		out = append(out, devOut{Key: "mesh:" + a, Kind: "pc", Route: "mesh", Addr: a, Name: p.Name, OS: p.OS, Online: p.Online, Section: "anywhere"})
	}
	return map[string]any{"devices": out, "mesh": appMeshMode() != "" && len(peers) > 0}
}

// ------------------------------------------------------------ LAN discovery (client side)

func appDiscoveryLoop() {
	for {
		appDiscoverOnce()
		time.Sleep(4 * time.Second)
	}
}

func appDiscoverOnce() {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return
	}
	defer c.Close()
	targets := map[string]bool{"255.255.255.255": true}
	for _, m := range macAddrs() {
		targets[m.Bcast] = true
	}
	for _, ip := range extraBroadcasts() {
		targets[ip] = true
	}
	for t := range targets {
		c.WriteToUDP([]byte("KADDUDESK?"), &net.UDPAddr{IP: net.ParseIP(t), Port: discoveryPort})
	}
	buf := make([]byte, 4096)
	end := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(end) {
		c.SetReadDeadline(end)
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			break
		}
		var j nearbyDev
		var raw map[string]any
		if json.Unmarshal(buf[:n], &raw) != nil || raw["app"] != "kaddudesk" {
			continue
		}
		json.Unmarshal(buf[:n], &j)
		j.Host = from.IP.String()
		if j.Port == 0 {
			j.Port = 47800
		}
		j.Addr = net.JoinHostPort(j.Host, itoa(j.Port))
		j.Seen = time.Now()
		app.mu.Lock()
		// one entry per device: prefer home-LAN addresses over VPN/virtual adapters
		key := j.FP
		if key == "" {
			key = j.Addr
		}
		if old := app.nearby[key]; old == nil || time.Since(old.Seen) > 10*time.Second || ipRank(j.Host) <= ipRank(old.Host) {
			app.nearby[key] = &j
		} else {
			old.Seen = time.Now()
		}
		app.mu.Unlock()
	}
}

func ipRank(ip string) int {
	switch {
	case strings.HasPrefix(ip, "192.168.56."), strings.HasPrefix(ip, "169.254."):
		return 4
	case strings.HasPrefix(ip, "192.168."):
		return 0
	case strings.HasPrefix(ip, "10."):
		return 1
	case strings.HasPrefix(ip, "172."):
		return 2
	}
	return 3
}

// ------------------------------------------------------------ connecting

func appDial(ctx context.Context, route, addr string) (net.Conn, error) {
	if route == "mesh" {
		return appMeshDial(ctx, addr)
	}
	return (&net.Dialer{Timeout: 6 * time.Second, KeepAlive: 15 * time.Second}).DialContext(ctx, "tcp", addr)
}

// appProbe connects once, records the certificate and reads /api/info.
func appProbe(route, addr string) (string, map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	raw, err := appDial(ctx, route, addr)
	if err != nil {
		return "", nil, err
	}
	defer raw.Close()
	raw.SetDeadline(time.Now().Add(8 * time.Second))
	tc := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(ctx); err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(tc.ConnectionState().PeerCertificates[0].Raw)
	fp := strings.ToUpper(hex.EncodeToString(sum[:]))
	fmt.Fprintf(tc, "GET /api/info HTTP/1.1\r\nHost: kd-device\r\nConnection: close\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(tc), nil)
	if err != nil {
		return fp, nil, err
	}
	defer resp.Body.Close()
	var info map[string]any
	json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&info)
	if info == nil || (info["app"] != "kaddudesk" && info["app"] != "kaddudesk-hub") {
		return fp, nil, errors.New("that isn't a KadduDesk device")
	}
	return fp, info, nil
}

func deviceTransport(d *AppDevice) *http.Transport {
	route, addr := d.Route, d.Addr
	return &http.Transport{
		DialContext:           func(ctx context.Context, _, _ string) (net.Conn, error) { return appDial(ctx, route, addr) },
		TLSClientConfig:       pinnedTLS(d.FP),
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
		ForceAttemptHTTP2:     false,
	}
}

func proxyFor(d *AppDevice, hub bool) *httputil.ReverseProxy {
	app.mu.Lock()
	if p := app.proxies[d.Key]; p != nil {
		app.mu.Unlock()
		return p
	}
	app.mu.Unlock()
	key := d.Key
	vhost := "kd-device"
	if hub {
		vhost = "kd-hub"
	}
	p := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "https", Host: vhost})
			pr.Out.Host = vhost
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Set("Origin", "https://"+vhost)
			pr.Out.Header.Del("Referer")
			if hub {
				if v := hubCookie(key); v != "" {
					pr.Out.Header.Set("Cookie", sessCookie+"="+v)
				}
			}
		},
		Transport:     deviceTransport(d),
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			if hub {
				for _, c := range resp.Cookies() {
					if c.Name == sessCookie {
						setHubCookie(key, c)
					}
				}
			}
			resp.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeJSON(w, 502, map[string]any{"error": "Can't reach the device: " + friendlyNetErr(err)})
		},
	}
	app.mu.Lock()
	app.proxies[key] = p
	app.mu.Unlock()
	return p
}

func hubCookie(key string) string {
	app.mu.Lock()
	defer app.mu.Unlock()
	if v, ok := app.sessCk[key]; ok {
		return v
	}
	if c, ok := app.cfg.HubCookies[key]; ok && time.Now().Before(c.Expires) {
		return c.Value
	}
	return ""
}

func setHubCookie(key string, c *http.Cookie) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if c.Value == "" || c.MaxAge < 0 {
		delete(app.sessCk, key)
		delete(app.cfg.HubCookies, key)
		saveAppConfigLocked()
		return
	}
	if !c.Expires.IsZero() { // "keep me signed in"
		app.cfg.HubCookies[key] = appCookie{Value: c.Value, Expires: c.Expires}
		delete(app.sessCk, key)
		saveAppConfigLocked()
		return
	}
	app.sessCk[key] = c.Value
}

// /app/s/<key>/<rest> — a session to a device (direct or mesh)
func appSession(w http.ResponseWriter, r *http.Request, rest string) {
	key, sub, ok := strings.Cut(rest, "/")
	d := savedDevice(key)
	if d == nil {
		http.Error(w, "Unknown device — go back and pick it again.", 404)
		return
	}
	if !ok {
		http.Redirect(w, r, "/app/s/"+key+"/", http.StatusFound)
		return
	}
	if strings.HasPrefix(sub, "api/") || sub == "ws" {
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = "/"+sub, ""
		proxyFor(d, false).ServeHTTP(w, r2)
		return
	}
	serveSessionStatic(w, r, sub)
}

// /app/hubui/<key>/<rest> — the hub screens (bundled) + its API and devices through the hub
func appHubUI(w http.ResponseWriter, r *http.Request, rest string) {
	key, sub, ok := strings.Cut(rest, "/")
	d := savedDevice(key)
	if d == nil || d.Kind != "hub" {
		http.Error(w, "Unknown hub — go back and pick it again.", 404)
		return
	}
	if !ok {
		http.Redirect(w, r, "/app/hubui/"+key+"/", http.StatusFound)
		return
	}
	proxyTo := func(target string) {
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = target, ""
		proxyFor(d, true).ServeHTTP(w, r2)
	}
	switch {
	case sub == "" || sub == "index.html":
		hs, _ := fs.Sub(hubFS, "hubweb")
		http.ServeFileFS(w, r, hs, "index.html")
	case sub == "hub.js":
		hs, _ := fs.Sub(hubFS, "hubweb")
		http.ServeFileFS(w, r, hs, "hub.js")
	case sub == "bridge.js" || sub == "style.css" || sub == "icon.png":
		serveSessionStatic(w, r, sub)
	case strings.HasPrefix(sub, "hub/api/"):
		proxyTo("/" + sub)
	case strings.HasPrefix(sub, "d/"):
		parts := strings.SplitN(strings.TrimPrefix(sub, "d/"), "/", 2)
		if len(parts) < 2 {
			http.Redirect(w, r, "/app/hubui/"+key+"/"+sub+"/", http.StatusFound)
			return
		}
		inner := parts[1]
		if strings.HasPrefix(inner, "api/") || inner == "ws" {
			proxyTo("/d/" + parts[0] + "/" + inner)
			return
		}
		serveSessionStatic(w, r, inner)
	default:
		http.NotFound(w, r)
	}
}

// ------------------------------------------------------------ this device (local host control)

// externalHost returns how to reach a host running as a separate program on this PC.
func externalHost() (base, key string, ok bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, "adminkey"))
	if err != nil {
		return "", "", false
	}
	port := 47801
	if cb, err := os.ReadFile(filepath.Join(dataDir, "server.json")); err == nil {
		var c struct {
			AdminPort int `json:"adminPort"`
		}
		if json.Unmarshal(cb, &c) == nil && c.AdminPort > 0 {
			port = c.AdminPort
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), strings.TrimSpace(string(b)), true
}

func externalHostOK() bool {
	app.mu.Lock()
	if time.Since(app.extCache) < 2*time.Second {
		ok := app.extOK
		app.mu.Unlock()
		return ok
	}
	app.mu.Unlock()
	ok := false
	if base, key, have := externalHost(); have {
		req, _ := http.NewRequest("GET", base+"/admin/mesh/status", nil)
		req.Header.Set("X-KD-Key", key)
		cl := &http.Client{Timeout: 1500 * time.Millisecond}
		if resp, err := cl.Do(req); err == nil {
			ok = resp.StatusCode == 200
			resp.Body.Close()
		}
	}
	app.mu.Lock()
	app.extCache, app.extOK = time.Now(), ok
	app.mu.Unlock()
	return ok
}

func externalHostHasMesh() bool {
	if !externalHostOK() {
		return false
	}
	var st map[string]any
	if err := extAdminJSON("GET", "/admin/mesh/status", nil, &st); err != nil {
		return false
	}
	on, _ := st["enabled"].(bool)
	return on
}

func extAdminJSON(method, p string, body any, out any) error {
	base, key, ok := externalHost()
	if !ok {
		return errors.New("sharing is off")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, base+p, rd)
	req.Header.Set("X-KD-Key", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var j map[string]any
	json.NewDecoder(resp.Body).Decode(&j)
	if resp.StatusCode != 200 {
		if e, _ := j["error"].(string); e != "" {
			return errors.New(e)
		}
		return fmt.Errorf("error %d", resp.StatusCode)
	}
	if out != nil {
		b, _ := json.Marshal(j)
		json.Unmarshal(b, out)
	}
	return nil
}

// /app/local/<rest> — this device's sharing settings (the old control panel, inside the app)
func appLocal(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "" || rest == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(adminHTML)
		return
	}
	if !strings.HasPrefix(rest, "admin/") {
		http.NotFound(w, r)
		return
	}
	if app.inproc && hostStarted {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + rest
		r2.Header.Set("X-KD-Key", adminKey)
		r2.Host = "127.0.0.1"
		adminMuxV.ServeHTTP(w, r2)
		return
	}
	base, key, ok := externalHost()
	if !ok || !externalHostOK() {
		writeJSON(w, 503, map[string]any{"error": "Sharing is off on this device", "off": true})
		return
	}
	u, _ := url.Parse(base)
	p := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(u)
			pr.Out.URL.Path = "/" + rest
			pr.Out.Host = u.Host
			pr.Out.Header.Set("X-KD-Key", key)
			pr.Out.Header.Del("Cookie")
		},
		FlushInterval: -1,
	}
	p.ServeHTTP(w, r)
}

// appStartSharing turns this device's sharing on.
func appStartSharing() error {
	if app.inproc {
		cfgMu.Lock()
		cfg.Paused = false
		saveConfigLocked()
		cfgMu.Unlock()
		return nil
	}
	if runtime.GOOS != "windows" {
		if acquireHostLock() {
			if err := startHost(); err != nil {
				return err
			}
			app.inproc = true
			return nil
		}
		return errors.New("KadduDesk Server is already running separately on this computer")
	}
	return launchHostElevated()
}

// ------------------------------------------------------------ mesh from the app

// appMeshMode: which node the app uses — "inproc" (Linux/Android host node), "host"
// (the separate KadduDesk server on this PC), "own" (the app's own node), or "".
func appMeshMode() string {
	if app.inproc {
		if getConfig().MeshEnabled {
			return "inproc"
		}
		return ""
	}
	if externalHostHasMesh() {
		return "host"
	}
	if meshRunning() {
		return "own"
	}
	return ""
}

func appMeshStatus() map[string]any {
	switch appMeshMode() {
	case "inproc", "own":
		m := meshStatusMap()
		m["enabled"] = true
		return m
	case "host":
		var st map[string]any
		if extAdminJSON("GET", "/admin/mesh/status", nil, &st) == nil {
			return st
		}
	}
	return map[string]any{"enabled": false, "state": "Off"}
}

func appMeshEnable(on bool) error {
	if app.inproc {
		cfgMu.Lock()
		cfg.MeshEnabled = on
		saveConfigLocked()
		cfgMu.Unlock()
		if on {
			return startMesh(false)
		}
		stopMesh()
		return nil
	}
	if externalHostOK() { // let this PC's KadduDesk server own the node (one sign-in per PC)
		q := "0"
		if on {
			q = "1"
		}
		if meshRunning() {
			stopMesh()
		}
		return extAdminJSON("GET", "/admin/mesh/enable?on="+q, nil, nil)
	}
	app.mu.Lock()
	app.cfg.MeshEnabled = on
	saveAppConfigLocked()
	app.mu.Unlock()
	if on {
		return startMesh(true)
	}
	stopMesh()
	return nil
}

func appMeshCall(what string) (string, error) {
	switch appMeshMode() {
	case "host":
		var j map[string]any
		if err := extAdminJSON("GET", "/admin/mesh/"+what, nil, &j); err != nil {
			return "", err
		}
		u, _ := j["url"].(string)
		return u, nil
	case "inproc", "own":
		if what == "login" {
			return meshLogin()
		}
		return "", meshLogout()
	}
	return "", errors.New("turn on Connect from anywhere first")
}

func appMeshPeers() []meshPeer {
	switch appMeshMode() {
	case "inproc", "own":
		return meshPeersList()
	case "host":
		var j struct {
			Peers []meshPeer `json:"peers"`
		}
		if extAdminJSON("GET", "/admin/mesh/peers", nil, &j) == nil {
			return j.Peers
		}
	}
	return nil
}

func appMeshDial(ctx context.Context, addr string) (net.Conn, error) {
	switch appMeshMode() {
	case "inproc", "own":
		return meshDialLocal(ctx, addr)
	case "host":
		base, key, _ := externalHost()
		d := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
		ws, _, err := d.DialContext(ctx, strings.Replace(base, "http://", "ws://", 1)+"/admin/mesh/dial?addr="+url.QueryEscape(addr),
			http.Header{"X-KD-Key": {key}})
		if err != nil {
			return nil, err
		}
		return newWSConn(ws), nil
	}
	return nil, errors.New("Connect from anywhere is off — turn it on in the Anywhere tab")
}
