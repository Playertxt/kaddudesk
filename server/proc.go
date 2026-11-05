package main

import (
	"context"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// deskProc is a child process started on a specific desktop (e.g. winsta0\Winlogon).
// os/exec can't set STARTUPINFO.lpDesktop, which service mode needs so ffmpeg's
// Desktop Duplication runs on the lock screen / UAC desktop.
type deskProc struct {
	h      windows.Handle
	Stderr *os.File
	done   chan struct{}
}

func startOnDesktop(ctx context.Context, exe string, args []string, desktop string) (*deskProc, error) {
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	var r, w windows.Handle
	if err := windows.CreatePipe(&r, &w, sa, 0); err != nil {
		return nil, err
	}
	windows.SetHandleInformation(r, windows.HANDLE_FLAG_INHERIT, 0)
	nulName, _ := windows.UTF16PtrFromString("NUL")
	nul, err := windows.CreateFile(nulName, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, sa, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		windows.CloseHandle(r)
		windows.CloseHandle(w)
		return nil, err
	}
	parts := []string{syscall.EscapeArg(exe)}
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	cmd, _ := windows.UTF16PtrFromString(strings.Join(parts, " "))
	si := windows.StartupInfo{
		Flags:      windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_HIDE,
		StdInput:   nul, StdOutput: nul, StdErr: w,
	}
	si.Cb = uint32(unsafe.Sizeof(si))
	if desktop != "" {
		si.Desktop, _ = windows.UTF16PtrFromString(desktop)
	}
	var pi windows.ProcessInformation
	syscall.ForkLock.Lock() // keep other spawns from inheriting our pipe end
	err = windows.CreateProcess(nil, cmd, nil, nil, true, windows.CREATE_NO_WINDOW, nil, nil, &si, &pi)
	syscall.ForkLock.Unlock()
	windows.CloseHandle(w)
	windows.CloseHandle(nul)
	if err != nil {
		windows.CloseHandle(r)
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	if jobHandle != 0 {
		windows.AssignProcessToJobObject(jobHandle, pi.Process)
	}
	p := &deskProc{h: pi.Process, Stderr: os.NewFile(uintptr(r), "ffmpeg-stderr"), done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			windows.TerminateProcess(pi.Process, 1)
		case <-p.done:
		}
	}()
	return p, nil
}

// Wait blocks until the process exits and returns its exit code.
func (p *deskProc) Wait() uint32 {
	windows.WaitForSingleObject(p.h, windows.INFINITE)
	var code uint32
	windows.GetExitCodeProcess(p.h, &code)
	close(p.done)
	windows.CloseHandle(p.h)
	p.Stderr.Close()
	return code
}
