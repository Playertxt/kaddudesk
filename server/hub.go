package main

// The hub (runs on your main PC): devices connect OUT to it and register with a device ID;
// clients log in to the hub, see every device and open one at /d/<id>/. The hub relays all
// traffic (video, input, files) through the devices' outbound connections, so any device can
// control any other one, across networks, with the load on the main PC.

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed hubweb
var hubFS embed.FS

type HubDevRec struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	OS       string    `json:"os"`
	LastSeen time.Time `json:"lastSeen"`
	LastIP   string    `json:"lastIP"`
	Macs     []macInfo `json:"macs"`
}

type hubDev struct {
	id, name, os, version, ip string
	macs                      []macInfo
	ws                        *websocket.Conn
	wmu                       sync.Mutex
	since                     time.Time
}

var hub = struct {
	mu      sync.Mutex
	devs    map[string]*hubDev
	pending map[string]chan *wsNetConn
	proxies map[string]*httputil.ReverseProxy
	srv     *http.Server
	sess    map[string]time.Time // short-lived client sessions (tokenHash → expiry)
}{devs: map[string]*hubDev{}, pending: map[string]chan *wsNetConn{}, proxies: map[string]*httputil.ReverseProxy{}, sess: map[string]time.Time{}}

var hubMuxV *http.ServeMux

func currentHubMux() *http.ServeMux {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return hubMuxV
}

