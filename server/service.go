package main

// Service mode — how KadduDesk reaches the lock screen, sign-in screen and UAC prompts.
//
//   KadduDesk service (SYSTEM, session 0)            ← installed via the control panel
//     └─ spawns: KadduDeskServer.exe -agent         ← SYSTEM token, but in the active console
//                                                      session, so it can capture/inject on
//                                                      both the "Default" and "Winlogon" desktops
//   The service respawns the agent when it exits or when the console session changes
//   (sign-out, user switch) and performs the real Ctrl+Alt+Del (SendSAS) on request.

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "KadduDesk"
	serviceDisplay = "KadduDesk Remote Desktop"
	sasEventName   = `Global\KadduDeskSAS`
)

var agentMode bool // true when running as the SYSTEM agent spawned by the service

var (
	wtsapi32          = windows.NewLazySystemDLL("wtsapi32.dll")
	pWTSLogoffSession = wtsapi32.NewProc("WTSLogoffSession")
)

// machineDataDir is shared by the service, the agent and the installer.
func machineDataDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "KadduDesk")
}

func installDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "KadduDesk")
}

// secureDir creates dir readable only by SYSTEM and Administrators (it holds the TLS key and PIN).
func secureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func isSystem() bool {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && u.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
}

// ------------------------------------------------------------ service status helpers

func serviceInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	s.Close()
	return true
}

func serviceRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	defer s.Close()
	st, err := s.Query()
	return err == nil && st.State == svc.Running
}

// ------------------------------------------------------------ install / uninstall

// installService copies this exe to Program Files (an admin-only location, so a normal user
// can't swap the binary a SYSTEM service runs), migrates config + certificate (so the PIN and
// the fingerprint clients pinned stay the same), registers and starts the service.
func installService() error {
	if !isAdmin() {
		return errors.New("administrator rights required")
	}
	self, _ := os.Executable()
	dst := filepath.Join(installDir(), "KadduDeskServer.exe")
	if err := os.MkdirAll(installDir(), 0o755); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(dst)) {
		// stop an older service first so its exe isn't locked
		stopAndDeleteService(false)
		if err := copyFile(self, dst); err != nil {
			return fmt.Errorf("copy to %s: %w", dst, err)
		}
	}
	md := machineDataDir()
	if err := secureDir(md); err != nil {
		return fmt.Errorf("data folder: %w", err)
	}
	for _, f := range []string{"server.json", "cert.pem", "key.pem", "adminkey"} {
		src, to := filepath.Join(dataDir, f), filepath.Join(md, f)
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if _, err := os.Stat(src); err == nil {
			copyFile(src, to)
		}
	}
	// allow services to generate Ctrl+Alt+Del (SoftwareSASGeneration: 1 = services)
	if k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if v, _, err := k.GetIntegerValue("SoftwareSASGeneration"); err != nil || v == 0 {
			k.SetDWordValue("SoftwareSASGeneration", 1)
		}
		k.Close()
	}

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		stopAndDeleteService(false)
		time.Sleep(500 * time.Millisecond)
	}
	s, err = m.CreateService(serviceName, dst, mgr.Config{
		DisplayName:  serviceDisplay,
		Description:  "Lets you control this PC remotely with KadduDesk, including the lock screen and UAC prompts.",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}, "-service")
	if err != nil {
		return err
	}
	defer s.Close()
	s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 2 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
	}, 3600)
	// the logon task would start a second (user-mode) copy; the service replaces it
	if autostartEnabled() {
		setAutostart(false)
	}
	addFirewallRuleFor(dst, false)
	logf("service installed: %s", dst)
	return nil
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

