package main

// Host side of the hub: this PC keeps an outbound control connection to the hub. When a
// client opens this device through the hub, the hub asks us to "dial back"; we open a data
// WebSocket to the hub and serve our normal web app over it. Works behind any NAT/router.

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ------------------------------------------------------------ WebSocket ⇄ net.Conn

// wsNetConn carries a byte stream over binary WebSocket messages. Read deadlines are
// handled here (not on the socket): net/http sets an already-expired deadline to stop its
// background read when a handler hijacks the connection, and a gorilla WebSocket can't
// recover from a read timeout.
type wsNetConn struct {
	ws    *websocket.Conn
	wmu   sync.Mutex
	rch   chan []byte
	buf   []byte
	dmu   sync.Mutex
	dl    time.Time
	dlCh  chan struct{}
	addr  net.Addr
	once  sync.Once
	close chan struct{}
}

func newWSConn(ws *websocket.Conn) *wsNetConn {
	c := &wsNetConn{ws: ws, addr: ws.RemoteAddr(), rch: make(chan []byte, 64), dlCh: make(chan struct{}, 1), close: make(chan struct{})}
	go func() {
		defer close(c.rch)
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.BinaryMessage || len(data) == 0 {
				continue
			}
			select {
			case c.rch <- data:
			case <-c.close:
				return
			}
		}
	}()
	return c
}

func (c *wsNetConn) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		c.dmu.Lock()
		dl := c.dl
		c.dmu.Unlock()
		var tc <-chan time.Time
		var t *time.Timer
		if !dl.IsZero() {
			d := time.Until(dl)
			if d <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			t = time.NewTimer(d)
			tc = t.C
		}
		select {
		case b, ok := <-c.rch:
			if t != nil {
				t.Stop()
			}
			if !ok {
				return 0, io.EOF
			}
			c.buf = b
		case <-tc:
			return 0, os.ErrDeadlineExceeded
		case <-c.dlCh: // deadline changed: re-evaluate
			if t != nil {
				t.Stop()
			}
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

func (c *wsNetConn) Write(p []byte) (int, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsNetConn) Close() error {
	c.once.Do(func() { close(c.close) })
	return c.ws.Close()
}
func (c *wsNetConn) LocalAddr() net.Addr  { return c.ws.LocalAddr() }
func (c *wsNetConn) RemoteAddr() net.Addr { return c.addr }
func (c *wsNetConn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	return c.SetWriteDeadline(t)
}
func (c *wsNetConn) SetReadDeadline(t time.Time) error {
	c.dmu.Lock()
	c.dl = t
	c.dmu.Unlock()
	select {
	case c.dlCh <- struct{}{}:
	default:
	}
	return nil
}
func (c *wsNetConn) SetWriteDeadline(t time.Time) error {
	if !t.IsZero() && time.Until(t) <= 0 {
		return nil // never poison the socket with an already-expired write deadline
	}
	return c.ws.SetWriteDeadline(t)
}

// tunnelListener feeds hub data connections into an http.Server.
type tunnelListener struct {
	ch     chan net.Conn
	closed chan struct{}
}

func (l *tunnelListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *tunnelListener) Close() error   { return nil }
func (l *tunnelListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

type ctxKey int

const viaTunnelKey ctxKey = 1

var tunnel = &tunnelListener{ch: make(chan net.Conn, 16), closed: make(chan struct{})}

// startTunnelServer serves the normal web app on connections that arrive through the hub.
func startTunnelServer(h http.Handler) {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 20 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, viaTunnelKey, true)
		}}
	go srv.Serve(tunnel)
}

// ------------------------------------------------------------ device ID

// deviceID is a stable 9-digit ID derived from this PC's certificate, AnyDesk-style.
func deviceID() string {
	s := sha256.Sum256([]byte("kaddudesk-id:" + certFingerprint))
	n := new(big.Int).SetBytes(s[:8])
	return fmt.Sprintf("%09d", new(big.Int).Mod(n, big.NewInt(1_000_000_000)).Int64())
}

func prettyID(id string) string {
	if len(id) != 9 {
		return id
	}
	return id[0:3] + " " + id[3:6] + " " + id[6:9]
}

// ------------------------------------------------------------ hub client (host side)

type hubLinkState struct {
	mu        sync.Mutex
	connected bool
	err       string
	since     time.Time
	kick      chan struct{}
}

var hubLink = &hubLinkState{kick: make(chan struct{}, 1)}

func (h *hubLinkState) set(ok bool, err string) {
	h.mu.Lock()
	h.connected, h.err = ok, err
	if ok {
		h.since = time.Now()
	}
	h.mu.Unlock()
}

