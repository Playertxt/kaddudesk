package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/systray"
)

var (
	flagNoTray    = flag.Bool("notray", false, "run without tray icon")
	flagMinimized = flag.Bool("minimized", false, "don't open the control panel on start")
	flagDevHTTP   = flag.String("devhttp", "", "also serve plain HTTP on a loopback address, e.g. 127.0.0.1:47899 (testing)")
	trayReady     bool
	trayMu        sync.Mutex
	mStatus       *systray.MenuItem
	mPause        *systray.MenuItem
)

var agentMode bool // Windows: running as the SYSTEM agent started by the service

func main() {
	flag.Parse()
	if platformEarly() { // service entry point, -install-service, ... handled and done
		return
	}
	selectDataDir()

	logFile, _ := os.OpenFile(filepath.Join(dataDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	logRing.f = logFile
	log.SetOutput(logRing)
	log.SetFlags(log.LstdFlags)

	// single instance: a second launch just opens the control panel of the running one
	if alreadyRunning() {
		return
	}

	loadConfig()
	logf("KadduDesk server %s starting on %s (admin=%v)", version, platformName(), isAdmin())
	cert, err := loadOrCreateCert()
	if err != nil {
		fatal("Could not create TLS certificate: %v", err)
	}
	if err := findFFmpeg(); err != nil {
		fatal("%v", err)
	}
	logf("ffmpeg: %s", ffmpegPath)
	platformStart()
	if err := inputInit(); err != nil {
		logf("input: %v", err)
	}
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
	if *flagNoTray || !trayAvailable() {
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
	platformQuit()
	kickAll()
	os.Exit(0)
}

// openAdminOfRunningCopy is used by a second launch: show the running copy's control panel.
func openAdminOfRunningCopy() {
	loadConfig()
	if b, err := os.ReadFile(filepath.Join(dataDir, "adminkey")); err == nil {
		adminKey = string(b)
		openBrowser(adminURL())
	}
}
