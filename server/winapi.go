package main

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dxgiDLL  = windows.NewLazySystemDLL("dxgi.dll")
	powrprof = windows.NewLazySystemDLL("powrprof.dll")

	pSendInput                     = user32.NewProc("SendInput")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetCursorInfo                 = user32.NewProc("GetCursorInfo")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pOpenClipboard                 = user32.NewProc("OpenClipboard")
	pCloseClipboard                = user32.NewProc("CloseClipboard")
	pEmptyClipboard                = user32.NewProc("EmptyClipboard")
	pGetClipboardData              = user32.NewProc("GetClipboardData")
	pSetClipboardData              = user32.NewProc("SetClipboardData")
	pGetClipboardSequenceNumber    = user32.NewProc("GetClipboardSequenceNumber")
	pLockWorkStation               = user32.NewProc("LockWorkStation")
	pMessageBoxTimeoutW            = user32.NewProc("MessageBoxTimeoutW")
	pEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pOpenInputDesktop              = user32.NewProc("OpenInputDesktop")
	pSetThreadDesktop              = user32.NewProc("SetThreadDesktop")
	pCloseDesktop                  = user32.NewProc("CloseDesktop")
	pGetUserObjectInformationW     = user32.NewProc("GetUserObjectInformationW")
	pVkKeyScanW                    = user32.NewProc("VkKeyScanW")
	pGlobalAlloc                   = kernel32.NewProc("GlobalAlloc")
	pGlobalLock                    = kernel32.NewProc("GlobalLock")
	pGlobalUnlock                  = kernel32.NewProc("GlobalUnlock")
	pGlobalFree                    = kernel32.NewProc("GlobalFree")
	pCreateDXGIFactory1            = dxgiDLL.NewProc("CreateDXGIFactory1")
	pSetSuspendState               = powrprof.NewProc("SetSuspendState")
)

func setDPIAware() {
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4 → physical pixel coordinates everywhere.
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(uintptr(^uintptr(3)))
	}
}

// ---------------------------------------------------------------- input

const (
	inputMouse    = 0
	inputKeyboard = 1

	mMove       = 0x0001
	mLeftDown   = 0x0002
	mLeftUp     = 0x0004
	mRightDown  = 0x0008
	mRightUp    = 0x0010
	mMiddleDown = 0x0020
	mMiddleUp   = 0x0040
	mXDown      = 0x0080
	mXUp        = 0x0100
	mWheel      = 0x0800
	mHWheel     = 0x1000
	mVirtDesk   = 0x4000
	mAbsolute   = 0x8000

	kExtended = 0x0001
	kUp       = 0x0002
	kUnicode  = 0x0004
	kScancode = 0x0008
)

type rawInput struct {
	Type uint32
	_    uint32
	U    [32]byte
}

type mouseInput struct {
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

type keybdInput struct {
	Vk, Scan uint16
	Flags    uint32
	Time     uint32
	Extra    uintptr
}

func mouseEv(dx, dy int32, data, flags uint32) rawInput {
	var in rawInput
	in.Type = inputMouse
	m := (*mouseInput)(unsafe.Pointer(&in.U[0]))
	m.Dx, m.Dy, m.MouseData, m.Flags = dx, dy, data, flags
	return in
}

func keyEv(vk, scan uint16, flags uint32) rawInput {
	var in rawInput
	in.Type = inputKeyboard
	k := (*keybdInput)(unsafe.Pointer(&in.U[0]))
	k.Vk, k.Scan, k.Flags = vk, scan, flags
	return in
}

func sendInputs(ins ...rawInput) {
	if len(ins) == 0 {
		return
	}
	r, _, _ := pSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	if int(r) != len(ins) {
		// Input desktop may have changed (UAC / lock screen). Try to follow it and retry once.
		if syncInputDesktop() {
			pSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
		}
	}
}

// syncInputDesktop attaches the calling thread to the current input desktop.
// Works fully when running as SYSTEM (service mode); as admin it reaches elevated apps.
func syncInputDesktop() bool {
	const genericAll = 0x10000000
	h, _, _ := pOpenInputDesktop.Call(0, 0, genericAll)
	if h == 0 {
		return false
	}
	r, _, _ := pSetThreadDesktop.Call(h)
	if r == 0 {
		pCloseDesktop.Call(h)
		return false
	}
	return true
}

func inputDesktopName() string {
	const genericRead = 0x80000000
	h, _, _ := pOpenInputDesktop.Call(0, 0, genericRead)
	if h == 0 {
		return "Winlogon" // access denied usually means the secure desktop
	}
	defer pCloseDesktop.Call(h)
	var buf [128]uint16
	var n uint32
	pGetUserObjectInformationW.Call(h, 2 /*UOI_NAME*/, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&n)))
	return windows.UTF16ToString(buf[:])
}

