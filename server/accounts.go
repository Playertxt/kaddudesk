package main

// Hub accounts, sessions, 2FA, workspaces and the audit log.
//
// Security design:
//   - passwords: Argon2id (64 MiB, t=3, p=2), per-user random salt, constant-time compare
//   - 2FA: TOTP (RFC 6238, SHA-1, 6 digits, 30 s, ±1 step) with replay protection,
//     10 single-use recovery codes stored as SHA-256 hashes
//   - sessions: 256-bit random token in a __Host- HttpOnly Secure SameSite=Strict cookie,
//     only its SHA-256 is stored; idle timeout + absolute lifetime; revocable
//   - CSRF: SameSite=Strict + a required custom header on every state-changing request
//   - brute force: per-account lockout (exponential, up to 24 h) + per-IP lockout,
//     generic error messages (no username enumeration), dummy hash on unknown users
//   - the first (owner) account can only be created from the hub PC itself (localhost panel)

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

type HubUser struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Role        string    `json:"role"` // owner | admin | member
	PassHash    string    `json:"passHash"`
	MustChange  bool      `json:"mustChange"` // temporary password set by an admin
	TOTPSecret  string    `json:"totpSecret,omitempty"`
	TOTPOn      bool      `json:"totpOn"`
	TOTPLast    int64     `json:"totpLast"`
	Recovery    []string  `json:"recovery,omitempty"`
	Disabled    bool      `json:"disabled"`
	Created     time.Time `json:"created"`
	LastLogin   time.Time `json:"lastLogin"`
	Fails       int       `json:"fails"`
	LockedUntil time.Time `json:"lockedUntil"`
}

type HubSession struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	UserID   string    `json:"userId"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"lastSeen"`
	Expires  time.Time `json:"expires"`
	IP       string    `json:"ip"`
	UA       string    `json:"ua"`
	Remember bool      `json:"remember"`
}

type WSMember struct {
	UserID string `json:"userId"`
	Perm   string `json:"perm"` // control | view
}

type Workspace struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Devices []string   `json:"devices"`
	Members []WSMember `json:"members"`
	Created time.Time  `json:"created"`
}

type HubSecurity struct {
	Require2FA  bool `json:"require2fa"`
	IdleMinutes int  `json:"idleMinutes"`
	RememberDay int  `json:"rememberDays"`
}

type accountData struct {
	Users      []HubUser    `json:"users"`
	Sessions   []HubSession `json:"sessions"`
	Workspaces []Workspace  `json:"workspaces"`
	Security   HubSecurity  `json:"security"`
}

var (
	acctMu sync.Mutex
	acct   accountData
)

func acctPath() string  { return filepath.Join(dataDir, "hub_accounts.json") }
func auditPath() string { return filepath.Join(dataDir, "hub_audit.log") }

func loadAccounts() {
	acctMu.Lock()
	defer acctMu.Unlock()
	acct = accountData{Security: HubSecurity{Require2FA: true, IdleMinutes: 60, RememberDay: 30}}
	if b, err := os.ReadFile(acctPath()); err == nil {
		json.Unmarshal(b, &acct)
	}
	if acct.Security.IdleMinutes <= 0 {
		acct.Security.IdleMinutes = 60
	}
	if acct.Security.RememberDay <= 0 {
		acct.Security.RememberDay = 30
	}
}

func saveAccountsLocked() {
	b, _ := json.MarshalIndent(acct, "", " ")
	tmp := acctPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, acctPath())
	}
}

func hasAccounts() bool {
	acctMu.Lock()
	defer acctMu.Unlock()
	return len(acct.Users) > 0
}

func userByIDLocked(id string) *HubUser {
	for i := range acct.Users {
		if acct.Users[i].ID == id {
			return &acct.Users[i]
		}
	}
	return nil
}

func userByNameLocked(name string) *HubUser {
	for i := range acct.Users {
		if strings.EqualFold(acct.Users[i].Username, name) {
			return &acct.Users[i]
		}
	}
	return nil
}

// ------------------------------------------------------------ passwords (Argon2id)

const (
	argonTime    = 3
	argonMem     = 64 * 1024
	argonThreads = 2
	argonLen     = 32
)

