package main

// "Connect from anywhere": an embedded Tailscale node (tsnet). After a one-time sign-in,
// every KadduDesk device on the same account can reach every other one from any network
// in the world — through CGNAT, mobile data and firewalls — with WireGuard end-to-end
// encryption, direct peer-to-peer when possible and Tailscale's relays otherwise.
// No router setup and no server of our own.

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

type meshNode struct {
	mu      sync.Mutex
	srv     *tsnet.Server
	lc      *local.Client
	host    bool // also accept incoming KadduDesk connections
	err     string
	lnDev   net.Listener
	lnHub   net.Listener
	started time.Time
}

var mesh = &meshNode{}

var hostSlug = regexp.MustCompile(`[^a-z0-9-]+`)

func meshHostname(app bool) string {
	n := strings.ToLower(getConfig().Name)
	if app {
		n = strings.ToLower(appCfgName())
	}
	n = strings.Trim(hostSlug.ReplaceAllString(n, "-"), "-")
	if len(n) > 30 {
		n = n[:30]
	}
	if n == "" {
		n = "device"
	}
	if app {
		return "kd-app-" + n
	}
	return "kd-" + n + "-" + deviceID()[:4]
}

// startMesh starts the node. host=true also serves this device (and its hub) on the tailnet.
func startMesh(app bool) error {
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	if mesh.srv != nil {
		return nil
	}
	dir := filepath.Join(dataDir, "mesh")
	if app {
		dir = filepath.Join(appDir(), "mesh")
	}
	s := &tsnet.Server{
		Hostname: meshHostname(app),
		Dir:      dir,
		Logf:     func(string, ...any) {},
		UserLogf: func(f string, a ...any) { logf("mesh: "+f, a...) },
	}
	if err := s.Start(); err != nil {
		mesh.err = err.Error()
		return err
	}
	lc, err := s.LocalClient()
	if err != nil {
		s.Close()
		mesh.err = err.Error()
		return err
	}
	mesh.srv, mesh.lc, mesh.host, mesh.err, mesh.started = s, lc, !app, "", time.Now()
	if !app {
		cert := serverCert
		tcfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		if ln, err := s.Listen("tcp", ":"+itoa(getConfig().Port)); err == nil {
			mesh.lnDev = ln
			go (&http.Server{Handler: webMux, ReadHeaderTimeout: 20 * time.Second}).Serve(tls.NewListener(ln, tcfg))
		}
		if ln, err := s.Listen("tcp", ":"+itoa(getConfig().HubPort)); err == nil {
			mesh.lnHub = ln
			go (&http.Server{ReadHeaderTimeout: 20 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if h := currentHubMux(); h != nil {
					h.ServeHTTP(w, r)
					return
				}
				http.NotFound(w, r)
			})}).Serve(tls.NewListener(ln, tcfg))
		}
	}
	logf("mesh: node %s started", s.Hostname)
	return nil
}

func stopMesh() {
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	if mesh.srv != nil {
		mesh.srv.Close()
	}
	mesh.srv, mesh.lc, mesh.lnDev, mesh.lnHub = nil, nil, nil, nil
}

func meshRunning() bool {
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	return mesh.srv != nil
}

func meshClient() *local.Client {
	mesh.mu.Lock()
	defer mesh.mu.Unlock()
	return mesh.lc
}

// meshStatusMap describes the node for the app's "Anywhere" screen.
func meshStatusMap() map[string]any {
	lc := meshClient()
	if lc == nil {
		mesh.mu.Lock()
		e := mesh.err
		mesh.mu.Unlock()
		return map[string]any{"running": false, "state": "Off", "error": e}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return map[string]any{"running": true, "state": "Starting", "error": err.Error()}
	}
	out := map[string]any{"running": true, "state": st.BackendState, "authURL": st.AuthURL}
	if st.Self != nil {
		var ips []string
		for _, ip := range st.Self.TailscaleIPs {
			if ip.Is4() {
				ips = append(ips, ip.String())
			}
		}
		out["ips"] = ips
		out["dnsName"] = strings.TrimSuffix(st.Self.DNSName, ".")
		if u, ok := st.User[st.Self.UserID]; ok {
			out["user"] = u.LoginName
		}
	}
	if st.CurrentTailnet != nil {
		out["tailnet"] = st.CurrentTailnet.Name
	}
	return out
}

