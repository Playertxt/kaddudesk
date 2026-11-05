package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================ ffmpeg helpers

func findFFmpeg() error {
	exe, _ := os.Executable()
	cands := []string{
		filepath.Join(filepath.Dir(exe), "ffmpeg.exe"),
		filepath.Join(filepath.Dir(exe), "bin", "ffmpeg.exe"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			ffmpegPath = c
			return nil
		}
	}
	if p, err := extractEmbeddedFFmpeg(); err == nil && p != "" {
		ffmpegPath = p
		return nil
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		ffmpegPath = p
		return nil
	}
	return errors.New("ffmpeg.exe not found next to the server")
}

// Job object so ffmpeg children die with us, even on crash.
var jobHandle windows.Handle

func initJob() {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	jobHandle = h
}

func ffCmd(ctx context.Context, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, ffmpegPath, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c
}

func attachJob(c *exec.Cmd) {
	if jobHandle == 0 || c.Process == nil {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if err == nil {
		windows.AssignProcessToJobObject(jobHandle, ph)
		windows.CloseHandle(ph)
	}
}

// ffStart launches ffmpeg on the given desktop ("" = ours).
func ffStart(ctx context.Context, args []string, desktop string) (*ffProc, error) {
	p, err := startOnDesktop(ctx, ffmpegPath, args, desktop)
	if err != nil {
		return nil, err
	}
	return &ffProc{Stderr: p.Stderr, wait: func() int { return int(p.Wait()) }}, nil
}

var encoderOrder = []string{"h264_nvenc", "h264_amf", "h264_qsv", "libx264"}

// encoderCandidates lists capture+encoder combos to probe, fastest first.
// zeroCopy = GPU frames straight into the encoder; alt = GDI capture fallback.
func encoderCandidates(order []string) []encChoice {
	var cands []encChoice
	for _, e := range order {
		if e == "h264_nvenc" || e == "h264_amf" {
			cands = append(cands, encChoice{e, true, false})
		}
		cands = append(cands, encChoice{e, false, false})
	}
	for _, e := range order {
		cands = append(cands, encChoice{e, false, true})
	}
	return cands
}

func fallbackChoice() encChoice { return encChoice{"libx264", false, true} }

func (v *videoPipeline) buildArgs(enc string, zeroCopy bool, gdi bool, out string, probe bool) []string {
	m := v.monitor()
	fps := v.cfg.FPS
	if fps <= 0 {
		fps = 60
	}
	w, h := v.outputSize()
	scaled := w != int(m.W)&^1 || h != int(m.H)&^1
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-fflags", "nobuffer", "-flags", "low_delay"}
	var vf []string
	if gdi {
		args = append(args, "-f", "gdigrab", "-framerate", itoa(fps), "-draw_mouse", "0",
			"-offset_x", itoa(int(m.X)), "-offset_y", itoa(int(m.Y)),
			"-video_size", fmt.Sprintf("%dx%d", m.W, m.H), "-i", "desktop")
		if scaled {
			vf = append(vf, fmt.Sprintf("scale=%d:%d:flags=bilinear", w, h))
		}
		vf = append(vf, "format=nv12")
	} else {
		if m.Adapter > 0 {
			args = append(args, "-init_hw_device", "d3d11va=kd:"+itoa(m.Adapter), "-filter_hw_device", "kd")
		}
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("ddagrab=output_idx=%d:framerate=%d:draw_mouse=0", m.Output, fps))
		if !(zeroCopy && !scaled) {
			vf = append(vf, "hwdownload", "format=bgra")
			if scaled {
				vf = append(vf, fmt.Sprintf("scale=%d:%d:flags=bilinear", w, h))
			}
			vf = append(vf, "format=nv12")
		}
	}
	if len(vf) > 0 {
		args = append(args, "-vf", strings.Join(vf, ","))
	}
	br := v.cfg.Bitrate
	if br <= 0 {
		br = 20000
	}
	buf := br * 2 / fps // ~2 frames of VBV → steady frame sizes, low jitter
	if buf < 200 {
		buf = 200
	}
	gop := fps * 4
	rate := []string{"-b:v", itoa(br) + "k", "-maxrate", itoa(br) + "k", "-bufsize", itoa(buf) + "k", "-g", itoa(gop), "-bf", "0"}
	switch enc {
	case "h264_nvenc":
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p2", "-tune", "ull", "-zerolatency", "1",
			"-delay", "0", "-rc", "cbr", "-rc-lookahead", "0", "-profile:v", "high", "-forced-idr", "1")
	case "h264_amf":
		args = append(args, "-c:v", "h264_amf", "-usage", "ultralowlatency", "-quality", "speed",
			"-rc", "cbr", "-profile:v", "high")
	case "h264_qsv":
		args = append(args, "-c:v", "h264_qsv", "-preset", "veryfast", "-look_ahead", "0",
			"-async_depth", "1", "-profile:v", "high")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "high")
	}
	args = append(args, rate...)
	if probe {
		return append(args, "-frames:v", "3", "-f", "null", "-")
	}
	return append(args, "-bsf:v", "dump_extra=freq=keyframe", "-an", "-f", "rtp", "-payload_type", "96", out)
}

func (a *audioPipeline) runOnce(ctx context.Context) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fmtCh := make(chan wasapiFormat, 1)
	pr, pw := io.Pipe()
	capErr := make(chan error, 1)
	go func() { capErr <- wasapiLoopback(cctx, pw, fmtCh) }()
	var f wasapiFormat
	select {
	case f = <-fmtCh:
	case err := <-capErr:
		return err
	case <-ctx.Done():
		return nil
	}
	sampleFmt := "f32le"
	if !f.float {
		sampleFmt = fmt.Sprintf("s%dle", f.bits)
	}
	out := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1400&rtcpport=%d", a.port, a.port)
	cmd := ffCmd(cctx, "-hide_banner", "-loglevel", "error", "-fflags", "nobuffer", "-probesize", "32",
		"-analyzeduration", "0", "-f", sampleFmt, "-ar", itoa(f.rate), "-ac", itoa(f.channels), "-i", "pipe:0",
		"-ac", "2", "-ar", "48000", "-c:a", "libopus", "-b:a", "128k", "-application", "lowdelay",
		"-frame_duration", "10", "-vbr", "on", "-f", "rtp", "-payload_type", "111", out)
	cmd.Stdin = pr
	if err := cmd.Start(); err != nil {
		return err
	}
	attachJob(cmd)
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-capErr:
		cancel()
		pw.Close()
		<-waitErr
		return err
	case err := <-waitErr:
		cancel()
		pr.Close()
		return fmt.Errorf("ffmpeg audio exited: %v", err)
	}
}
