package main

// Input on Linux goes through /dev/uinput: the kernel sees a real keyboard, mouse and
// absolute pointer, so it works the same under X11 and Wayland (and on the login screen).
// Access needs root or the udev rule installed by install-linux.sh.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02
	evAbs = 0x03

	relX, relY, relHWheel, relWheel, relWheelHiRes, relHWheelHiRes = 0x00, 0x01, 0x06, 0x08, 0x0b, 0x0c
	absX, absY                                                     = 0x00, 0x01
	absMax                                                         = 65535

	btnLeft, btnRight, btnMiddle, btnSide, btnExtra = 0x110, 0x111, 0x112, 0x113, 0x114

	uiSetEvBit      = 0x40045564
	uiSetKeyBit     = 0x40045565
	uiSetRelBit     = 0x40045566
	uiSetAbsBit     = 0x40045567
	uiSetPropBit    = 0x4004556e
	uiDevCreate     = 0x5501
	inputPropDirect = 0x01
)

type uinputDev struct {
	f *os.File
}

var (
	inMu      sync.Mutex
	kbdMouse  *uinputDev // keys + relative mouse + wheel
	absPtr    *uinputDev // absolute pointer + buttons
	curAbsX   int32
	curAbsY   int32
	inputErr  error
	wheelAccY int32
	wheelAccX int32
)

// uinput_user_dev (legacy setup ABI, supported by every kernel)
type uinputUserDev struct {
	Name         [80]byte
	Bustype      uint16
	Vendor       uint16
	Product      uint16
	Version      uint16
	FFEffectsMax uint32
	AbsMax       [64]int32
	AbsMin       [64]int32
	AbsFuzz      [64]int32
	AbsFlat      [64]int32
}

func newUinput(name string, setup func(fd uintptr) error, abs bool) (*uinputDev, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	fd := f.Fd()
	if err := setup(fd); err != nil {
		f.Close()
		return nil, err
	}
	var d uinputUserDev
	copy(d.Name[:], name)
	d.Bustype, d.Vendor, d.Product, d.Version = 0x03, 0x4b44, 0x0001, 1 // BUS_USB, "KD"
	if abs {
		d.AbsMax[absX], d.AbsMax[absY] = absMax, absMax
	}
	b := (*[unsafe.Sizeof(d)]byte)(unsafe.Pointer(&d))[:]
	if _, err := f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if err := unix.IoctlSetInt(int(fd), uiDevCreate, 0); err != nil {
		f.Close()
		return nil, err
	}
	return &uinputDev{f: f}, nil
}

func setBits(fd uintptr, req uint, bits ...int) error {
	for _, b := range bits {
		if err := unix.IoctlSetInt(int(fd), req, b); err != nil {
			return fmt.Errorf("ioctl %x %d: %w", req, b, err)
		}
	}
	return nil
}

func inputInit() error {
	inMu.Lock()
	defer inMu.Unlock()
	var err error
	kbdMouse, err = newUinput("KadduDesk keyboard+mouse", func(fd uintptr) error {
		if err := setBits(fd, uiSetEvBit, evKey, evRel, evSyn); err != nil {
			return err
		}
		for k := 1; k < 249; k++ {
			unix.IoctlSetInt(int(fd), uiSetKeyBit, k)
		}
		if err := setBits(fd, uiSetKeyBit, btnLeft, btnRight, btnMiddle, btnSide, btnExtra); err != nil {
			return err
		}
		return setBits(fd, uiSetRelBit, relX, relY, relWheel, relHWheel, relWheelHiRes, relHWheelHiRes)
	}, false)
	if err != nil {
		inputErr = fmt.Errorf("can't open /dev/uinput (%v) — mouse/keyboard control disabled. Run install-linux.sh once, or start as root", err)
		return inputErr
	}
	absPtr, err = newUinput("KadduDesk absolute pointer", func(fd uintptr) error {
		if err := setBits(fd, uiSetEvBit, evKey, evAbs, evSyn); err != nil {
			return err
		}
		if err := setBits(fd, uiSetKeyBit, btnLeft, btnRight, btnMiddle, btnSide, btnExtra); err != nil {
			return err
		}
		if err := setBits(fd, uiSetAbsBit, absX, absY); err != nil {
			return err
		}
		return setBits(fd, uiSetPropBit, inputPropDirect)
	}, true)
	if err != nil {
		inputErr = err
		return err
	}
	time.Sleep(300 * time.Millisecond) // let udev/libinput pick the devices up
	return nil
}