// meshLogin returns the sign-in URL (opened in the system browser once).
func meshLogin() (string, error) {
	lc := meshClient()
	if lc == nil {
		return "", errors.New("turn on Connect from anywhere first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := lc.StatusWithoutPeers(ctx)
	if err == nil && st.BackendState == "Running" {
		return "", nil
	}
	if err == nil && st.AuthURL != "" {
		return st.AuthURL, nil
	}
	if err := lc.StartLoginInteractive(ctx); err != nil {
		return "", err
	}
	for ctx.Err() == nil {
		st, err := lc.StatusWithoutPeers(ctx)
		if err == nil && st.AuthURL != "" {
			return st.AuthURL, nil
		}
		if err == nil && st.BackendState == "Running" {
			return "", nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", errors.New("Tailscale didn't answer — check the internet connection")
}

func meshLogout() error {
	lc := meshClient()
	if lc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return lc.Logout(ctx)
}

type meshPeer struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	IP     string `json:"ip"`
	OS     string `json:"os"`
	Online bool   `json:"online"`
	App    bool   `json:"app"` // a KadduDesk app (can't be controlled), not a shared device
}

// meshPeersList lists the KadduDesk devices on the same account.
func meshPeersList() []meshPeer {
	lc := meshClient()
	if lc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	st, err := lc.Status(ctx)
	if err != nil {
		return nil
	}
	var out []meshPeer
	for _, p := range st.Peer {
		h := strings.ToLower(p.HostName)
		if !strings.HasPrefix(h, "kd-") {
			continue
		}
		ip := ""
		for _, a := range p.TailscaleIPs {
			if a.Is4() {
				ip = a.String()
			}
		}
		if ip == "" {
			continue
		}
		app := strings.HasPrefix(h, "kd-app-")
		name := strings.TrimPrefix(h, "kd-app-")
		if !app {
			name = strings.TrimPrefix(h, "kd-")
			if i := strings.LastIndex(name, "-"); i > 0 {
				name = name[:i]
			}
		}
		out = append(out, meshPeer{Name: name, Host: p.HostName, IP: ip, OS: osName(p.OS), Online: p.Online, App: app})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func osName(s string) string {
	switch strings.ToLower(s) {
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "android":
		return "Android"
	case "macos", "darwin":
		return "macOS"
	case "ios":
		return "iOS"
	}
	return s
}

func meshDialLocal(ctx context.Context, addr string) (net.Conn, error) {
	mesh.mu.Lock()
	s := mesh.srv
	mesh.mu.Unlock()
	if s == nil {
		return nil, errors.New("Connect from anywhere is off")
	}
	return s.Dial(ctx, "tcp", addr)
}

// ------------------------------------------------------------ admin API (used by the app on the same PC)

func registerMeshAdmin(api func(string, func(http.ResponseWriter, *http.Request))) {
	api("/admin/mesh/status", func(w http.ResponseWriter, r *http.Request) {
		m := meshStatusMap()
		m["enabled"] = getConfig().MeshEnabled
		writeJSON(w, 200, m)
	})
	api("/admin/mesh/enable", func(w http.ResponseWriter, r *http.Request) {
		on := r.URL.Query().Get("on") == "1"
		cfgMu.Lock()
		cfg.MeshEnabled = on
		saveConfigLocked()
		cfgMu.Unlock()
		if on {
			if err := startMesh(false); err != nil {
				writeJSON(w, 500, map[string]any{"error": err.Error()})
				return
			}
		} else {
			stopMesh()
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/mesh/login", func(w http.ResponseWriter, r *http.Request) {
		u, err := meshLogin()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"url": u})
	})
	api("/admin/mesh/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := meshLogout(); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	api("/admin/mesh/peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"peers": meshPeersList()})
	})
	// dial: the app on this PC uses this PC's node instead of signing in a second time
	api("/admin/mesh/dial", func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("addr")
		if h, _, err := net.SplitHostPort(addr); err != nil || !strings.HasPrefix(h, "100.") {
			http.Error(w, "bad address", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		up, err := meshDialLocal(ctx, addr)
		cancel()
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		ws, err := hubUpgrader.Upgrade(w, r, nil)
		if err != nil {
			up.Close()
			return
		}
		c := newWSConn(ws)
		go func() { io.Copy(up, c); up.Close() }()
		io.Copy(c, up)
		c.Close()
	})
}
