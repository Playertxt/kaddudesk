package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================ ffmpeg helpers

var ffmpegPath string

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

// ============================================================ RTP H.264 depacketizer

type depack struct {
	buf     []byte
	lastSeq uint16
	haveSeq bool
	broken  bool
	key     bool
}

var startCode = []byte{0, 0, 0, 1}

// push consumes one RTP packet. When an access unit is complete it returns it.
func (d *depack) push(pkt []byte) (frame []byte, key bool, ok bool) {
	if len(pkt) < 12 || pkt[0]>>6 != 2 {
		return nil, false, false
	}
	pt := pkt[1] & 0x7f
	if pt >= 72 && pt <= 76 { // RTCP
		return nil, false, false
	}
	marker := pkt[1]&0x80 != 0
	seq := binary.BigEndian.Uint16(pkt[2:])
	if d.haveSeq && seq != d.lastSeq+1 {
		d.broken = true
	}
	d.lastSeq, d.haveSeq = seq, true
	hdr := 12 + 4*int(pkt[0]&0x0f)
	if pkt[0]&0x10 != 0 && len(pkt) >= hdr+4 {
		hdr += 4 + 4*int(binary.BigEndian.Uint16(pkt[hdr+2:]))
	}
	end := len(pkt)
	if pkt[0]&0x20 != 0 {
		end -= int(pkt[end-1])
	}
	if hdr >= end {
		return nil, false, false
	}
	p := pkt[hdr:end]
	switch t := p[0] & 0x1f; {
	case t >= 1 && t <= 23:
		d.addNal(p)
	case t == 24: // STAP-A
		for i := 1; i+2 <= len(p); {
			n := int(binary.BigEndian.Uint16(p[i:]))
			i += 2
			if i+n > len(p) {
				break
			}
			d.addNal(p[i : i+n])
			i += n
		}
	case t == 28: // FU-A
		if len(p) < 2 {
			break
		}
		if p[1]&0x80 != 0 {
			d.addNal([]byte{(p[0] & 0xe0) | (p[1] & 0x1f)})
		}
		d.buf = append(d.buf, p[2:]...)
	}
	if !marker {
		return nil, false, false
	}
	frame, key = d.buf, d.key
	broken := d.broken
	d.buf, d.key, d.broken = nil, false, false
	if broken || len(frame) == 0 {
		return nil, false, false
	}
	return frame, key, true
}

func (d *depack) addNal(n []byte) {
	if len(n) == 0 {
		return
	}
	if t := n[0] & 0x1f; t == 5 || t == 7 {
		d.key = true
	}
	d.buf = append(d.buf, startCode...)
	d.buf = append(d.buf, n...)
}

// ============================================================ Video pipeline

type VideoCfg struct {
	Monitor   int `json:"monitor"`
	FPS       int `json:"fps"`
	Bitrate   int `json:"bitrate"`   // kbps
	MaxHeight int `json:"maxHeight"` // 0 = native
}

type Frame struct {
	Data []byte
	Key  bool
	TS   int64 // server receive time, µs (unix)
	Seq  uint32
}

type videoPipeline struct {
	mu        sync.Mutex
	cfg       VideoCfg
	running   bool
	cancel    context.CancelFunc
	encoder   string
	zeroCopy  bool
	subs      map[*videoSub]struct{}
	conn      *net.UDPConn
	port      int
	gen       atomic.Int64
	lastKeyRq time.Time
	frames    atomic.Int64
	bytes     atomic.Int64
	seq       atomic.Uint32
	lastErr   string
	failCount int
	monitors  []Monitor
	statsFPS  float64
	statsKbps float64
	desktop   string // input desktop name being captured ("Default", "Winlogon", ...)
}

