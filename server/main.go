package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

//go:embed assets/icon.ico
var trayIcon []byte

var (
	flagNoTray    = flag.Bool("notray", false, "run without tray icon")
	flagMinimized = flag.Bool("minimized", false, "don't open the control panel on start")
	flagDevHTTP   = flag.String("devhttp", "", "also serve plain HTTP on a loopback address, e.g. 127.0.0.1:47899 (testing)")
	flagService   = flag.Bool("service", false, "internal: run as the Windows service")
	flagAgent     = flag.Bool("agent", false, "internal: SYSTEM agent started by the service")
	flagInstall   = flag.Bool("install-service", false, "install Service mode (lock screen + UAC access) and exit")
	flagUninstall = flag.Bool("uninstall-service", false, "remove Service mode and exit")
	trayReady     bool
	trayMu        sync.Mutex
	mStatus       *systray.MenuItem
	mPause        *systray.MenuItem
)

func main() {
	flag.Parse()
	if *flagService {
		runService()
		return
	}
	setDPIAware()
	agentMode = *flagAgent
	if agentMode {
		initDataDirAt(machineDataDir())
	} else {
		initDataDir()
	}

	logFile, _ := os.OpenFile(filepath.Join(dataDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	logRing.f = logFile
	log.SetOutput(logRing)
	log.SetFlags(log.LstdFlags)

	// single instance: a second launch just opens the control panel of the running one
	name, _ := windows.UTF16PtrFromString("Global\\KadduDeskServerMutex")
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		loadConfig()
		if b, err := os.ReadFile(filepath.Join(dataDir, "adminkey")); err == nil {
			adminKey = string(b)
			openBrowser(adminURL())
		}
		return
	}
	_ = h

	loadConfig()
	logf("KadduDesk server %s starting (admin=%v)", version, isAdmin())
	cert, err := loadOrCreateCert()
	if err != nil {
		fatal("Could not create TLS certificate: %v", err)
	}
	if err := findFFmpeg(); err != nil {
		fatal("%v\n\nPut ffmpeg.exe in the same folder as KadduDeskServer.exe.", err)
	}
	logf("ffmpeg: %s", ffmpegPath)
	initJob()
	initCursors()
	if err := video.init(); err != nil {
		fatal("video init: %v", err)
	}
	if err := audio.init(); err != nil {
		logf("audio init: %v", err)
	}
	go inputLoop()
	go cursorLoop()
	go clipboardLoop()
	go discoveryLoop()
	go desktopLoop()
	err = startWebServer(cert)
	for i := 0; err != nil && agentMode && i < 20; i++ { // old copy may still be exiting
		time.Sleep(500 * time.Millisecond)
		err = startWebServer(cert)
	}
	if err != nil {
		fatal("Could not listen on port %d: %v\n\nIs another program using it?", getConfig().Port, err)
	}
	if err := startAdmin(); err != nil {
		logf("admin panel: %v", err)
	}
	if c := getConfig(); !c.FirewallDone && isAdmin() && !agentMode {
		if err := addFirewallRule(false); err == nil {
			cfgMu.Lock()
			cfg.FirewallDone = true
			saveConfigLocked()
			cfgMu.Unlock()
		}
	}
	go video.probeEncoder() // warm up so the first connection is instant
	c := getConfig()
	logf("listening on https://0.0.0.0:%d  (fingerprint %s)", c.Port, prettyFingerprint())
	for _, ip := range localIPs() {
		logf("  connect: https://%s:%d/", ip, c.Port)
	}
	if !*flagMinimized {
		openBrowser(adminURL())
	}
	if *flagNoTray {
		select {}
	}
	systray.Run(onTrayReady, func() {})
}

func onTrayReady() {
	systray.SetIcon(trayIcon)
	systray.SetTitle("KadduDesk")
	mStatus = systray.AddMenuItem("KadduDesk server", "")
	mStatus.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open control panel", "")
	mPause = systray.AddMenuItemCheckbox("Pause sharing", "Refuse all connections", getConfig().Paused)
	mKick := systray.AddMenuItem("Disconnect everyone", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit KadduDesk", "")
	trayMu.Lock()
	trayReady = true
	trayMu.Unlock()
	updateTray()
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(adminURL())
			case <-mPause.ClickedCh:
				cfgMu.Lock()
				cfg.Paused = !cfg.Paused
				saveConfigLocked()
				p := cfg.Paused
				cfgMu.Unlock()
				if p {
					kickAll()
				}
				updateTray()
			case <-mKick.ClickedCh:
				kickAll()
			case <-mQuit.ClickedCh:
				quit()
			}
		}
	}()
	systray.SetOnTapped(func() { openBrowser(adminURL()) })
}

func updateTray() {
	trayMu.Lock()
	ready := trayReady
	trayMu.Unlock()
	if !ready {
		return
	}
	c := getConfig()
	n := clientCount()
	status := fmt.Sprintf("Ready  ·  PIN %s", c.PinPlain)
	if c.Paused {
		status = "Paused"
		mPause.Check()
	} else {
		mPause.Uncheck()
	}
	if n > 0 {
		status = fmt.Sprintf("%d device(s) connected", n)
	}
	mStatus.SetTitle(status)
	systray.SetTooltip("KadduDesk — " + status)
}

func quit() {
	if agentMode { // the service would just restart us: stop the service instead
		go stopAndDeleteService(false)
		time.Sleep(3 * time.Second)
	}
	kickAll()
	if jobHandle != 0 {
		windows.CloseHandle(jobHandle) // kills ffmpeg children
	}
	os.Exit(0)
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

// sendSAS triggers the real Ctrl+Alt+Del. Windows only allows this from SYSTEM.
func sendSAS() bool {
	if agentMode {
		return requestSAS()
	}
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil || !u.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return false
	}
	p := windows.NewLazySystemDLL("sas.dll").NewProc("SendSAS")
	if p.Find() != nil {
		return false
	}
	p.Call(0)
	return true
}

var _ = unsafe.Sizeof(0)