func sysMetric(i int) int32 {
	r, _, _ := pGetSystemMetrics.Call(uintptr(i))
	return int32(r)
}

// absolute move to physical virtual-desktop coordinates
func mouseMoveAbs(x, y int32) rawInput {
	vx, vy := sysMetric(76), sysMetric(77)
	vw, vh := sysMetric(78), sysMetric(79)
	if vw <= 1 || vh <= 1 {
		return mouseEv(0, 0, 0, 0)
	}
	nx := int32((int64(x-vx)*65535 + int64(vw-1)/2) / int64(vw-1))
	ny := int32((int64(y-vy)*65535 + int64(vh-1)/2) / int64(vh-1))
	return mouseEv(nx, ny, 0, mMove|mAbsolute|mVirtDesk)
}

func mouseButton(btn int, down bool) rawInput {
	var f, data uint32
	switch btn {
	case 0:
		f = pick(down, mLeftDown, mLeftUp)
	case 1:
		f = pick(down, mMiddleDown, mMiddleUp)
	case 2:
		f = pick(down, mRightDown, mRightUp)
	case 3:
		f, data = pick(down, mXDown, mXUp), 1
	case 4:
		f, data = pick(down, mXDown, mXUp), 2
	}
	return mouseEv(0, 0, data, f)
}

func pick(c bool, a, b uint32) uint32 {
	if c {
		return a
	}
	return b
}

func mouseWheel(dx, dy int32) []rawInput {
	var out []rawInput
	if dy != 0 {
		out = append(out, mouseEv(0, 0, uint32(-dy), mWheel))
	}
	if dx != 0 {
		out = append(out, mouseEv(0, 0, uint32(dx), mHWheel))
	}
	return out
}

// keyByCode presses/releases a key identified by a DOM KeyboardEvent.code.
func keyByCode(code string, down bool) {
	k, ok := keyMap[code]
	if !ok {
		return
	}
	var flags uint32
	if !down {
		flags |= kUp
	}
	if k.vk != 0 {
		if k.ext {
			flags |= kExtended
		}
		sendInputs(keyEv(k.vk, k.sc, flags))
		return
	}
	flags |= kScancode
	if k.ext {
		flags |= kExtended
	}
	sendInputs(keyEv(0, k.sc, flags))
}

func typeText(s string) {
	var ins []rawInput
	for _, r := range s {
		if r == '\n' || r == '\r' {
			ins = append(ins, keyEv(0, 0x1C, kScancode), keyEv(0, 0x1C, kScancode|kUp))
			continue
		}
		if r == '\t' {
			ins = append(ins, keyEv(0, 0x0F, kScancode), keyEv(0, 0x0F, kScancode|kUp))
			continue
		}
		for _, u := range windows.StringToUTF16(string(r)) {
			if u == 0 {
				continue
			}
			ins = append(ins, keyEv(0, u, kUnicode), keyEv(0, u, kUnicode|kUp))
		}
	}
	sendInputs(ins...)
}

// ---------------------------------------------------------------- cursor shape

