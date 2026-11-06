//go:build linux && !kdandroid

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed web/icon.png
var trayIcon []byte

func platformName() string { return "Linux" }

func trayAvailable() bool { return os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" }

func platformEarly() bool { return false }

func selectDataDir() { initDataDir() }

func initDataDir() {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".config")
	}
	initDataDirAt(filepath.Join(base, "kaddudesk"))
}

func initDataDirAt(dir string) {
	dataDir = dir
	_ = os.MkdirAll(dataDir, 0o700)
}

var lockFile *os.File

// alreadyRunning uses an flock in the data dir.
func alreadyRunning() bool {
	f, err := os.OpenFile(filepath.Join(dataDir, "server.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		openAdminOfRunningCopy()
		return true
	}
	lockFile = f
	return false
}

func platformStart() {}
func platformQuit()  {}

func openBrowser(u string) {
	if p := firstExisting("xdg-open", "gio", "sensible-browser"); p != "" {
		if strings.HasSuffix(p, "gio") {
			exec.Command(p, "open", u).Start()
			return
		}
		exec.Command(p, u).Start()
		return
	}
	logf("open this in a browser: %s", u)
}

func fatal(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	logf("FATAL: %s", msg)
	fmt.Fprintln(os.Stderr, "KadduDesk:", msg)
	if p := firstExisting("zenity"); p != "" && os.Getenv("DISPLAY")+os.Getenv("WAYLAND_DISPLAY") != "" {
		exec.Command(p, "--error", "--title", "KadduDesk Server", "--text", msg).Run()
	}
	os.Exit(1)
}

func sendSAS() bool { return false }

// ------------------------------------------------------------ files

func rootEntries() []fileEntry {
	ents := []fileEntry{{Name: "/", Path: "/", Dir: true}}
	for _, d := range []string{"/media", "/mnt", "/run/media"} {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			ents = append(ents, fileEntry{Name: d, Path: d, Dir: true})
		}
	}
	return ents
}

func userHome() string { h, _ := os.UserHomeDir(); return h }

func uploadDir() string {
	if d := getConfig().UploadDir; d != "" {
		return d
	}
	return filepath.Join(userHome(), "Downloads", "KadduDesk")
}

// ------------------------------------------------------------ "service mode" is Windows-only

var errNoService = errors.New("Service mode is for Windows. On Linux use 'Start at login'; the lock screen is captured on X11 and with KMS capture")

func installService() error           { return errNoService }
func startService() error             { return errNoService }
func stopAndDeleteService(bool) error { return nil }
func serviceInstalled() bool          { return false }
func serviceRunning() bool            { return false }
func requestSAS() bool                { return false }

// ------------------------------------------------------------ start at login (XDG autostart)

func autostartFile() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(userHome(), ".config")
	}
	return filepath.Join(base, "autostart", "kaddudesk-server.desktop")
}

func autostartEnabled() bool { _, err := os.Stat(autostartFile()); return err == nil }

func setAutostart(on bool) error {
	if !on {
		err := os.Remove(autostartFile())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, _ := os.Executable()
	os.MkdirAll(filepath.Dir(autostartFile()), 0o755)
	desk := "[Desktop Entry]\nType=Application\nName=KadduDesk Server\nComment=Remote desktop server\n" +
		"Exec=\"" + exe + "\" -minimized\nIcon=kaddudesk\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(autostartFile(), []byte(desk), 0o644)
}

// ------------------------------------------------------------ firewall (ufw, if active and we're root)

func addFirewallRule(public bool) error {
	p := firstExisting("ufw")
	if p == "" {
		return nil
	}
	if !isAdmin() {
		return errors.New("run: sudo ufw allow 47800/tcp && sudo ufw allow 47810/udp")
	}
	port := fmt.Sprint(getConfig().Port)
	if out, err := exec.Command(p, "allow", port+"/tcp").CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	exec.Command(p, "allow", "47810/udp").Run()
	return nil
}

func addHubFirewallRule() {
	if p := firstExisting("ufw"); p != "" && isAdmin() {
		exec.Command(p, "allow", fmt.Sprint(getConfig().HubPort)+"/tcp").Run()
	}
}

var hostLockFile *os.File

// acquireHostLock: the app may run the host in-process unless a separate server already does.
func acquireHostLock() bool {
	if lockFile != nil || hostLockFile != nil {
		return true
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "server.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return false
	}
	hostLockFile = f
	return true
}

func launchHostElevated() error { return errors.New("not needed on Linux") }

func extraBroadcasts() []string { return nil }

func defaultAppPaths() []string {
	return []string{"/opt/kaddudesk-client/kaddudesk-client", "/usr/local/bin/kaddudesk"}
}

func notify(title, msg string) {
	if p := firstExisting("notify-send"); p != "" {
		exec.Command(p, title, msg).Start()
	}
}

func runAsUser(cmdline string) error { return exec.Command("sh", "-c", cmdline).Start() }

func platformIPs() []string { return nil }
