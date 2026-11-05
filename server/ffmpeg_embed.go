//go:build embedffmpeg

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

//go:embed bin/ffmpeg.exe.gz
var ffmpegGz []byte

// extractEmbeddedFFmpeg unpacks the bundled ffmpeg once into %LOCALAPPDATA%\KadduDesk.
func extractEmbeddedFFmpeg() (string, error) {
	sum := sha256.Sum256(ffmpegGz)
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "KadduDesk")
	os.MkdirAll(dir, 0o755)
	dst := filepath.Join(dir, "ffmpeg-"+hex.EncodeToString(sum[:4])+".exe")
	if st, err := os.Stat(dst); err == nil && st.Size() > 1<<20 {
		return dst, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(ffmpegGz))
	if err != nil {
		return "", err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, zr); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	return dst, os.Rename(tmp, dst)
}
