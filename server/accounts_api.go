package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

type apiOpt struct {
	auth  bool // must be signed in
	full  bool // and done with mandatory setup (2FA enrolled, temp password changed)
	admin bool // and owner/admin
}

func secHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Strict-Transport-Security", "max-age=31536000")
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(v)
}

// hubAPI wraps every hub API handler with CSRF, session and role checks.
func hubAPI(o apiOpt, fn func(w http.ResponseWriter, r *http.Request, u *HubUser, s *HubSession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secHeaders(w)
		if r.Method != http.MethodGet && r.Header.Get("X-KD-CSRF") != "1" {
			writeJSON(w, 403, map[string]any{"error": "CSRF check failed"})
			return
		}
		var u *HubUser
		var s *HubSession
		if o.auth {
			u, s = currentUser(r)
			if u == nil {
				writeJSON(w, 401, map[string]any{"error": "Please sign in"})
				return
			}
			if o.full && (u.MustChange || needs2FASetup(u)) {
				writeJSON(w, 403, map[string]any{"error": "Finish securing your account first", "setup": true})
				return
			}
			if o.admin && !isAdminRole(u) {
				writeJSON(w, 403, map[string]any{"error": "Only the owner or an admin can do that"})
				return
			}
		}
		fn(w, r, u, s)
	}
}

func userJSON(u *HubUser) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "totp": u.TOTPOn, "disabled": u.Disabled,
		"created": u.Created, "lastLogin": u.LastLogin, "locked": time.Now().Before(u.LockedUntil), "mustChange": u.MustChange,
		"recoveryLeft": len(u.Recovery)}
}

