package main

import (
	"os"
	"path/filepath"
	"strings"
)

func initDataDir() {
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	initDataDirAt(filepath.Join(base, "KadduDesk"))
}

// initDataDirAt selects the settings folder. Service mode uses %ProgramData%\KadduDesk,
// locked down to SYSTEM + Administrators because it holds the TLS key and PIN.
func initDataDirAt(dir string) {
	dataDir = dir
	if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(machineDataDir())) {
		if _, err := os.Stat(dir); err != nil {
			secureDir(dir)
		}
	}
	_ = os.MkdirAll(dataDir, 0o700)
}