func (h *hubLinkState) status() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return map[string]any{"connected": h.connected, "error": h.err, "since": h.since}
}

// reconnectHub makes the hub client re-read its settings now.
func reconnectHub() {
	select {
	case hubLink.kick <- struct{}{}:
	default:
	}
}

func pinnedTLS(fp string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // identity is checked by fingerprint pin instead of a CA
		MinVersion:         tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no certificate")
			}
			s := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if fp != "" && !strings.EqualFold(hex.EncodeToString(s[:]), fp) {
				return errors.New("hub certificate does not match the trusted fingerprint")
			}
			return nil
		},
	}
}

func hubDialer(fp string) *websocket.Dialer {
	return &websocket.Dialer{TLSClientConfig: pinnedTLS(fp), HandshakeTimeout: 8 * time.Second,
		ReadBufferSize: 64 << 10, WriteBufferSize: 256 << 10}
}

func hubURL(addr, path string) string {
	if !strings.Contains(addr, ":") || strings.HasSuffix(addr, "]") {
		addr += ":47820"
	}
	return "wss://" + addr + path
}

// probeHubFingerprint connects once to read the hub's certificate fingerprint (for TOFU).
func probeHubFingerprint(addr string) (string, string, error) {
	if !strings.Contains(addr, ":") {
		addr += ":47820"
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	cs := conn.ConnectionState()
	s := sha256.Sum256(cs.PeerCertificates[0].Raw)
	fp := strings.ToUpper(hex.EncodeToString(s[:]))
	// ask its name
	cl := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: pinnedTLS(fp)}}
	name := ""
	if resp, err := cl.Get("https://" + addr + "/api/info"); err == nil {
		var j map[string]any
		json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&j)
		resp.Body.Close()
		if j["app"] != "kaddudesk-hub" {
			return fp, "", errors.New("that address is not a KadduDesk hub")
		}
		name, _ = j["name"].(string)
	}
	return fp, name, nil
}

func hubClientLoop() {
	backoff := time.Second
	for {
		c := getConfig()
		if c.HubAddr == "" || c.HubKey == "" {
			hubLink.set(false, "")
			<-hubLink.kick
			continue
		}
		err := hubSession(c)
		if err != nil {
			hubLink.set(false, err.Error())
			logf("hub: %v", err)
		}
		select {
		case <-hubLink.kick:
			backoff = time.Second
		case <-time.After(backoff):
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

func hubSession(c Config) error {
	hdr := http.Header{"X-KD-Hub-Key": {c.HubKey}}
	ws, resp, err := hubDialer(c.HubFP).Dial(hubURL(c.HubAddr, "/hub/host"), hdr)
	if err != nil {
		if resp != nil && resp.StatusCode == 403 {
			return errors.New("the hub rejected the host key")
		}
		return fmt.Errorf("can't reach hub %s: %v", c.HubAddr, err)
	}
	defer ws.Close()
	ws.WriteJSON(map[string]any{"t": "hello", "id": deviceID(), "name": c.Name, "os": platformName(),
		"version": version, "fp": certFingerprint, "macs": macAddrs()})
	hubLink.set(true, "")
	logf("hub: connected to %s as %s", c.HubAddr, prettyID(deviceID()))
	stop := make(chan struct{})
	defer close(stop)
	go func() { // keepalive + react to settings changes
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-hubLink.kick:
				ws.Close()
				reconnectHub() // let the outer loop restart immediately
				return
			case <-t.C:
				ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			}
		}
	}()
	ws.SetReadLimit(1 << 16)
	for {
		ws.SetReadDeadline(time.Now().Add(70 * time.Second))
		var m struct {
			T string `json:"t"`
			C string `json:"c"`
		}
		if err := ws.ReadJSON(&m); err != nil {
			return errors.New("connection to hub lost")
		}
		if m.T == "dial" && m.C != "" {
			go hubDialBack(c, m.C)
		}
	}
}

func hubDialBack(c Config, connID string) {
	hdr := http.Header{"X-KD-Hub-Key": {c.HubKey}}
	ws, _, err := hubDialer(c.HubFP).Dial(hubURL(c.HubAddr, "/hub/data?c="+url.QueryEscape(connID)+"&id="+deviceID()), hdr)
	if err != nil {
		logf("hub: data connection failed: %v", err)
		return
	}
	if tc, ok := ws.UnderlyingConn().(*tls.Conn); ok {
		if nc, ok := tc.NetConn().(*net.TCPConn); ok {
			nc.SetNoDelay(true)
		}
	}
	select {
	case tunnel.ch <- newWSConn(ws):
	case <-time.After(5 * time.Second):
		ws.Close()
	}
}