type cursorInfo struct {
	Size    uint32
	Flags   uint32
	HCursor uintptr
	X, Y    int32
}

var cursorNames map[uintptr]string

func initCursors() {
	ids := map[uintptr]string{
		32512: "default", 32513: "text", 32514: "wait", 32515: "crosshair", 32516: "default",
		32642: "nwse-resize", 32643: "nesw-resize", 32644: "ew-resize", 32645: "ns-resize",
		32646: "move", 32648: "not-allowed", 32649: "pointer", 32650: "progress", 32651: "help",
	}
	cursorNames = map[uintptr]string{}
	for id, n := range ids {
		h, _, _ := pLoadCursorW.Call(0, id)
		if h != 0 {
			cursorNames[h] = n
		}
	}
}

// getCursor returns the CSS cursor name, position and visibility.
func getCursor() (string, int32, int32, bool) {
	ci := cursorInfo{Size: uint32(unsafe.Sizeof(cursorInfo{}))}
	r, _, _ := pGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci)))
	if r == 0 {
		return "default", 0, 0, true
	}
	if ci.Flags&1 == 0 {
		return "none", ci.X, ci.Y, false
	}
	n, ok := cursorNames[ci.HCursor]
	if !ok {
		n = "default"
	}
	return n, ci.X, ci.Y, true
}

// ---------------------------------------------------------------- monitors (DXGI)