// noBrowserPage answers browsers that open a KadduDesk port directly.
func noBrowserPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>KadduDesk</title><body style="font:16px system-ui;background:#0f1115;color:#eee;display:grid;place-items:center;height:90vh;text-align:center"><div><h2>KadduDesk works in its app</h2><p>Install the KadduDesk app on Windows, Android or Linux and connect from there.</p></div>`))
}

var idRe = regexp.MustCompile(`^/d/(\d{9})(/.*)?$`)

func hubRunning() bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return hub.srv != nil
}

// applyHubSettings starts or stops the hub listener to match the config.
func applyHubSettings(cert tls.Certificate) error {
	c := getConfig()
	hub.mu.Lock()
	running := hub.srv != nil
	hub.mu.Unlock()
	if running {
		hub.mu.Lock()
		hub.srv.Close()
		hub.srv = nil
		hubMuxV = nil
		hub.mu.Unlock()
	}
	if !c.HubEnabled {
		return nil
	}
	if c.HubHostKey == "" {
		cfgMu.Lock()
		cfg.HubHostKey = randomHex(12)
		saveConfigLocked()
		cfgMu.Unlock()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", noBrowserPage) // KadduDesk is app-only: no web pages
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app": "kaddudesk-hub", "name": getConfig().Name, "version": version, "fp": certFingerprint, "os": platformName(), "id": deviceID()})
	})
	registerAccountAPI(mux)
	mux.HandleFunc("/hub/host", hubHostConn)
	mux.HandleFunc("/hub/data", hubDataConn)
	mux.HandleFunc("/d/", hubDeviceProxy)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", c.HubPort), Handler: mux, ReadHeaderTimeout: 20 * time.Second,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("hub port %d: %w", c.HubPort, err)
	}
	hub.mu.Lock()
	hub.srv = srv
	hubMuxV = mux
	hub.mu.Unlock()
	go srv.ServeTLS(tcpNoDelay{ln}, "", "")
	logf("hub listening on :%d", c.HubPort)
	if a := *flagDevHub; a != "" && (strings.HasPrefix(a, "127.0.0.1:") || strings.HasPrefix(a, "localhost:")) {
		if dl, err := net.Listen("tcp", a); err == nil {
			go http.Serve(dl, mux)
			logf("dev hub http on %s", a)
		}
	}
	return nil
}

func hubDeviceList() []map[string]any {
	c := getConfig()
	self := deviceID()
	out := []map[string]any{{"id": self, "pretty": prettyID(self), "name": c.Name, "os": platformName(),
		"online": true, "self": true, "version": version}}
	hub.mu.Lock()
	online := map[string]*hubDev{}
	for id, d := range hub.devs {
		online[id] = d
	}
	hub.mu.Unlock()
	seen := map[string]bool{self: true}
	var rest []map[string]any
	for id, d := range online {
		seen[id] = true
		rest = append(rest, map[string]any{"id": id, "pretty": prettyID(id), "name": d.name, "os": d.os, "online": true,
			"since": d.since, "ip": d.ip, "version": d.version, "wake": len(d.macs) > 0})
	}
	for _, k := range c.HubKnown {
		if !seen[k.ID] {
			rest = append(rest, map[string]any{"id": k.ID, "pretty": prettyID(k.ID), "name": k.Name, "os": k.OS,
				"online": false, "lastSeen": k.LastSeen, "wake": len(k.Macs) > 0})
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		if rest[i]["online"] != rest[j]["online"] {
			return rest[i]["online"].(bool)
		}
		return strings.ToLower(rest[i]["name"].(string)) < strings.ToLower(rest[j]["name"].(string))
	})
	return append(out, rest...)
}

// ------------------------------------------------------------ device (host) connections

func hubKeyOK(r *http.Request) bool {
	k := getConfig().HubHostKey
	return k != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-KD-Hub-Key")), []byte(k)) == 1
}

var hubUpgrader = websocket.Upgrader{ReadBufferSize: 64 << 10, WriteBufferSize: 256 << 10,
	CheckOrigin: func(*http.Request) bool { return true }} // native host connections, key-authenticated

func hubHostConn(w http.ResponseWriter, r *http.Request) {
	if !hubKeyOK(r) {
		http.Error(w, "bad host key", 403)
		return
	}
	ws, err := hubUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(1 << 16)
	var hello struct {
		T, ID, Name, OS, Version, FP string
		Macs                         []macInfo
	}
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	if ws.ReadJSON(&hello) != nil || hello.T != "hello" || !regexp.MustCompile(`^\d{9}$`).MatchString(hello.ID) {
		return
	}
	if hello.ID == deviceID() {
		return // that's us
	}
	d := &hubDev{id: hello.ID, name: hello.Name, os: hello.OS, version: hello.Version, ip: clientIP(r), macs: hello.Macs, ws: ws, since: time.Now()}
	hub.mu.Lock()
	if old := hub.devs[d.id]; old != nil {
		old.ws.Close()
	}
	hub.devs[d.id] = d
	delete(hub.proxies, d.id)
	hub.mu.Unlock()
	rememberHubDevice(d)
	logf("hub: device online %s %q (%s, %s)", prettyID(d.id), d.name, d.os, d.ip)
	defer func() {
		hub.mu.Lock()
		if hub.devs[d.id] == d {
			delete(hub.devs, d.id)
		}
		hub.mu.Unlock()
		rememberHubDevice(d)
		logf("hub: device offline %s %q", prettyID(d.id), d.name)
	}()
	ws.SetPingHandler(func(s string) error {
		ws.SetReadDeadline(time.Now().Add(70 * time.Second))
		d.wmu.Lock()
		defer d.wmu.Unlock()
		return ws.WriteControl(websocket.PongMessage, []byte(s), time.Now().Add(5*time.Second))
	})
	for {
		ws.SetReadDeadline(time.Now().Add(70 * time.Second))
		if _, _, err := ws.ReadMessage(); err != nil {
			return
		}
	}
}

func rememberHubDevice(d *hubDev) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	rec := HubDevRec{ID: d.id, Name: d.name, OS: d.os, LastSeen: time.Now(), LastIP: d.ip, Macs: d.macs}
	for i := range cfg.HubKnown {
		if cfg.HubKnown[i].ID == d.id {
			cfg.HubKnown[i] = rec
			saveConfigLocked()
			return
		}
	}
	cfg.HubKnown = append(cfg.HubKnown, rec)
	saveConfigLocked()
}

func forgetHubDevice(id string) {
	cfgMu.Lock()
	out := cfg.HubKnown[:0]
	for _, k := range cfg.HubKnown {
		if k.ID != id {
			out = append(out, k)
		}
	}
	cfg.HubKnown = out
	saveConfigLocked()
	cfgMu.Unlock()
	hub.mu.Lock()
	if d := hub.devs[id]; d != nil {
		d.ws.Close()
	}
	hub.mu.Unlock()
}

func hubDataConn(w http.ResponseWriter, r *http.Request) {
	if !hubKeyOK(r) {
		http.Error(w, "bad host key", 403)
		return
	}
	id := r.URL.Query().Get("c")
	hub.mu.Lock()
	ch := hub.pending[id]
	delete(hub.pending, id)
	hub.mu.Unlock()
	if ch == nil {
		http.Error(w, "unknown connection", 404)
		return
	}
	ws, err := hubUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch <- newWSConn(ws)
}

// dialDevice asks the device to open a data connection back to us.
func dialDevice(ctx context.Context, id string) (net.Conn, error) {
	hub.mu.Lock()
	d := hub.devs[id]
	if d == nil {
		hub.mu.Unlock()
		return nil, errors.New("device offline")
	}
	cid := randomHex(10)
	ch := make(chan *wsNetConn, 1)
	hub.pending[cid] = ch
	hub.mu.Unlock()
	defer func() {
		hub.mu.Lock()
		delete(hub.pending, cid)
		hub.mu.Unlock()
	}()
	d.wmu.Lock()
	d.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := d.ws.WriteJSON(map[string]string{"t": "dial", "c": cid})
	d.wmu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case c := <-ch:
		return c, nil
	case <-time.After(10 * time.Second):
		return nil, errors.New("device did not answer")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func deviceProxy(id string) *httputil.ReverseProxy {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if p := hub.proxies[id]; p != nil {
		return p
	}
	p := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: "device-" + id})
			m := idRe.FindStringSubmatch(pr.In.URL.Path)
			rest := "/"
			if m != nil && m[2] != "" {
				rest = m[2]
			}
			pr.Out.URL.Path, pr.Out.URL.RawPath = rest, ""
			pr.Out.Host = pr.In.Host // keep Origin/Host consistent for the device's WebSocket check
			pr.SetXForwarded()
			pr.Out.Header.Del("Cookie") // the hub cookie is ours, not the device's
			pr.Out.Header.Del("X-KD-Hub-User")
			pr.Out.Header.Del("X-KD-Hub-Perm")
			if id, ok := pr.In.Context().Value(hubIdentKey).(*hubIdentity); ok {
				pr.Out.Header.Set("X-KD-Hub-User", id.name)
				pr.Out.Header.Set("X-KD-Hub-Perm", id.perm)
			}
		},
		Transport: &http.Transport{
			DialContext:         func(ctx context.Context, _, _ string) (net.Conn, error) { return dialDevice(ctx, id) },
			MaxIdleConnsPerHost: 6, IdleConnTimeout: 2 * time.Minute, ResponseHeaderTimeout: 30 * time.Second,
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "KadduDesk hub: "+err.Error(), http.StatusBadGateway)
		},
	}
	hub.proxies[id] = p
	return p
}

func hubDeviceProxy(w http.ResponseWriter, r *http.Request) {
	m := idRe.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	u, _ := currentUser(r)
	if u == nil || u.MustChange || needs2FASetup(u) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	id := m[1]
	perm := devicePerm(u, id)
	if perm == "" {
		http.Error(w, "You don't have access to this device", http.StatusForbidden)
		return
	}
	if m[2] == "/ws" {
		go audit("device-session", u.Username, clientIP(r), prettyID(id)+" ("+perm+")")
	}
	r = r.WithContext(context.WithValue(r.Context(), hubIdentKey, &hubIdentity{name: u.Username, perm: perm}))
	if m[2] == "" {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
		return
	}
	if id == deviceID() { // this PC itself: serve the local app in-process
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = m[2], ""
		r2.Header.Del("Cookie")
		r2.Header.Del("X-KD-Hub-User")
		r2.Header.Del("X-KD-Hub-Perm")
		webMux.ServeHTTP(w, r2)
		return
	}
	deviceProxy(id).ServeHTTP(w, r)
}

// ------------------------------------------------------------ Wake-on-LAN (sent from the hub's network)

func sendWOL(mac, bcast string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return errors.New("bad MAC")
	}
	pkt := make([]byte, 6, 102)
	for i := range pkt {
		pkt[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		pkt = append(pkt, hw...)
	}
	targets := []string{"255.255.255.255"}
	if bcast != "" {
		targets = append(targets, bcast)
	}
	for _, m := range macAddrs() {
		targets = append(targets, m.Bcast)
	}
	var lastErr error
	for _, t := range targets {
		for _, port := range []int{9, 7} {
			c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP(t), Port: port})
			if err != nil {
				lastErr = err
				continue
			}
			c.Write(pkt)
			c.Close()
		}
	}
	return lastErr
}