// stopAndDeleteService stops the service (which also ends the agent) and optionally deletes it.
func stopAndDeleteService(del bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	if del {
		if err := s.Delete(); err != nil {
			return err
		}
	}
	if st, err := s.Control(svc.Stop); err == nil {
		for i := 0; i < 50 && st.State != svc.Stopped; i++ {
			time.Sleep(100 * time.Millisecond)
			st, _ = s.Query()
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

// ------------------------------------------------------------ the service itself

type kdService struct{}

func runService() {
	initDataDirAt(machineDataDir())
	if f, err := os.OpenFile(filepath.Join(dataDir, "service.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(f)
	}
	if err := svc.Run(serviceName, &kdService{}); err != nil {
		log.Printf("service: %v", err)
	}
}

func (kdService) Execute(args []string, req <-chan svc.ChangeRequest, st chan<- svc.Status) (bool, uint32) {
	st <- svc.Status{State: svc.StartPending}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { superviseAgent(stop); close(done) }()
	go sasListener(stop)
	st <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			st <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			st <- svc.Status{State: svc.StopPending}
			close(stop)
			<-done
			return false, 0
		}
	}
	return false, 0
}

// superviseAgent keeps exactly one agent alive in the active console session.
func superviseAgent(stop <-chan struct{}) {
	var proc windows.Handle
	var sess uint32 = 0xFFFFFFFF
	kill := func() {
		if proc != 0 {
			windows.TerminateProcess(proc, 0)
			windows.WaitForSingleObject(proc, 3000)
			windows.CloseHandle(proc)
			proc = 0
		}
	}
	defer kill()
	backoff := time.Second
	for {
		cur := windows.WTSGetActiveConsoleSessionId()
		alive := proc != 0
		if alive {
			if ev, _ := windows.WaitForSingleObject(proc, 0); ev == windows.WAIT_OBJECT_0 {
				var code uint32
				windows.GetExitCodeProcess(proc, &code)
				log.Printf("agent exited (code %d)", code)
				windows.CloseHandle(proc)
				proc, alive = 0, false
				time.Sleep(backoff)
				if backoff < 10*time.Second {
					backoff *= 2
				}
			}
		}
		if alive && cur != sess {
			log.Printf("console session changed %d -> %d, restarting agent", sess, cur)
			kill()
			alive = false
		}
		if !alive && cur != 0xFFFFFFFF {
			h, err := spawnAgent(cur)
			if err != nil {
				log.Printf("spawn agent in session %d: %v", cur, err)
			} else {
				proc, sess = h, cur
				log.Printf("agent started in session %d", cur)
				go func(h windows.Handle) { // reset backoff once the agent proves stable
					time.Sleep(30 * time.Second)
					backoff = time.Second
				}(h)
			}
		}
		select {
		case <-stop:
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// spawnAgent starts "<exe> -agent" as SYSTEM inside the given session on winsta0\default.
func spawnAgent(session uint32) (windows.Handle, error) {
	var self windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ALL_ACCESS, &self); err != nil {
		return 0, err
	}
	defer self.Close()
	var tok windows.Token
	if err := windows.DuplicateTokenEx(self, windows.MAXIMUM_ALLOWED, nil, windows.SecurityIdentification, windows.TokenPrimary, &tok); err != nil {
		return 0, err
	}
	defer tok.Close()
	if err := windows.SetTokenInformation(tok, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4); err != nil {
		return 0, fmt.Errorf("set session: %w", err)
	}
	exe, _ := os.Executable()
	cmd, _ := windows.UTF16PtrFromString(syscall.EscapeArg(exe) + " -agent -minimized")
	desk, _ := windows.UTF16PtrFromString(`winsta0\default`)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	si := windows.StartupInfo{Desktop: desk}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(tok, nil, cmd, nil, nil, false,
		windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, nil, dir, &si, &pi); err != nil {
		return 0, err
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}

// sasListener waits for the agent to ask for Ctrl+Alt+Del. SendSAS only works from a service.
func sasListener(stop <-chan struct{}) {
	// Allow SYSTEM (the agent) to signal; nobody else.
	sd, _ := windows.SecurityDescriptorFromString("D:(A;;GA;;;SY)")
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(sasEventName)
	ev, err := windows.CreateEvent(sa, 0, 0, name)
	if err != nil {
		log.Printf("SAS event: %v", err)
		return
	}
	defer windows.CloseHandle(ev)
	p := windows.NewLazySystemDLL("sas.dll").NewProc("SendSAS")
	for {
		select {
		case <-stop:
			return
		default:
		}
		if r, _ := windows.WaitForSingleObject(ev, 500); r == windows.WAIT_OBJECT_0 && p.Find() == nil {
			p.Call(0)
			log.Printf("SendSAS")
		}
	}
}

// requestSAS (agent side) asks the service to press Ctrl+Alt+Del.
func requestSAS() bool {
	name, _ := windows.UTF16PtrFromString(sasEventName)
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ev)
	return windows.SetEvent(ev) == nil
}

// ------------------------------------------------------------ acting as the signed-in user

// consoleUserToken returns a primary token of the user signed in at the console (agent mode).
func consoleUserToken() (windows.Token, error) {
	var t windows.Token
	if err := windows.WTSQueryUserToken(windows.WTSGetActiveConsoleSessionId(), &t); err != nil {
		return 0, err
	}
	var p windows.Token
	err := windows.DuplicateTokenEx(t, windows.MAXIMUM_ALLOWED, nil, windows.SecurityIdentification, windows.TokenPrimary, &p)
	t.Close()
	return p, err
}

// runAsUser starts a command as the signed-in user (so apps don't open as SYSTEM).
// Outside agent mode it just runs it normally.
func runAsUser(cmdline string) error {
	if !agentMode {
		cmd, _ := windows.UTF16PtrFromString(cmdline)
		si := windows.StartupInfo{}
		si.Cb = uint32(unsafe.Sizeof(si))
		var pi windows.ProcessInformation
		if err := windows.CreateProcess(nil, cmd, nil, nil, false, 0, nil, nil, &si, &pi); err != nil {
			return err
		}
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		return nil
	}
	tok, err := consoleUserToken()
	if err != nil {
		return errors.New("nobody is signed in on this PC")
	}
	defer tok.Close()
	var env *uint16
	if windows.CreateEnvironmentBlock(&env, tok, false) == nil {
		defer windows.DestroyEnvironmentBlock(env)
	}
	cmd, _ := windows.UTF16PtrFromString(cmdline)
	desk, _ := windows.UTF16PtrFromString(`winsta0\default`)
	si := windows.StartupInfo{Desktop: desk}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(tok, nil, cmd, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT, env, nil, &si, &pi); err != nil {
		return err
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return nil
}

var homeCache struct {
	dir string
	at  time.Time
}

// userHome is the signed-in user's profile folder (SYSTEM's own profile is useless here).
func userHome() string {
	if !agentMode {
		h, _ := os.UserHomeDir()
		return h
	}
	if time.Since(homeCache.at) < 30*time.Second && homeCache.dir != "" {
		return homeCache.dir
	}
	tok, err := consoleUserToken()
	if err != nil {
		return `C:\Users\Public`
	}
	defer tok.Close()
	d, err := tok.GetUserProfileDirectory()
	if err != nil {
		return `C:\Users\Public`
	}
	homeCache.dir, homeCache.at = d, time.Now()
	return d
}

func uploadDir() string {
	if d := getConfig().UploadDir; d != "" {
		return d
	}
	return filepath.Join(userHome(), "Downloads", "KadduDesk")
}

func logoffConsole() error {
	r, _, err := pWTSLogoffSession.Call(0, uintptr(windows.WTSGetActiveConsoleSessionId()), 0)
	if r == 0 {
		return err
	}
	return nil
}

func enablePrivilege(name string) {
	var tok windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok) != nil {
		return
	}
	defer tok.Close()
	var luid windows.LUID
	n, _ := windows.UTF16PtrFromString(name)
	if windows.LookupPrivilegeValue(nil, n, &luid) != nil {
		return
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}
