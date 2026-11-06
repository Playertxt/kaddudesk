//go:build !kdandroid

package main

import (
	"fmt"
	"sync"

	"fyne.io/systray"
)

var (
	trayReady bool
	trayMu    sync.Mutex
	mStatus   *systray.MenuItem
	mPause    *systray.MenuItem
)

func runTray() { systray.Run(onTrayReady, func() {}) }

func onTrayReady() {
	systray.SetIcon(trayIcon)
	systray.SetTitle("KadduDesk")
	mStatus = systray.AddMenuItem("KadduDesk", "")
	mStatus.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open KadduDesk", "")
	mPause = systray.AddMenuItemCheckbox("Pause sharing", "Refuse all connections", getConfig().Paused)
	mKick := systray.AddMenuItem("Disconnect everyone", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Stop sharing and quit", "")
	trayMu.Lock()
	trayReady = true
	trayMu.Unlock()
	updateTray()
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openApp()
			case <-mPause.ClickedCh:
				cfgMu.Lock()
				cfg.Paused = !cfg.Paused
				saveConfigLocked()
				p := cfg.Paused
				cfgMu.Unlock()
				if p {
					kickAll()
				}
				updateTray()
			case <-mKick.ClickedCh:
				kickAll()
			case <-mQuit.ClickedCh:
				quit()
			}
		}
	}()
	systray.SetOnTapped(openApp)
}

func updateTray() {
	trayMu.Lock()
	ready := trayReady
	trayMu.Unlock()
	if !ready {
		return
	}
	c := getConfig()
	n := clientCount()
	status := fmt.Sprintf("Sharing  ·  ID %s  ·  PIN %s", prettyID(deviceID()), c.PinPlain)
	if c.Paused {
		status = "Sharing paused"
		mPause.Check()
	} else {
		mPause.Uncheck()
	}
	if n > 0 {
		status = fmt.Sprintf("%d device(s) connected", n)
	}
	mStatus.SetTitle(status)
	systray.SetTooltip("KadduDesk — " + status)
}