// captureDesktop is the STARTUPINFO desktop for ffmpeg. Only the SYSTEM agent may
// attach to the secure desktop; in normal mode ffmpeg inherits ours.
func (v *videoPipeline) captureDesktop() string {
	if agentMode && v.desktop != "" {
		return `winsta0\` + v.desktop
	}
	return ""
}

// setDesktop is called when the input desktop changes (lock screen, UAC, sign-in).
func (v *videoPipeline) setDesktop(name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.desktop == name {
		return
	}
	v.desktop = name
	if agentMode && v.running {
		v.lastKeyRq = time.Now()
		v.startLocked()
	}
}

var video = &videoPipeline{subs: map[*videoSub]struct{}{}}

// videoSub is one viewer's frame queue. If it overflows (slow network), frames are
// dropped and the viewer waits for the next keyframe instead of building latency.
type videoSub struct {
	ch      chan Frame
	dropped atomic.Bool
}

var encoderOrder = []string{"h264_nvenc", "h264_amf", "h264_qsv", "libx264"}

func (v *videoPipeline) init() error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return err
	}
	_ = c.SetReadBuffer(16 << 20)
	v.conn = c
	v.port = c.LocalAddr().(*net.UDPAddr).Port
	v.monitors = listMonitors()
	c0 := getConfig()
	v.cfg = VideoCfg{FPS: c0.DefaultFPS, Bitrate: c0.DefaultBitrate}
	go v.readLoop()
	go v.statsLoop()
	return nil
}

func (v *videoPipeline) statsLoop() {
	t := time.NewTicker(time.Second)
	for range t.C {
		f, b := v.frames.Swap(0), v.bytes.Swap(0)
		v.mu.Lock()
		v.statsFPS, v.statsKbps = float64(f), float64(b)*8/1000
		v.mu.Unlock()
	}
}

func (v *videoPipeline) readLoop() {
	buf := make([]byte, 65536)
	var d depack
	gen := v.gen.Load()
	for {
		n, _, err := v.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue // WSAECONNRESET etc.
		}
		if g := v.gen.Load(); g != gen { // encoder restarted → reset state
			gen, d = g, depack{}
		}
		frame, key, ok := d.push(buf[:n])
		if !ok {
			continue
		}
		f := Frame{Data: frame, Key: key, TS: time.Now().UnixMicro(), Seq: v.seq.Add(1)}
		v.frames.Add(1)
		v.bytes.Add(int64(len(frame)))
		v.mu.Lock()
		for sub := range v.subs {
			select {
			case sub.ch <- f:
			default: // viewer is behind; it resyncs on the next keyframe
				sub.dropped.Store(true)
			}
		}
		v.mu.Unlock()
	}
}

func (v *videoPipeline) subscribe() *videoSub {
	sub := &videoSub{ch: make(chan Frame, 6)}
	v.mu.Lock()
	v.subs[sub] = struct{}{}
	v.mu.Unlock()
	return sub
}

func (v *videoPipeline) unsubscribe(sub *videoSub) {
	v.mu.Lock()
	delete(v.subs, sub)
	empty := len(v.subs) == 0
	v.mu.Unlock()
	if empty {
		go func() {
			time.Sleep(3 * time.Second) // keep warm briefly for quick reconnects
			v.mu.Lock()
			if len(v.subs) == 0 {
				v.stopLocked()
			}
			v.mu.Unlock()
		}()
	}
}

// configure (re)starts the encoder with cfg. Restarting also yields an instant keyframe.
func (v *videoPipeline) configure(c VideoCfg) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if c.Monitor < 0 || c.Monitor >= len(v.monitors) {
		c.Monitor = 0
	}
	c.FPS = clamp(c.FPS, 5, 144)
	c.Bitrate = clamp(c.Bitrate, 300, 150000)
	if c.MaxHeight < 0 {
		c.MaxHeight = 0
	}
	if v.running && c == v.cfg {
		return false
	}
	v.cfg = c
	v.lastKeyRq = time.Now()
	v.startLocked()
	return true
}

// requestKeyframe restarts the encoder (debounced). ffmpeg has no external IDR trigger.
func (v *videoPipeline) requestKeyframe() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Since(v.lastKeyRq) < 700*time.Millisecond {
		return
	}
	v.lastKeyRq = time.Now()
	v.startLocked()
}

func (v *videoPipeline) stopLocked() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.running = false
}

func (v *videoPipeline) monitor() Monitor {
	if v.cfg.Monitor < len(v.monitors) {
		return v.monitors[v.cfg.Monitor]
	}
	return v.monitors[0]
}

func (v *videoPipeline) outputSize() (int, int) {
	m := v.monitor()
	w, h := int(m.W), int(m.H)
	if v.cfg.MaxHeight > 0 && h > v.cfg.MaxHeight {
		w = w * v.cfg.MaxHeight / h
		h = v.cfg.MaxHeight
	}
	return w &^ 1, h &^ 1
}

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

type encChoice struct {
	enc      string
	zeroCopy bool
	gdi      bool
}

var (
	probedMu     sync.Mutex
	probedChoice *encChoice
)

// probeEncoder finds the fastest working capture+encoder combo on this PC.
func (v *videoPipeline) probeEncoder() encChoice {
	probedMu.Lock()
	defer probedMu.Unlock()
	if probedChoice != nil {
		return *probedChoice
	}
	order := encoderOrder
	if e := getConfig().Encoder; e != "" && e != "auto" {
		order = append([]string{e}, encoderOrder...)
	}
	try := func(c encChoice) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		v.mu.Lock()
		args, desk := v.buildArgs(c.enc, c.zeroCopy, c.gdi, "", true), v.captureDesktop()
		v.mu.Unlock()
		p, err := startOnDesktop(ctx, ffmpegPath, args, desk)
		if err != nil {
			log.Printf("probe start: %v", err)
			return false
		}
		out, _ := io.ReadAll(io.LimitReader(p.Stderr, 4096))
		if code := p.Wait(); code != 0 {
			log.Printf("probe %s zc=%v gdi=%v failed (%d): %s", c.enc, c.zeroCopy, c.gdi, code, strings.TrimSpace(string(out)))
			return false
		}
		return true
	}
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
	for _, c := range cands {
		if try(c) {
			log.Printf("encoder selected: %s (zero-copy=%v, gdigrab=%v)", c.enc, c.zeroCopy, c.gdi)
			probedChoice = &c
			return c
		}
	}
	c := encChoice{"libx264", false, true}
	probedChoice = &c
	return c
}

func resetEncoderProbe() {
	probedMu.Lock()
	probedChoice = nil
	probedMu.Unlock()
}

func (v *videoPipeline) startLocked() {
	v.stopLocked()
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.running = true
	go v.run(ctx)
}

func (v *videoPipeline) run(ctx context.Context) {
	choice := v.probeEncoder()
	v.mu.Lock()
	v.encoder = choice.enc
	v.zeroCopy = choice.zeroCopy
	v.mu.Unlock()
	quickFails := 0
	for ctx.Err() == nil {
		v.mu.Lock()
		out := fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=32000&rtcpport=%d", v.port, v.port)
		args := v.buildArgs(choice.enc, choice.zeroCopy, choice.gdi, out, false)
		desk := v.captureDesktop()
		v.mu.Unlock()
		v.gen.Add(1)
		started := time.Now()
		p, err := startOnDesktop(ctx, ffmpegPath, args, desk)
		if err != nil {
			v.lastErr = err.Error()
			log.Printf("ffmpeg start: %v", err)
			return
		}
		var errLines []string
		sc := bufio.NewScanner(p.Stderr)
		for sc.Scan() {
			l := sc.Text()
			if len(errLines) < 20 {
				errLines = append(errLines, l)
			}
		}
		code := p.Wait()
		if ctx.Err() != nil {
			return
		}
		v.lastErr = strings.Join(errLines, " | ")
		log.Printf("ffmpeg exited (%d) on %q: %s", code, desk, v.lastErr)
		secure := inputDesktopName() != "Default"
		switch {
		case secure && !agentMode:
			// Lock screen / UAC prompt: only service mode can capture it. Wait it out
			// instead of counting failures (which would downgrade the encoder).
			quickFails = 0
		case time.Since(started) < 3*time.Second:
			quickFails++
		default:
			quickFails = 0
		}
		if quickFails >= 3 {
			// Something changed (driver, desktop, monitor). Re-probe and fall back.
			resetEncoderProbe()
			v.mu.Lock()
			v.monitors = listMonitors()
			v.mu.Unlock()
			choice = v.probeEncoder()
			quickFails = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(400 * time.Millisecond): // e.g. desktop switch (UAC/lock); retry
		}
	}
}

// ============================================================ Audio pipeline (WASAPI loopback → Opus)

type audioPipeline struct {
	mu      sync.Mutex
	subs    map[chan []byte]struct{}
	cancel  context.CancelFunc
	conn    *net.UDPConn
	port    int
	running bool
}

var audio = &audioPipeline{subs: map[chan []byte]struct{}{}}

func (a *audioPipeline) init() error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return err
	}
	_ = c.SetReadBuffer(2 << 20)
	a.conn = c
	a.port = c.LocalAddr().(*net.UDPAddr).Port
	go a.readLoop()
	return nil
}

func (a *audioPipeline) readLoop() {
	buf := make([]byte, 8192)
	for {
		n, _, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if n < 12 || buf[0]>>6 != 2 {
			continue
		}
		if pt := buf[1] & 0x7f; pt >= 72 && pt <= 76 {
			continue
		}
		hdr := 12 + 4*int(buf[0]&0x0f)
		if hdr >= n {
			continue
		}
		pkt := append([]byte(nil), buf[hdr:n]...)
		a.mu.Lock()
		for ch := range a.subs {
			select {
			case ch <- pkt:
			default:
			}
		}
		a.mu.Unlock()
	}
}

func (a *audioPipeline) subscribe() chan []byte {
	ch := make(chan []byte, 32)
	a.mu.Lock()
	a.subs[ch] = struct{}{}
	if !a.running {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel, a.running = cancel, true
		go a.run(ctx)
	}
	a.mu.Unlock()
	return ch
}

func (a *audioPipeline) unsubscribe(ch chan []byte) {
	a.mu.Lock()
	delete(a.subs, ch)
	if len(a.subs) == 0 && a.running {
		a.cancel()
		a.running = false
	}
	a.mu.Unlock()
}

func (a *audioPipeline) run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := a.runOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("audio: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second): // device changed / no device: retry
		}
	}
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

// ============================================================ small utils

func itoa(i int) string { return fmt.Sprint(i) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