type Monitor struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Adapter int    `json:"adapter"`
	Output  int    `json:"output"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	W       int32  `json:"w"`
	H       int32  `json:"h"`
	Primary bool   `json:"primary"`
	GPU     string `json:"gpu"`
}

type rect struct{ L, T, R, B int32 }

type dxgiOutputDesc struct {
	DeviceName [32]uint16
	Desktop    rect
	Attached   int32
	Rotation   uint32
	Monitor    uintptr
}

type dxgiAdapterDesc struct {
	Description [128]uint16
	VendorID    uint32
	DeviceID    uint32
	SubSysID    uint32
	Revision    uint32
	DedVid      uintptr
	DedSys      uintptr
	Shared      uintptr
	Luid        [8]byte
}

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

var iidFactory1 = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}

func listMonitors() []Monitor {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var out []Monitor
	if pCreateDXGIFactory1.Find() == nil {
		var factory uintptr
		hr, _, _ := pCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidFactory1)), uintptr(unsafe.Pointer(&factory)))
		if hr == 0 && factory != 0 {
			for a := 0; a < 16; a++ {
				var adapter uintptr
				if comCall(factory, 12, uintptr(a), uintptr(unsafe.Pointer(&adapter))) != 0 || adapter == 0 {
					break
				}
				var ad dxgiAdapterDesc
				comCall(adapter, 8, uintptr(unsafe.Pointer(&ad)))
				gpu := windows.UTF16ToString(ad.Description[:])
				for o := 0; o < 16; o++ {
					var output uintptr
					if comCall(adapter, 7, uintptr(o), uintptr(unsafe.Pointer(&output))) != 0 || output == 0 {
						break
					}
					var d dxgiOutputDesc
					comCall(output, 7, uintptr(unsafe.Pointer(&d)))
					comRelease(output)
					if d.Attached == 0 {
						continue
					}
					out = append(out, Monitor{
						Index: len(out), Name: windows.UTF16ToString(d.DeviceName[:]),
						Adapter: a, Output: o,
						X: d.Desktop.L, Y: d.Desktop.T, W: d.Desktop.R - d.Desktop.L, H: d.Desktop.B - d.Desktop.T,
						Primary: d.Desktop.L == 0 && d.Desktop.T == 0, GPU: gpu,
					})
				}
				comRelease(adapter)
			}
			comRelease(factory)
		}
	}
	if len(out) == 0 {
		w, h := sysMetric(0), sysMetric(1)
		out = append(out, Monitor{Index: 0, Name: "Primary", X: 0, Y: 0, W: w, H: h, Primary: true})
	}
	return out
}

// ---------------------------------------------------------------- clipboard

func withClipboard(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var ok uintptr
	for i := 0; i < 10; i++ {
		ok, _, _ = pOpenClipboard.Call(0)
		if ok != 0 {
			break
		}
		windows.SleepEx(20, false)
	}
	if ok == 0 {
		return errors.New("clipboard busy")
	}
	defer pCloseClipboard.Call()
	return fn()
}

func getClipboardText() (string, error) {
	var s string
	err := withClipboard(func() error {
		h, _, _ := pGetClipboardData.Call(13)
		if h == 0 {
			return nil
		}
		p, _, _ := pGlobalLock.Call(h)
		if p == 0 {
			return errors.New("lock failed")
		}
		defer pGlobalUnlock.Call(h)
		s = windows.UTF16PtrToString((*uint16)(unsafe.Pointer(p)))
		return nil
	})
	return s, err
}

func setClipboardText(s string) error {
	u := windows.StringToUTF16(s)
	return withClipboard(func() error {
		pEmptyClipboard.Call()
		h, _, _ := pGlobalAlloc.Call(0x0002 /*GMEM_MOVEABLE*/, uintptr(len(u)*2))
		if h == 0 {
			return errors.New("alloc failed")
		}
		p, _, _ := pGlobalLock.Call(h)
		copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
		pGlobalUnlock.Call(h)
		if r, _, _ := pSetClipboardData.Call(13, h); r == 0 {
			pGlobalFree.Call(h)
			return errors.New("set failed")
		}
		return nil
	})
}

func clipboardSeq() uint32 {
	r, _, _ := pGetClipboardSequenceNumber.Call()
	return uint32(r)
}

// ---------------------------------------------------------------- power & misc

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 /*CREATE_NO_WINDOW*/}
	return c
}

func powerAction(a string) error {
	if agentMode { // we are SYSTEM: user-facing actions must run as the signed-in user
		switch a {
		case "lock":
			return runAsUser("rundll32.exe user32.dll,LockWorkStation")
		case "signout":
			return logoffConsole()
		case "taskmgr":
			return runAsUser("taskmgr.exe")
		case "explorer":
			return runAsUser("explorer.exe")
		case "cmd":
			return runAsUser("cmd.exe")
		case "settings":
			return runAsUser(`explorer.exe "ms-settings:"`)
		}
	}
	switch a {
	case "lock":
		pLockWorkStation.Call()
	case "sleep":
		go func() { windows.SleepEx(800, false); pSetSuspendState.Call(0, 0, 0) }()
	case "restart":
		return hiddenCmd("shutdown.exe", "/r", "/t", "3").Start()
	case "shutdown":
		return hiddenCmd("shutdown.exe", "/s", "/t", "3").Start()
	case "signout":
		return hiddenCmd("shutdown.exe", "/l").Start()
	case "taskmgr":
		return exec.Command("taskmgr.exe").Start()
	case "explorer":
		return exec.Command("explorer.exe").Start()
	case "cmd":
		return exec.Command("cmd.exe", "/c", "start", "cmd.exe").Start()
	case "settings":
		return exec.Command("cmd.exe", "/c", "start", "ms-settings:").Start()
	default:
		return errors.New("unknown action")
	}
	return nil
}

// askApproval shows a top-most Yes/No box on the host. Returns true if allowed.
func askApproval(title, msg string, timeoutMs uint32) bool {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	const flags = 0x4 /*YESNO*/ | 0x20 /*ICONQUESTION*/ | 0x100 /*DEFBUTTON2*/ | 0x40000 /*TOPMOST*/ | 0x10000 /*SETFOREGROUND*/ | 0x1000 /*SYSTEMMODAL*/
	r, _, _ := pMessageBoxTimeoutW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags, 0, uintptr(timeoutMs))
	return r == 6 // IDYES
}

func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS, 0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	ok, err := windows.Token(0).IsMember(sid)
	return err == nil && ok
}
