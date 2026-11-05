package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrustedDevice is a client that logged in with "remember this device".
type TrustedDevice struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"lastSeen"`
	LastIP    string    `json:"lastIP"`
}

// Config is persisted to %APPDATA%\KadduDesk\server.json
type Config struct {
	Name            string          `json:"name"`
	Port            int             `json:"port"`
	AdminPort       int             `json:"adminPort"`
	PinSalt         string          `json:"pinSalt"`
	PinHash         string          `json:"pinHash"`
	PinPlain        string          `json:"pinPlain"` // kept so the owner can read it in the local admin panel
	RequireApproval bool            `json:"requireApproval"`
	AllowControl    bool            `json:"allowControl"`
	AllowFiles      bool            `json:"allowFiles"`
	AllowAudio      bool            `json:"allowAudio"`
	AllowClipboard  bool            `json:"allowClipboard"`
	AllowPower      bool            `json:"allowPower"`
	DefaultFPS      int             `json:"defaultFps"`
	DefaultBitrate  int             `json:"defaultBitrateKbps"`
	Encoder         string          `json:"encoder"` // auto | h264_nvenc | h264_qsv | h264_amf | libx264
	UploadDir       string          `json:"uploadDir"`
	Devices         []TrustedDevice `json:"devices"`
	FirewallDone    bool            `json:"firewallDone"`
	Paused          bool            `json:"paused"`

	// hub (this PC relays for other devices)
	HubEnabled   bool            `json:"hubEnabled"`
	HubPort      int             `json:"hubPort"`
	HubPassSalt  string          `json:"hubPassSalt"`
	HubPassHash  string          `json:"hubPassHash"`
	HubPassPlain string          `json:"hubPassPlain"`
	HubHostKey   string          `json:"hubHostKey"`
	HubTokens    []TrustedDevice `json:"hubTokens"`
	HubKnown     []HubDevRec     `json:"hubKnown"`

	// joining a hub (this PC is reachable through it)
	HubAddr string `json:"hubAddr"`
	HubKey  string `json:"hubKey"`
	HubFP   string `json:"hubFp"`
}

var (
	cfgMu   sync.RWMutex
	cfg     Config
	dataDir string
)

func cfgPath() string { return filepath.Join(dataDir, "server.json") }

func loadConfig() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	host, _ := os.Hostname()
	cfg = Config{
		Name:           host,
		Port:           47800,
		AdminPort:      47801,
		AllowControl:   true,
		AllowFiles:     true,
		AllowAudio:     true,
		AllowClipboard: true,
		AllowPower:     true,
		DefaultFPS:     60,
		DefaultBitrate: 20000,
		HubPort:        47820,
		Encoder:        "auto",
		UploadDir:      "", // empty = <signed-in user>\Downloads\KadduDesk (see uploadDir)
	}
	if b, err := os.ReadFile(cfgPath()); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.PinHash == "" {
		setPinLocked(randomPin(6))
	}
	saveConfigLocked()
}

func saveConfigLocked() {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := cfgPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, cfgPath())
	}
}

func saveConfig() {
	cfgMu.Lock()
	saveConfigLocked()
	cfgMu.Unlock()
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	c := cfg
	c.Devices = append([]TrustedDevice(nil), cfg.Devices...)
	c.HubKnown = append([]HubDevRec(nil), cfg.HubKnown...)
	return c
}

func randomPin(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		s += v.String()
	}
	return s
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPin(pin, salt string) string {
	k, _ := pbkdf2.Key(sha256.New, pin, []byte(salt), 120000, 32)
	return hex.EncodeToString(k)
}

func setPinLocked(pin string) {
	cfg.PinSalt = randomHex(16)
	cfg.PinHash = hashPin(pin, cfg.PinSalt)
	cfg.PinPlain = pin
}

func setPin(pin string) error {
	if len(pin) < 4 || len(pin) > 32 {
		return fmt.Errorf("PIN must be 4-32 characters")
	}
	cfgMu.Lock()
	setPinLocked(pin)
	saveConfigLocked()
	cfgMu.Unlock()
	return nil
}

func checkPin(pin string) bool {
	cfgMu.RLock()
	salt, h := cfg.PinSalt, cfg.PinHash
	cfgMu.RUnlock()
	got := hashPin(pin, salt)
	return subtle.ConstantTimeCompare([]byte(got), []byte(h)) == 1
}

func tokenHash(tok string) string {
	s := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(s[:])
}

// addDevice creates a long-lived token for a remembered device.
func addDevice(name, ip string) (id, token string) {
	token = randomHex(32)
	id = randomHex(6)
	cfgMu.Lock()
	cfg.Devices = append(cfg.Devices, TrustedDevice{
		ID: id, Name: name, TokenHash: tokenHash(token),
		Created: time.Now(), LastSeen: time.Now(), LastIP: ip,
	})
	saveConfigLocked()
	cfgMu.Unlock()
	return
}

func removeDevice(id string) {
	cfgMu.Lock()
	out := cfg.Devices[:0]
	for _, d := range cfg.Devices {
		if d.ID != id {
			out = append(out, d)
		}
	}
	cfg.Devices = out
	saveConfigLocked()
	cfgMu.Unlock()
	kickDevice(id)
}

// --- short-lived session tokens (login without "remember") ---

type sessTok struct {
	exp      time.Time
	deviceID string
	name     string
}

var (
	sessMu   sync.Mutex
	sessToks = map[string]sessTok{}
)

func newSessionToken(name string) string {
	t := randomHex(32)
	sessMu.Lock()
	sessToks[tokenHash(t)] = sessTok{exp: time.Now().Add(12 * time.Hour), name: name}
	sessMu.Unlock()
	return t
}

// validateToken returns (deviceID, deviceName, ok).
func validateToken(tok, ip string) (string, string, bool) {
	if tok == "" {
		return "", "", false
	}
	h := tokenHash(tok)
	sessMu.Lock()
	if s, ok := sessToks[h]; ok {
		sessMu.Unlock()
		if time.Now().Before(s.exp) {
			return "", s.name, true
		}
		return "", "", false
	}
	sessMu.Unlock()
	cfgMu.Lock()
	defer cfgMu.Unlock()
	for i := range cfg.Devices {
		if subtle.ConstantTimeCompare([]byte(cfg.Devices[i].TokenHash), []byte(h)) == 1 {
			cfg.Devices[i].LastSeen = time.Now()
			cfg.Devices[i].LastIP = ip
			saveConfigLocked()
			return cfg.Devices[i].ID, cfg.Devices[i].Name, true
		}
	}
	return "", "", false
}
