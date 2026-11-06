package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	flagNoTray    = flag.Bool("notray", false, "run without tray icon")
	flagMinimized = flag.Bool("minimized", false, "start in the background")
	flagDevHTTP   = flag.String("devhttp", "", "also serve plain HTTP on a loopback address, e.g. 127.0.0.1:47899 (testing)")
	flagDevHub    = flag.String("devhub", "", "also serve the hub over plain HTTP on a loopback address (testing)")
	flagProfile   = flag.String("profile", "", "run a separate instance with its own settings (testing / multiple instances)")
	flagApp       = flag.Bool("app", false, "run as the KadduDesk app core (the app's UI and connections)")
	flagAppPort   = flag.Int("appport", 0, "fixed loopback port for the app UI (0 = remembered/random)")
	flagDataDir   = flag.String("datadir", "", "settings folder (used by the Android app)")
	flagAppPath   = flag.String("apppath", "", "path of the KadduDesk app, opened from the tray icon")
)

var serverCert tls.Certificate

var agentMode bool // Windows: running as the SYSTEM agent started by the service

var hostStarted bool

func main() {
	flag.Parse()
	if platformEarly() { // service entry point, -install-service, self-elevation … handled
		return
	}
	if *flagDataDir != "" {
		initDataDirAt(*flagDataDir)
	} else {
		selectDataDir()
	}
	if *flagProfile != "" {
		initDataDirAt(dataDir + "-" + *flagProfile)
	}
	if *flagApp {
		runApp()
		return
	}
	setupLog(filepath.Join(dataDir, "server.log"))
	// single instance: a second launch just brings up the app / control panel of the running one
	if alreadyRunning() {
		return
	}
	if err := startHost(); err != nil {
		fatal("%v", err)
	}
	if *flagNoTray || !trayAvailable() {
		select {}
	}
	runTray()
}

func setupLog(path string) {
	logFile, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	logRing.f = logFile
	log.SetOutput(logRing)
	log.SetFlags(log.LstdFlags)
}

// startHost brings up everything that makes this device controllable. Used by the
// standalone server and, on Linux/Android, inside the app itself.
func startHost() error {
	loadConfig()
	loadAccounts()
	loadAudit()
	logf("KadduDesk %s host starting on %s (admin=%v)", version, platformName(), isAdmin())
	cert, err := loadOrCreateCert()
	if err != nil {
		return fmt.Errorf("could not create TLS certificate: %v", err)
	}
	if err := findFFmpeg(); err != nil {
		return err
	}
	logf("ffmpeg: %s", ffmpegPath)
	platformStart()
	if err := inputInit(); err != nil {
		logf("input: %v", err)
	}
	if err := video.init(); err != nil {
		return fmt.Errorf("video init: %v", err)
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
		return fmt.Errorf("could not listen on port %d: %v — is another program using it?", getConfig().Port, err)
	}
	serverCert = cert
	startTunnelServer(webMux) // serves devices that reach us through a hub
	go hubClientLoop()
	if err := applyHubSettings(cert); err != nil {
		logf("hub: %v", err)
	}
	if err := startAdmin(); err != nil {
		logf("admin panel: %v", err)
	}
	if getConfig().MeshEnabled {
		go startMesh(false)
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
	logf("listening on :%d  (fingerprint %s, ID %s)", c.Port, prettyFingerprint(), prettyID(deviceID()))
	hostStarted = true
	return nil
}

func quit() {
	platformQuit()
	kickAll()
	os.Exit(0)
}

// openApp launches the KadduDesk app (the tray icon's "Open KadduDesk").
func openApp() {
	for _, p := range appCandidates() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if runAsUser(`"`+p+`"`) == nil || exec.Command(p).Start() == nil {
				return
			}
		}
	}
	notify("KadduDesk", fmt.Sprintf("This device is shared.\n\nID: %s\nPIN: %s\n\nInstall the KadduDesk app to change settings.", prettyID(deviceID()), getConfig().PinPlain))
}

func appCandidates() []string {
	var out []string
	if *flagAppPath != "" {
		out = append(out, *flagAppPath)
	}
	if p := getConfig().AppPath; p != "" {
		out = append(out, p)
	}
	return append(out, defaultAppPaths()...)
}

// openAdminOfRunningCopy is used by a second launch: show the running copy.
func openAdminOfRunningCopy() {
	loadConfig()
	openApp()
}

var errNotSupported = errors.New("not supported on this device")
