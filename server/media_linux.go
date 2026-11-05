package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// ffmpegKMS is an optional copy of ffmpeg with cap_sys_admin (set up by install-linux.sh),
// needed for kmsgrab — the only way to capture a Wayland desktop without a portal prompt.
var ffmpegKMS string

func findFFmpeg() error {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	for _, c := range []string{filepath.Join(dir, "ffmpeg"), "/opt/kaddudesk/ffmpeg"} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			ffmpegPath = c
		}
	}
	if ffmpegPath == "" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = p
		}
	}
	for _, c := range []string{filepath.Join(dir, "ffmpeg-kms"), "/opt/kaddudesk/ffmpeg-kms"} {
		if _, err := os.Stat(c); err == nil {
			ffmpegKMS = c
		}
	}
	if ffmpegPath == "" {
		return errors.New("ffmpeg not found. Install it: sudo apt install ffmpeg  (Fedora: sudo dnf install ffmpeg, Arch: sudo pacman -S ffmpeg)")
	}
	return nil
}

func ffStart(ctx context.Context, args []string, desktop string) (*ffProc, error) {
	bin := ffmpegPath
	for _, a := range args {
		if a == "kmsgrab" && ffmpegKMS != "" && !isAdmin() {
			bin = ffmpegKMS
		}
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL} // die with us
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &ffProc{Stderr: stderr, wait: func() int {
		cmd.Wait()
		if cmd.ProcessState == nil {
			return -1
		}
		return cmd.ProcessState.ExitCode()
	}}, nil
}

var encoderOrder = []string{"h264_nvenc", "h264_vaapi", "h264_qsv", "libx264"}

// encoderCandidates: alt=false → x11grab (X11, any GPU); alt=true → kmsgrab + VAAPI
// (Wayland or login screen; Intel/AMD). On Wayland, KMS is tried first.
func encoderCandidates(order []string) []encChoice {
	var x11, kms []encChoice
	for _, e := range order {
		x11 = append(x11, encChoice{e, false, false})
	}
	kms = append(kms, encChoice{"h264_vaapi", true, true})
	if isWayland() {
		return append(kms, x11...)
	}
	return append(x11, kms...)
}

func fallbackChoice() encChoice { return encChoice{"libx264", false, false} }

func renderNode() string {
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	if len(nodes) > 0 {
		return nodes[0]
	}
	return "/dev/dri/renderD128"
}

func drmCard() string {
	cards, _ := filepath.Glob("/dev/dri/card*")
	if len(cards) > 0 {
		return cards[0]
	}
	return "/dev/dri/card0"
}

func (v *videoPipeline) buildArgs(enc string, zeroCopy bool, kms bool, out string, probe bool) []string {
	m := v.monitor()
	fps := v.cfg.FPS
	if fps <= 0 {
		fps = 60
	}
	w, h := v.outputSize()
	scaled := w != int(m.W)&^1 || h != int(m.H)&^1
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-fflags", "nobuffer", "-flags", "low_delay"}
	var vf []string
	if kms {
		args = append(args, "-device", drmCard(), "-framerate", itoa(fps), "-f", "kmsgrab", "-i", "-")
		vf = append(vf, "hwmap=derive_device=vaapi", fmt.Sprintf("scale_vaapi=w=%d:h=%d:format=nv12", w, h))
		enc = "h264_vaapi"
	} else {
		if enc == "h264_vaapi" {
			args = append(args, "-vaapi_device", renderNode())
		}
		args = append(args, "-f", "x11grab", "-framerate", itoa(fps), "-draw_mouse", "0",
			"-video_size", fmt.Sprintf("%dx%d", m.W, m.H), "-i", fmt.Sprintf("%s+%d,%d", xDisplay(), m.X, m.Y))
		if scaled {
			vf = append(vf, fmt.Sprintf("scale=%d:%d:flags=bilinear", w, h))
		}
		vf = append(vf, "format=nv12")
		if enc == "h264_vaapi" {
			vf = append(vf, "hwupload")
		}
	}
	args = append(args, "-vf", strings.Join(vf, ","))
	br := v.cfg.Bitrate
	if br <= 0 {
		br = 20000
	}
	buf := br * 2 / fps
	if buf < 200 {
		buf = 200
	}
	rate := []string{"-b:v", itoa(br) + "k", "-maxrate", itoa(br) + "k", "-bufsize", itoa(buf) + "k", "-g", itoa(fps * 4), "-bf", "0"}
	switch enc {
	case "h264_nvenc":
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p2", "-tune", "ull", "-zerolatency", "1",
			"-delay", "0", "-rc", "cbr", "-rc-lookahead", "0", "-profile:v", "high", "-forced-idr", "1")
	case "h264_vaapi":
		args = append(args, "-c:v", "h264_vaapi", "-rc_mode", "CBR", "-profile:v", "high", "-async_depth", "1")
	case "h264_qsv":
		args = append(args, "-c:v", "h264_qsv", "-preset", "veryfast", "-look_ahead", "0", "-async_depth", "1", "-profile:v", "high")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "high")
	}
	args = append(args, rate...)
	if probe {
		return append(args, "-frames:v", "3", "-f", "null", "-")
	}
	return append(args, "-bsf:v", "dump_extra=freq=keyframe", "-an", "-f", "rtp", "-payload_type", "96", out)
}

// runOnce streams system audio: the PulseAudio/PipeWire monitor of the default output.
func (a *audioPipeline) runOnce(ctx context.Context) error {
	out := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1400&rtcpport=%d", a.port, a.port)
	p, err := ffStart(ctx, []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-fflags", "nobuffer",
		"-f", "pulse", "-fragment_size", "1920", "-i", "@DEFAULT_MONITOR@",
		"-ac", "2", "-ar", "48000", "-c:a", "libopus", "-b:a", "128k", "-application", "lowdelay",
		"-frame_duration", "10", "-vbr", "on", "-f", "rtp", "-payload_type", "111", out}, "")
	if err != nil {
		return err
	}
	msg := make([]byte, 2048)
	n, _ := p.Stderr.Read(msg)
	code := p.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("ffmpeg audio exited (%d): %s", code, strings.TrimSpace(string(msg[:max(n, 0)])))
}