func (d *uinputDev) emit(evs ...[3]int32) {
	if d == nil {
		return
	}
	buf := make([]byte, 0, 24*(len(evs)+1))
	put := func(t, c uint16, v int32) {
		var e [24]byte // struct input_event on 64-bit: timeval(16) type code value
		binary.LittleEndian.PutUint16(e[16:], t)
		binary.LittleEndian.PutUint16(e[18:], c)
		binary.LittleEndian.PutUint32(e[20:], uint32(v))
		buf = append(buf, e[:]...)
	}
	for _, e := range evs {
		put(uint16(e[0]), uint16(e[1]), e[2])
	}
	put(evSyn, 0, 0)
	d.f.Write(buf)
}

// inputMoveAbs moves to virtual-desktop pixel (x, y).
func inputMoveAbs(x, y int32) {
	vx, vy, vw, vh := virtualScreen()
	curAbsX, curAbsY = x, y
	if vw <= 1 || vh <= 1 {
		return
	}
	ax := int32(int64(x-vx) * absMax / int64(vw-1))
	ay := int32(int64(y-vy) * absMax / int64(vh-1))
	absPtr.emit([3]int32{evAbs, absX, ax}, [3]int32{evAbs, absY, ay})
}

func inputMoveRel(dx, dy int32) {
	kbdMouse.emit([3]int32{evRel, relX, dx}, [3]int32{evRel, relY, dy})
}

var linBtn = [5]int32{btnLeft, btnMiddle, btnRight, btnSide, btnExtra} // client: 0 L,1 M,2 R,3 back,4 fwd

func inputButton(b int, d bool) {
	if b < 0 || b > 4 {
		return
	}
	v := int32(0)
	if d {
		v = 1
	}
	dev := absPtr
	if dev == nil {
		dev = kbdMouse
	}
	dev.emit([3]int32{evKey, linBtn[b], v})
}

// inputWheel takes Windows-style units (120 = one notch, dy>0 = scroll down).
func inputWheel(dx, dy int32) {
	var evs [][3]int32
	if dy != 0 {
		evs = append(evs, [3]int32{evRel, relWheelHiRes, -dy})
		wheelAccY += -dy
		for wheelAccY >= 120 || wheelAccY <= -120 {
			n := int32(1)
			if wheelAccY < 0 {
				n = -1
			}
			evs = append(evs, [3]int32{evRel, relWheel, n})
			wheelAccY -= n * 120
		}
	}
	if dx != 0 {
		evs = append(evs, [3]int32{evRel, relHWheelHiRes, dx})
		wheelAccX += dx
		for wheelAccX >= 120 || wheelAccX <= -120 {
			n := int32(1)
			if wheelAccX < 0 {
				n = -1
			}
			evs = append(evs, [3]int32{evRel, relHWheel, n})
			wheelAccX -= n * 120
		}
	}
	if len(evs) > 0 {
		kbdMouse.emit(evs...)
	}
}

func keyByCode(code string, down bool) {
	k, ok := linKeys[code]
	if !ok {
		return
	}
	v := int32(0)
	if down {
		v = 1
	}
	kbdMouse.emit([3]int32{evKey, int32(k), v})
}

func tapLinKey(k int, shift bool) {
	if shift {
		kbdMouse.emit([3]int32{evKey, 42, 1})
	}
	kbdMouse.emit([3]int32{evKey, int32(k), 1})
	kbdMouse.emit([3]int32{evKey, int32(k), 0})
	if shift {
		kbdMouse.emit([3]int32{evKey, 42, 0})
	}
}

// typeText types text. ASCII goes through uinput (US layout positions); anything else
// uses xdotool (X11) or wtype (Wayland) when installed.
func typeText(s string) {
	var rest strings.Builder
	flush := func() {
		if rest.Len() == 0 {
			return
		}
		t := rest.String()
		rest.Reset()
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if p, err := exec.LookPath("wtype"); err == nil {
				exec.Command(p, t).Run()
				return
			}
		}
		if p, err := exec.LookPath("xdotool"); err == nil {
			exec.Command(p, "type", "--clearmodifiers", "--", t).Run()
		}
	}
	for _, r := range s {
		if k, ok := asciiKeys[r]; ok {
			flush()
			tapLinKey(k.code, k.shift)
			time.Sleep(2 * time.Millisecond)
			continue
		}
		rest.WriteRune(r)
	}
	flush()
}

func syncInputDesktop() bool { return true }

func inputDesktopName() string { return "Default" }

func initCursors() {}

// getCursor: Linux has no cheap portable cursor query; report where we last put it.
func getCursor() (string, int32, int32, bool) {
	return "default", curAbsX, curAbsY, true
}

var errNoUinput = errors.New("no uinput")
