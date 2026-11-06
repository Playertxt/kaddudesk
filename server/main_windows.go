package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

//go:embed assets/icon.ico
var trayIcon []byte

var (
	flagService   = flag.Bool("service", false, "internal: run as the Windows service")
	flagAgent     = flag.Bool("agent", false, "internal: SYSTEM agent started by the service")
	flagInstall   = flag.Bool("install-service", false, "install Service mode (lock screen + UAC access) and exit")
	flagUninstall = flag.Bool("uninstall-service", false, "remove Service mode and exit")
	flagNoElevate = flag.Bool("noelevate", false, "don't ask for admin rights (testing; admin windows can't be controlled)")
)

func platformName() string { return "Windows" }

func trayAvailable() bool { return true }

// platformEarly handles the service entry point and the install/uninstall commands.
func platformEarly() bool {
	if *flagService {
		runService()
		return true
	}
	setDPIAware()
	agentMode = *flagAgent
	// Sharing this PC needs admin rights (to control admin windows). The exe itself starts
	// without them so the app can run it; as a host it re-launches itself elevated.
	if !*flagApp && !agentMode && !*flagInstall && !*flagUninstall && !*flagNoElevate && !isAdmin() {
		if err := shellRunAs(os.Args[1:]); err != nil {
			fatal("KadduDesk needs administrator rights to share this PC: %v", err)
		}
		return true
	}
	if *flagInstall || *flagUninstall {
		initDataDir()
		loadConfig()
		var err error
		if *flagInstall {
			if err = installService(); err == nil {
				err = startService()
			}
		} else {
			err = stopAndDeleteService(true)
		}
		if err != nil {
			fatal("Service mode: %v", err)
		}
		return true
	}
	return false
}

func selectDataDir() {
	if agentMode {
		initDataDirAt(machineDataDir())
	} else {
		initDataDir()
	}
}

var instanceMutex windows.Handle

func alreadyRunning() bool {
	name, _ := windows.UTF16PtrFromString(`Global\KadduDeskServerMutex` + *flagProfile)
	h, err := windows.CreateMutex(nil, false, name)
	if err != windows.ERROR_ALREADY_EXISTS {
		instanceMutex = h
		if !agentMode && serviceRunning() {
			// Service mode is active but its agent isn't up yet; don't fight it for the ports.
			fatal("KadduDesk is running in Service mode. Open the control panel from the tray icon.")
		}
		return false
	}
	if agentMode {
		os.Exit(3) // a user-mode copy still owns the ports; the service retries shortly
	}
	if serviceInstalled() { // the SYSTEM agent is the running copy: use its settings and key
		initDataDirAt(machineDataDir())
	}
	openAdminOfRunningCopy()
	return true
}

func platformStart() {
	initJob()
	initCursors()
	if agentMode {
		enablePrivilege("SeShutdownPrivilege")
	}
}

func platformQuit() {
	if agentMode { // the service would just restart us: stop the service instead
		go stopAndDeleteService(false)
		time.Sleep(3 * time.Second)
	}
	if jobHandle != 0 {
		windows.CloseHandle(jobHandle) // kills ffmpeg children
	}
}

// openBrowser asks Explorer to open the URL so the browser starts un-elevated.
func openBrowser(u string) {
	if agentMode { // we are SYSTEM: open it as the signed-in user
		runAsUser(`explorer.exe "` + u + `"`)
		return
	}
	if err := exec.Command("explorer.exe", u).Start(); err != nil {
		exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	}
}

func fatal(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	logf("FATAL: %s", msg)
	t, _ := windows.UTF16PtrFromString("KadduDesk Server")
	m, _ := windows.UTF16PtrFromString(msg)
	windows.MessageBox(0, m, t, 0x10)
	os.Exit(1)
}

// sendSAS triggers the real Ctrl+Alt+Del. Windows only allows this from a SYSTEM service.
func sendSAS() bool {
	if agentMode {
		return requestSAS()
	}
	if !isSystem() {
		return false
	}
	p := windows.NewLazySystemDLL("sas.dll").NewProc("SendSAS")
	if p.Find() != nil {
		return false
	}
	p.Call(0)
	return true
}

func shellRunAs(args []string) error {
	exe, _ := os.Executable()
	var parts []string
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(parts, " "))
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
	return windows.ShellExecute(0, verb, file, params, dir, windows.SW_SHOWNORMAL)
}

// launchHostElevated starts sharing from the app: the host runs as admin (one UAC prompt).
func launchHostElevated() error {
	args := []string{"-minimized"}
	if *flagAppPath != "" {
		args = append(args, "-apppath", *flagAppPath)
	}
	if err := shellRunAs(args); err != nil {
		return fmt.Errorf("sharing needs administrator rights: %v", err)
	}
	for i := 0; i < 40; i++ { // wait until it answers
		time.Sleep(250 * time.Millisecond)
		app.mu.Lock()
		app.extCache = time.Time{}
		app.mu.Unlock()
		if externalHostOK() {
			return nil
		}
	}
	return fmt.Errorf("sharing didn't start — check the KadduDesk tray icon")
}

func acquireHostLock() bool { return false }

func extraBroadcasts() []string { return nil }

func defaultAppPaths() []string {
	la := os.Getenv("LOCALAPPDATA")
	pf := os.Getenv("ProgramFiles")
	return []string{filepath.Join(la, "Programs", "KadduDesk", "KadduDesk.exe"), filepath.Join(pf, "KadduDesk", "KadduDesk.exe")}
}

func notify(title, msg string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	go windows.MessageBox(0, m, t, 0x40)
}

func platformIPs() []string { return nil }