func registerAccountAPI(mux *http.ServeMux) {
	// ---------------- sign in
	mux.HandleFunc("/hub/api/auth/state", hubAPI(apiOpt{}, func(w http.ResponseWriter, r *http.Request, _ *HubUser, _ *HubSession) {
		writeJSON(w, 200, map[string]any{"hasAccounts": hasAccounts(), "name": getConfig().Name})
	}))
	mux.HandleFunc("/hub/api/auth/login", hubAPI(apiOpt{}, func(w http.ResponseWriter, r *http.Request, _ *HubUser, _ *HubSession) {
		ip := clientIP(r)
		if d := lockedOut(ip); d > 0 {
			writeJSON(w, 429, map[string]any{"error": "Too many attempts from your network. Try again later."})
			return
		}
		var req struct {
			Username, Password string
			Remember           bool
		}
		if readJSON(r, &req) != nil {
			writeJSON(w, 400, map[string]any{"error": "bad request"})
			return
		}
		acctMu.Lock()
		u := userByNameLocked(strings.TrimSpace(req.Username))
		if u == nil {
			acctMu.Unlock()
			verifyPassword(req.Password, dummyHash) // same cost as a real check
			noteFail(ip)
			audit("login-failed", req.Username, ip, "unknown user")
			writeJSON(w, 401, map[string]any{"error": "Wrong username or password"})
			return
		}
		if time.Now().Before(u.LockedUntil) {
			acctMu.Unlock()
			audit("login-blocked", u.Username, ip, "account locked")
			writeJSON(w, 429, map[string]any{"error": "This account is temporarily locked after too many attempts."})
			return
		}
		hash := u.PassHash
		acctMu.Unlock()
		ok := verifyPassword(req.Password, hash)
		acctMu.Lock()
		u = userByNameLocked(strings.TrimSpace(req.Username))
		if !ok || u == nil || u.Disabled {
			if u != nil {
				lockUserLocked(u)
				saveAccountsLocked()
			}
			acctMu.Unlock()
			noteFail(ip)
			audit("login-failed", req.Username, ip, "bad password or disabled")
			writeJSON(w, 401, map[string]any{"error": "Wrong username or password"})
			return
		}
		if u.TOTPOn { // second step
			t := randomHex(24)
			mfaMu.Lock()
			mfaTickets[sha256hex(t)] = struct {
				user     string
				exp      time.Time
				remember bool
				tries    int
			}{u.ID, time.Now().Add(5 * time.Minute), req.Remember, 0}
			mfaMu.Unlock()
			acctMu.Unlock()
			writeJSON(w, 200, map[string]any{"mfa": true, "ticket": t})
			return
		}
		u.Fails, u.LockedUntil, u.LastLogin = 0, time.Time{}, time.Now()
		tok, s := newSessionLocked(u, r, req.Remember)
		name := u.Username
		acctMu.Unlock()
		clearFail(ip)
		setSessionCookie(w, tok, s)
		audit("login", name, ip, "password")
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/hub/api/auth/mfa", hubAPI(apiOpt{}, func(w http.ResponseWriter, r *http.Request, _ *HubUser, _ *HubSession) {
		ip := clientIP(r)
		var req struct{ Ticket, Code string }
		if readJSON(r, &req) != nil {
			writeJSON(w, 400, map[string]any{"error": "bad request"})
			return
		}
		th := sha256hex(req.Ticket)
		mfaMu.Lock()
		t, ok := mfaTickets[th]
		if ok {
			t.tries++
			mfaTickets[th] = t
		}
		mfaMu.Unlock()
		if !ok || time.Now().After(t.exp) || t.tries > 5 {
			writeJSON(w, 401, map[string]any{"error": "Sign-in expired. Start again.", "restart": true})
			return
		}
		acctMu.Lock()
		u := userByIDLocked(t.user)
		if u == nil || u.Disabled || time.Now().Before(u.LockedUntil) {
			acctMu.Unlock()
			writeJSON(w, 401, map[string]any{"error": "Sign-in expired. Start again.", "restart": true})
			return
		}
		how := "totp"
		if c := checkTOTP(u.TOTPSecret, req.Code, u.TOTPLast); c >= 0 {
			u.TOTPLast = c
		} else if useRecoveryLocked(u, req.Code) {
			how = "recovery code"
		} else {
			lockUserLocked(u)
			saveAccountsLocked()
			name := u.Username
			acctMu.Unlock()
			noteFail(ip)
			audit("login-failed", name, ip, "wrong 2FA code")
			writeJSON(w, 401, map[string]any{"error": "Wrong code"})
			return
		}
		mfaMu.Lock()
		delete(mfaTickets, th)
		mfaMu.Unlock()
		u.Fails, u.LockedUntil, u.LastLogin = 0, time.Time{}, time.Now()
		tok, s := newSessionLocked(u, r, t.remember)
		name, left := u.Username, len(u.Recovery)
		acctMu.Unlock()
		clearFail(ip)
		setSessionCookie(w, tok, s)
		audit("login", name, ip, "password + "+how)
		writeJSON(w, 200, map[string]any{"ok": true, "recoveryLeft": left})
	}))
	mux.HandleFunc("/hub/api/auth/logout", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, s *HubSession) {
		acctMu.Lock()
		out := acct.Sessions[:0]
		for _, x := range acct.Sessions {
			if x.ID != s.ID {
				out = append(out, x)
			}
		}
		acct.Sessions = out
		saveAccountsLocked()
		acctMu.Unlock()
		clearSessionCookie(w)
		audit("logout", u.Username, clientIP(r), "")
		writeJSON(w, 200, map[string]any{"ok": true})
	}))

	// ---------------- my account
	mux.HandleFunc("/hub/api/me", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, s *HubSession) {
		acctMu.Lock()
		sec := acct.Security
		acctMu.Unlock()
		writeJSON(w, 200, map[string]any{"user": userJSON(u), "needs2fa": needs2FASetup(u), "mustChange": u.MustChange,
			"security": sec, "session": s.ID, "hub": getConfig().Name})
	}))
	mux.HandleFunc("/hub/api/me/password", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, s *HubSession) {
		var req struct{ Current, New string }
		readJSON(r, &req)
		if !verifyPassword(req.Current, u.PassHash) {
			audit("password-change-failed", u.Username, clientIP(r), "wrong current password")
			writeJSON(w, 400, map[string]any{"error": "Current password is wrong"})
			return
		}
		if err := checkPasswordPolicy(u.Username, req.New); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		if verifyPassword(req.New, u.PassHash) {
			writeJSON(w, 400, map[string]any{"error": "Choose a different password"})
			return
		}
		acctMu.Lock()
		if x := userByIDLocked(u.ID); x != nil {
			x.PassHash, x.MustChange = hashPassword(req.New), false
			revokeSessionsLocked(u.ID, s.ID) // sign out everywhere else
		}
		saveAccountsLocked()
		acctMu.Unlock()
		audit("password-changed", u.Username, clientIP(r), "other sessions signed out")
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	pending2FA := map[string]string{} // userID → secret being enrolled
	mux.HandleFunc("/hub/api/me/2fa/begin", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		if u.TOTPOn {
			writeJSON(w, 400, map[string]any{"error": "2FA is already on"})
			return
		}
		sec := newTOTPSecret()
		acctMu.Lock()
		pending2FA[u.ID] = sec
		acctMu.Unlock()
		label := url.PathEscape("KadduDesk (" + getConfig().Name + "):" + u.Username)
		uri := "otpauth://totp/" + label + "?secret=" + sec + "&issuer=KadduDesk&algorithm=SHA1&digits=6&period=30"
		png, _ := qrcode.Encode(uri, qrcode.Medium, 260)
		writeJSON(w, 200, map[string]any{"secret": sec, "uri": uri, "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
	}))
	mux.HandleFunc("/hub/api/me/2fa/enable", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		var req struct{ Code string }
		readJSON(r, &req)
		acctMu.Lock()
		defer acctMu.Unlock()
		sec := pending2FA[u.ID]
		x := userByIDLocked(u.ID)
		c := int64(-1)
		if sec != "" {
			c = checkTOTP(sec, req.Code, 0)
		}
		if x == nil || c < 0 {
			writeJSON(w, 400, map[string]any{"error": "That code doesn't match. Check your phone's clock and try the next code."})
			return
		}
		plain, hashed := newRecoveryCodes()
		x.TOTPSecret, x.TOTPOn, x.TOTPLast, x.Recovery = sec, true, c, hashed
		delete(pending2FA, u.ID)
		saveAccountsLocked()
		go audit("2fa-enabled", u.Username, clientIP(r), "")
		writeJSON(w, 200, map[string]any{"ok": true, "recovery": plain})
	}))
	mux.HandleFunc("/hub/api/me/2fa/disable", hubAPI(apiOpt{auth: true, full: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		var req struct{ Password, Code string }
		readJSON(r, &req)
		acctMu.Lock()
		defer acctMu.Unlock()
		if acct.Security.Require2FA {
			writeJSON(w, 400, map[string]any{"error": "2FA is required on this hub"})
			return
		}
		x := userByIDLocked(u.ID)
		if x == nil || !verifyPassword(req.Password, x.PassHash) || checkTOTP(x.TOTPSecret, req.Code, x.TOTPLast) < 0 {
			writeJSON(w, 400, map[string]any{"error": "Password or code is wrong"})
			return
		}
		x.TOTPOn, x.TOTPSecret, x.Recovery = false, "", nil
		saveAccountsLocked()
		go audit("2fa-disabled", u.Username, clientIP(r), "")
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/hub/api/me/2fa/recovery", hubAPI(apiOpt{auth: true, full: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		var req struct{ Password string }
		readJSON(r, &req)
		if !verifyPassword(req.Password, u.PassHash) || !u.TOTPOn {
			writeJSON(w, 400, map[string]any{"error": "Password is wrong"})
			return
		}
		plain, hashed := newRecoveryCodes()
		acctMu.Lock()
		if x := userByIDLocked(u.ID); x != nil {
			x.Recovery = hashed
		}
		saveAccountsLocked()
		acctMu.Unlock()
		audit("recovery-codes-regenerated", u.Username, clientIP(r), "")
		writeJSON(w, 200, map[string]any{"recovery": plain})
	}))
	mux.HandleFunc("/hub/api/me/sessions", hubAPI(apiOpt{auth: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, s *HubSession) {
		if r.Method == http.MethodPost { // revoke {id} or {all: true}
			var req struct {
				ID  string
				All bool
			}
			readJSON(r, &req)
			acctMu.Lock()
			if req.All {
				revokeSessionsLocked(u.ID, s.ID)
			} else {
				out := acct.Sessions[:0]
				for _, x := range acct.Sessions {
					if !(x.UserID == u.ID && x.ID == req.ID && x.ID != s.ID) {
						out = append(out, x)
					}
				}
				acct.Sessions = out
			}
			saveAccountsLocked()
			acctMu.Unlock()
			audit("sessions-revoked", u.Username, clientIP(r), "")
		}
		acctMu.Lock()
		var list []map[string]any
		for _, x := range acct.Sessions {
			if x.UserID == u.ID && time.Now().Before(x.Expires) {
				list = append(list, map[string]any{"id": x.ID, "ip": x.IP, "ua": x.UA, "created": x.Created,
					"lastSeen": x.LastSeen, "current": x.ID == s.ID, "remember": x.Remember})
			}
		}
		acctMu.Unlock()
		writeJSON(w, 200, map[string]any{"sessions": list})
	}))

	// ---------------- devices & workspaces
	mux.HandleFunc("/hub/api/devices", hubAPI(apiOpt{auth: true, full: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		writeJSON(w, 200, map[string]any{"devices": devicesFor(u), "hub": getConfig().Name, "me": userJSON(u)})
	}))
	mux.HandleFunc("/hub/api/workspaces", hubAPI(apiOpt{auth: true, full: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		writeJSON(w, 200, map[string]any{"workspaces": workspacesJSON(u)})
	}))
	mux.HandleFunc("/hub/api/wake", hubAPI(apiOpt{auth: true, full: true}, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		id := r.URL.Query().Get("id")
		if devicePerm(u, id) != "control" {
			writeJSON(w, 403, map[string]any{"error": "no access"})
			return
		}
		for _, d := range getConfig().HubKnown {
			if d.ID == id {
				n := 0
				for _, m := range d.Macs {
					if sendWOL(m.MAC, m.Bcast) == nil {
						n++
					}
				}
				audit("wake", u.Username, clientIP(r), prettyID(id))
				writeJSON(w, 200, map[string]any{"ok": n > 0})
				return
			}
		}
		writeJSON(w, 404, map[string]any{"error": "unknown device"})
	}))

	// ---------------- administration
	adm := apiOpt{auth: true, full: true, admin: true}
	mux.HandleFunc("/hub/api/admin/users", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		if r.Method == http.MethodPost { // create
			var req struct{ Username, Password, Role string }
			readJSON(r, &req)
			req.Username = strings.TrimSpace(req.Username)
			if req.Role != "admin" {
				req.Role = "member"
			}
			if req.Role == "admin" && u.Role != "owner" {
				writeJSON(w, 403, map[string]any{"error": "Only the owner can create admins"})
				return
			}
			if !validUsername(req.Username) {
				writeJSON(w, 400, map[string]any{"error": "Username: 3-32 letters, digits, . _ - @"})
				return
			}
			if err := checkPasswordPolicy(req.Username, req.Password); err != nil {
				writeJSON(w, 400, map[string]any{"error": err.Error()})
				return
			}
			acctMu.Lock()
			if userByNameLocked(req.Username) != nil {
				acctMu.Unlock()
				writeJSON(w, 400, map[string]any{"error": "That username is taken"})
				return
			}
			acct.Users = append(acct.Users, HubUser{ID: randomHex(6), Username: req.Username, Role: req.Role,
				PassHash: hashPassword(req.Password), MustChange: true, Created: time.Now()})
			saveAccountsLocked()
			acctMu.Unlock()
			audit("user-created", u.Username, clientIP(r), req.Username+" ("+req.Role+")")
		}
		acctMu.Lock()
		var list []map[string]any
		for i := range acct.Users {
			list = append(list, userJSON(&acct.Users[i]))
		}
		acctMu.Unlock()
		writeJSON(w, 200, map[string]any{"users": list})
	}))
	mux.HandleFunc("/hub/api/admin/users/update", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		var req struct {
			ID, Role, Password                 string
			Disabled, Reset2FA, Delete, Unlock *bool
		}
		readJSON(r, &req)
		acctMu.Lock()
		defer acctMu.Unlock()
		x := userByIDLocked(req.ID)
		if x == nil {
			writeJSON(w, 404, map[string]any{"error": "no such user"})
			return
		}
		if x.Role == "owner" && x.ID != u.ID || (x.Role == "admin" && u.Role != "owner" && x.ID != u.ID) {
			writeJSON(w, 403, map[string]any{"error": "You can't change that account"})
			return
		}
		what := []string{}
		if req.Delete != nil && *req.Delete {
			if x.Role == "owner" {
				writeJSON(w, 400, map[string]any{"error": "The owner can't be deleted"})
				return
			}
			out := acct.Users[:0]
			for _, y := range acct.Users {
				if y.ID != x.ID {
					out = append(out, y)
				}
			}
			name := x.Username
			acct.Users = out
			revokeSessionsLocked(req.ID, "")
			for i := range acct.Workspaces {
				ms := acct.Workspaces[i].Members[:0]
				for _, m := range acct.Workspaces[i].Members {
					if m.UserID != req.ID {
						ms = append(ms, m)
					}
				}
				acct.Workspaces[i].Members = ms
			}
			saveAccountsLocked()
			go audit("user-deleted", u.Username, clientIP(r), name)
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		if req.Role == "admin" || req.Role == "member" {
			if x.Role == "owner" || u.Role != "owner" {
				writeJSON(w, 403, map[string]any{"error": "Only the owner can change roles"})
				return
			}
			x.Role = req.Role
			what = append(what, "role="+req.Role)
		}
		if req.Disabled != nil && x.Role != "owner" {
			x.Disabled = *req.Disabled
			if x.Disabled {
				revokeSessionsLocked(x.ID, "")
			}
			what = append(what, "disabled="+map[bool]string{true: "yes", false: "no"}[x.Disabled])
		}
		if req.Reset2FA != nil && *req.Reset2FA {
			x.TOTPOn, x.TOTPSecret, x.Recovery = false, "", nil
			revokeSessionsLocked(x.ID, "")
			what = append(what, "2FA reset")
		}
		if req.Unlock != nil && *req.Unlock {
			x.Fails, x.LockedUntil = 0, time.Time{}
			what = append(what, "unlocked")
		}
		if req.Password != "" {
			if err := checkPasswordPolicy(x.Username, req.Password); err != nil {
				writeJSON(w, 400, map[string]any{"error": err.Error()})
				return
			}
			x.PassHash, x.MustChange = hashPassword(req.Password), x.ID != u.ID
			revokeSessionsLocked(x.ID, "")
			what = append(what, "password reset")
		}
		saveAccountsLocked()
		go audit("user-updated", u.Username, clientIP(r), x.Username+": "+strings.Join(what, ", "))
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/hub/api/admin/workspaces", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		var req struct {
			ID, Name string
			Devices  *[]string
			Members  *[]WSMember
			Delete   bool
		}
		if readJSON(r, &req) != nil {
			writeJSON(w, 400, map[string]any{"error": "bad request"})
			return
		}
		acctMu.Lock()
		defer acctMu.Unlock()
		if req.ID == "" { // create
			name := strings.TrimSpace(req.Name)
			if name == "" || len(name) > 60 {
				writeJSON(w, 400, map[string]any{"error": "Give the workspace a name"})
				return
			}
			ws := Workspace{ID: randomHex(6), Name: name, Created: time.Now()}
			acct.Workspaces = append(acct.Workspaces, ws)
			saveAccountsLocked()
			go audit("workspace-created", u.Username, clientIP(r), name)
			writeJSON(w, 200, map[string]any{"ok": true, "id": ws.ID})
			return
		}
		for i := range acct.Workspaces {
			ws := &acct.Workspaces[i]
			if ws.ID != req.ID {
				continue
			}
			if req.Delete {
				name := ws.Name
				acct.Workspaces = append(acct.Workspaces[:i], acct.Workspaces[i+1:]...)
				saveAccountsLocked()
				go audit("workspace-deleted", u.Username, clientIP(r), name)
				writeJSON(w, 200, map[string]any{"ok": true})
				return
			}
			if n := strings.TrimSpace(req.Name); n != "" && len(n) <= 60 {
				ws.Name = n
			}
			if req.Devices != nil {
				ws.Devices = uniq(*req.Devices)
			}
			if req.Members != nil {
				var ms []WSMember
				seen := map[string]bool{}
				for _, m := range *req.Members {
					if userByIDLocked(m.UserID) != nil && !seen[m.UserID] {
						if m.Perm != "view" {
							m.Perm = "control"
						}
						seen[m.UserID] = true
						ms = append(ms, m)
					}
				}
				ws.Members = ms
			}
			saveAccountsLocked()
			go audit("workspace-updated", u.Username, clientIP(r), ws.Name)
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		writeJSON(w, 404, map[string]any{"error": "no such workspace"})
	}))
	mux.HandleFunc("/hub/api/admin/security", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		if r.Method == http.MethodPost {
			if u.Role != "owner" {
				writeJSON(w, 403, map[string]any{"error": "Only the owner can change security settings"})
				return
			}
			var req HubSecurity
			readJSON(r, &req)
			acctMu.Lock()
			acct.Security.Require2FA = req.Require2FA
			acct.Security.IdleMinutes = clamp(req.IdleMinutes, 5, 24*60)
			acct.Security.RememberDay = clamp(req.RememberDay, 1, 90)
			saveAccountsLocked()
			acctMu.Unlock()
			audit("security-changed", u.Username, clientIP(r), "")
		}
		acctMu.Lock()
		sec := acct.Security
		acctMu.Unlock()
		writeJSON(w, 200, sec)
	}))
	mux.HandleFunc("/hub/api/admin/audit", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		writeJSON(w, 200, map[string]any{"entries": auditTail(300)})
	}))
	mux.HandleFunc("/hub/api/admin/forget", hubAPI(adm, func(w http.ResponseWriter, r *http.Request, u *HubUser, _ *HubSession) {
		id := r.URL.Query().Get("id")
		forgetHubDevice(id)
		audit("device-removed", u.Username, clientIP(r), prettyID(id))
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if len(s) == 9 && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// devicesFor lists the devices this user may see, with their permission.
func devicesFor(u *HubUser) []map[string]any {
	var out []map[string]any
	for _, d := range hubDeviceList() {
		if p := devicePerm(u, d["id"].(string)); p != "" {
			d["perm"] = p
			out = append(out, d)
		}
	}
	return out
}

func workspacesJSON(u *HubUser) []map[string]any {
	all := map[string]map[string]any{}
	for _, d := range hubDeviceList() {
		all[d["id"].(string)] = d
	}
	acctMu.Lock()
	names := map[string]string{}
	for _, x := range acct.Users {
		names[x.ID] = x.Username
	}
	acctMu.Unlock()
	var out []map[string]any
	for _, w := range visibleWorkspaces(u) {
		var devs []map[string]any
		for _, id := range w.Devices {
			if d := all[id]; d != nil {
				devs = append(devs, d)
			} else {
				devs = append(devs, map[string]any{"id": id, "pretty": prettyID(id), "name": "Unknown device", "online": false})
			}
		}
		var mem []map[string]any
		myPerm := ""
		for _, m := range w.Members {
			mem = append(mem, map[string]any{"userId": m.UserID, "username": names[m.UserID], "perm": m.Perm})
			if m.UserID == u.ID {
				myPerm = m.Perm
			}
		}
		if isAdminRole(u) {
			myPerm = "control"
		}
		sort.SliceStable(devs, func(i, j int) bool { return devs[i]["online"] == true && devs[j]["online"] != true })
		out = append(out, map[string]any{"id": w.ID, "name": w.Name, "devices": devs, "members": mem, "perm": myPerm})
	}
	return out
}
