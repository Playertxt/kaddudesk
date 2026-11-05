package main

import (
	"context"
	"io"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

type wasapiFormat struct {
	rate, channels, bits int
	float                bool
}

// wasapiLoopback captures what the PC is playing (system audio) and writes raw PCM to w.
func wasapiLoopback(ctx context.Context, w *io.PipeWriter, fmtCh chan<- wasapiFormat) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_ = ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	defer ole.CoUninitialize()
	defer w.Close()

	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return err
	}
	defer mmde.Release()
	var mmd *wca.IMMDevice
	if err := mmde.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &mmd); err != nil {
		return err
	}
	defer mmd.Release()
	var ac *wca.IAudioClient
	if err := mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return err
	}
	defer ac.Release()
	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return err
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	f := wasapiFormat{rate: int(wfx.NSamplesPerSec), channels: int(wfx.NChannels), bits: int(wfx.WBitsPerSample)}
	switch wfx.WFormatTag {
	case 3:
		f.float = true
	case 0xFFFE: // WAVEFORMATEXTENSIBLE: SubFormat GUID starts at byte 24
		sub := *(*uint32)(unsafe.Add(unsafe.Pointer(wfx), 24))
		f.float = sub == 3
	}
	block := int(wfx.NBlockAlign)

	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK,
		wca.REFERENCE_TIME(200000), 0, wfx, nil); err != nil {
		return err
	}
	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return err
	}
	defer acc.Release()
	if err := ac.Start(); err != nil {
		return err
	}
	defer ac.Stop()
	fmtCh <- f

	var zeros []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		time.Sleep(5 * time.Millisecond)
		var n uint32
		if err := acc.GetNextPacketSize(&n); err != nil {
			return err
		}
		for n > 0 {
			var data *byte
			var frames, flags uint32
			var devPos, qpc uint64
			if err := acc.GetBuffer(&data, &frames, &flags, &devPos, &qpc); err != nil {
				return err
			}
			size := int(frames) * block
			var chunk []byte
			if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == nil {
				if len(zeros) < size {
					zeros = make([]byte, size)
				}
				chunk = zeros[:size]
			} else {
				chunk = append([]byte(nil), unsafe.Slice(data, size)...)
			}
			acc.ReleaseBuffer(frames)
			if _, err := w.Write(chunk); err != nil {
				return err
			}
			if err := acc.GetNextPacketSize(&n); err != nil {
				return err
			}
		}
	}
}