func hashPassword(p string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	k := argon2.IDKey([]byte(p), salt, argonTime, argonMem, argonThreads, argonLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMem, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

func verifyPassword(p, enc string) bool {
	parts := strings.Split(enc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var th uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &th); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, t, m, th, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is verified against when the username doesn't exist, so timing doesn't reveal it.
var dummyHash = hashPassword("kaddudesk-dummy-" + randomHex(8))

var commonPasswords = map[string]bool{"password": true, "password1": true, "password123": true, "123456789": true,
	"1234567890": true, "qwertyuiop": true, "iloveyou": true, "admin12345": true, "letmein123": true,
	"welcome123": true, "1q2w3e4r5t": true, "qwerty123": true, "abc1234567": true, "kaddudesk": true}

func checkPasswordPolicy(username, p string) error {
	if len([]rune(p)) < 10 {
		return errors.New("Password must be at least 10 characters")
	}
	if len(p) > 200 {
		return errors.New("Password is too long")
	}
	lp := strings.ToLower(p)
	if commonPasswords[lp] || strings.Contains(lp, strings.ToLower(username)) && len(username) >= 3 {
		return errors.New("That password is too easy to guess")
	}
	kinds := 0
	var hasL, hasU, hasD, hasS bool
	for _, r := range p {
		switch {
		case unicode.IsLower(r):
			hasL = true
		case unicode.IsUpper(r):
			hasU = true
		case unicode.IsDigit(r):
			hasD = true
		default:
			hasS = true
		}
	}
	for _, b := range []bool{hasL, hasU, hasD, hasS} {
		if b {
			kinds++
		}
	}
	if kinds < 3 && len([]rune(p)) < 16 {
		return errors.New("Use at least 3 of: lowercase, UPPERCASE, digits, symbols (or 16+ characters)")
	}
	return nil
}

func validUsername(u string) bool {
	if len(u) < 3 || len(u) > 32 {
		return false
	}
	for _, r := range u {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == '@') {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------ TOTP

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return b32.EncodeToString(b)
}

func totpAt(secret string, counter int64) string {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", v)
}

// checkTOTP returns the matched counter (to block replay) or -1.
func checkTOTP(secret, code string, last int64) int64 {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return -1
	}
	now := time.Now().Unix() / 30
	for _, c := range []int64{now, now - 1, now + 1} {
		if c > last && subtle.ConstantTimeCompare([]byte(totpAt(secret, c)), []byte(code)) == 1 {
			return c
		}
	}
	return -1
}

func newRecoveryCodes() (plain, hashed []string) {
	const alpha = "abcdefghjkmnpqrstuvwxyz23456789"
	for i := 0; i < 10; i++ {
		b := make([]byte, 10)
		rand.Read(b)
		var s strings.Builder
		for j, x := range b {
			if j == 5 {
				s.WriteByte('-')
			}
			s.WriteByte(alpha[int(x)%len(alpha)])
		}
		plain = append(plain, s.String())
		hashed = append(hashed, sha256hex(s.String()))
	}
	return
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// useRecoveryLocked consumes a recovery code.
func useRecoveryLocked(u *HubUser, code string) bool {
	h := sha256hex(strings.ToLower(strings.TrimSpace(code)))
	for i, r := range u.Recovery {
		if subtle.ConstantTimeCompare([]byte(r), []byte(h)) == 1 {
			u.Recovery = append(u.Recovery[:i], u.Recovery[i+1:]...)
			return true
		}
	}
	return false
}

// ------------------------------------------------------------ lockout

func lockUserLocked(u *HubUser) {
	u.Fails++
	if u.Fails >= 5 {
		d := 15 * time.Minute << min(u.Fails-5, 6) // 15 min … ~16 h
		if d > 24*time.Hour {
			d = 24 * time.Hour
		}
		u.LockedUntil = time.Now().Add(d)
	}
}

// ------------------------------------------------------------ sessions

const sessCookie = "__Host-kdhub"

func newSessionLocked(u *HubUser, r *http.Request, remember bool) (string, *HubSession) {
	tok := randomHex(32)
	life := time.Duration(acct.Security.IdleMinutes) * time.Minute
	if remember {
		life = time.Duration(acct.Security.RememberDay) * 24 * time.Hour
	}
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	s := HubSession{ID: randomHex(6), Hash: sha256hex(tok), UserID: u.ID, Created: time.Now(), LastSeen: time.Now(),
		Expires: time.Now().Add(life), IP: clientIP(r), UA: ua, Remember: remember}
	acct.Sessions = append(acct.Sessions, s)
	pruneSessionsLocked()
	saveAccountsLocked()
	return tok, &acct.Sessions[len(acct.Sessions)-1]
}

func pruneSessionsLocked() {
	out := acct.Sessions[:0]
	for _, s := range acct.Sessions {
		if time.Now().Before(s.Expires) {
			out = append(out, s)
		}
	}
	acct.Sessions = out
}

func setSessionCookie(w http.ResponseWriter, tok string, s *HubSession) {
	ck := &http.Cookie{Name: sessCookie, Value: tok, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
	if s.Remember {
		ck.Expires = s.Expires
	}
	http.SetCookie(w, ck)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessCookie, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

var lastSeenSave time.Time

// currentUser resolves the request's session (sliding idle timeout). Returns copies.
func currentUser(r *http.Request) (*HubUser, *HubSession) {
	ck, err := r.Cookie(sessCookie)
	if err != nil || len(ck.Value) != 64 {
		return nil, nil
	}
	h := sha256hex(ck.Value)
	acctMu.Lock()
	defer acctMu.Unlock()
	for i := range acct.Sessions {
		s := &acct.Sessions[i]
		if subtle.ConstantTimeCompare([]byte(s.Hash), []byte(h)) != 1 {
			continue
		}
		if time.Now().After(s.Expires) {
			return nil, nil
		}
		u := userByIDLocked(s.UserID)
		if u == nil || u.Disabled {
			return nil, nil
		}
		s.LastSeen = time.Now()
		if !s.Remember { // idle timeout slides
			s.Expires = time.Now().Add(time.Duration(acct.Security.IdleMinutes) * time.Minute)
		}
		if time.Since(lastSeenSave) > time.Minute {
			lastSeenSave = time.Now()
			saveAccountsLocked()
		}
		uc, sc := *u, *s
		return &uc, &sc
	}
	return nil, nil
}

func revokeSessionsLocked(userID, keepID string) {
	out := acct.Sessions[:0]
	for _, s := range acct.Sessions {
		if s.UserID != userID || s.ID == keepID {
			out = append(out, s)
		}
	}
	acct.Sessions = out
}

// needs2FASetup: policy requires 2FA but the user hasn't enabled it yet.
func needs2FASetup(u *HubUser) bool {
	acctMu.Lock()
	req := acct.Security.Require2FA
	acctMu.Unlock()
	return req && !u.TOTPOn
}

// short-lived "password OK, now give me your code" tickets
var (
	mfaMu      sync.Mutex
	mfaTickets = map[string]struct {
		user     string
		exp      time.Time
		remember bool
		tries    int
	}{}
)

// ------------------------------------------------------------ authorization

func isAdminRole(u *HubUser) bool { return u.Role == "owner" || u.Role == "admin" }

// devicePerm: "control", "view" or "" (no access).
func devicePerm(u *HubUser, devID string) string {
	if isAdminRole(u) {
		return "control"
	}
	acctMu.Lock()
	defer acctMu.Unlock()
	perm := ""
	for _, w := range acct.Workspaces {
		has := false
		for _, d := range w.Devices {
			if d == devID {
				has = true
			}
		}
		if !has {
			continue
		}
		for _, m := range w.Members {
			if m.UserID == u.ID {
				if m.Perm == "control" {
					return "control"
				}
				perm = "view"
			}
		}
	}
	return perm
}

func visibleWorkspaces(u *HubUser) []Workspace {
	acctMu.Lock()
	defer acctMu.Unlock()
	var out []Workspace
	for _, w := range acct.Workspaces {
		if isAdminRole(u) {
			out = append(out, w)
			continue
		}
		for _, m := range w.Members {
			if m.UserID == u.ID {
				out = append(out, w)
				break
			}
		}
	}
	return out
}

// ------------------------------------------------------------ audit log

type auditEntry struct {
	T      time.Time `json:"t"`
	Event  string    `json:"event"`
	User   string    `json:"user,omitempty"`
	IP     string    `json:"ip,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

var (
	auditMu  sync.Mutex
	auditBuf []auditEntry
)

func audit(event, user, ip, detail string) {
	e := auditEntry{T: time.Now(), Event: event, User: user, IP: ip, Detail: detail}
	auditMu.Lock()
	defer auditMu.Unlock()
	auditBuf = append(auditBuf, e)
	if len(auditBuf) > 1000 {
		auditBuf = auditBuf[len(auditBuf)-1000:]
	}
	if f, err := os.OpenFile(auditPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
		f.Close()
	}
	logf("audit: %s %s %s %s", event, user, ip, detail)
}

func loadAudit() {
	b, err := os.ReadFile(auditPath())
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > 1000 {
		lines = lines[len(lines)-1000:]
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	for _, l := range lines {
		var e auditEntry
		if json.Unmarshal([]byte(l), &e) == nil {
			auditBuf = append(auditBuf, e)
		}
	}
}

func auditTail(n int) []auditEntry {
	auditMu.Lock()
	defer auditMu.Unlock()
	out := make([]auditEntry, 0, n)
	for i := len(auditBuf) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, auditBuf[i])
	}
	return out
}

// ------------------------------------------------------------ owner bootstrap (localhost panel only)

func createOwner(username, password string) error {
	acctMu.Lock()
	defer acctMu.Unlock()
	if len(acct.Users) > 0 {
		return errors.New("an owner account already exists")
	}
	if !validUsername(username) {
		return errors.New("Username: 3-32 letters, digits, . _ - @")
	}
	if err := checkPasswordPolicy(username, password); err != nil {
		return err
	}
	acct.Users = append(acct.Users, HubUser{ID: randomHex(6), Username: username, Role: "owner",
		PassHash: hashPassword(password), Created: time.Now()})
	saveAccountsLocked()
	audit("owner-created", username, "local", "")
	return nil
}

// recoverOwner resets a user's 2FA and password from the hub PC (if you lost your phone).
func recoverAccount(username, password string) error {
	acctMu.Lock()
	defer acctMu.Unlock()
	u := userByNameLocked(username)
	if u == nil {
		return errors.New("no such user")
	}
	if err := checkPasswordPolicy(username, password); err != nil {
		return err
	}
	u.PassHash, u.TOTPOn, u.TOTPSecret, u.Recovery = hashPassword(password), false, "", nil
	u.Fails, u.LockedUntil, u.Disabled = 0, time.Time{}, false
	revokeSessionsLocked(u.ID, "")
	saveAccountsLocked()
	audit("account-recovered", username, "local", "password and 2FA reset from the hub PC")
	return nil
}
