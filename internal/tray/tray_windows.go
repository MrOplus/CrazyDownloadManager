//go:build windows

package tray

import (
	"runtime"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

var (
	started atomic.Bool
	status  atomic.Pointer[systray.MenuItem]
	exited  = make(chan struct{})
)

// Start shows the tray icon and reports whether it is up.
func Start(m Menu) bool {
	if !started.CompareAndSwap(false, true) {
		return true
	}
	ready := make(chan struct{})
	go func() {
		// The tray window gets its messages on the thread that created it,
		// so that thread must stay the same and keep pumping.
		runtime.LockOSThread()
		systray.Run(func() { setup(m); close(ready) }, func() { close(exited) })
	}()
	select {
	case <-ready:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func setup(m Menu) {
	systray.SetIcon(iconICO)
	systray.SetTooltip("Crazy Download Manager")
	systray.SetOnTapped(func() { call(m.Show) })

	st := systray.AddMenuItem("No active downloads", "")
	st.Disable()
	status.Store(st)
	systray.AddSeparator()
	show := systray.AddMenuItem("Show Crazy Download Manager", "")
	systray.AddSeparator()
	resume := systray.AddMenuItem("Resume all", "")
	pause := systray.AddMenuItem("Pause all", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Exit", "")

	go func() {
		for {
			select {
			case <-show.ClickedCh:
				call(m.Show)
			case <-resume.ClickedCh:
				call(m.ResumeAll)
			case <-pause.ClickedCh:
				call(m.PauseAll)
			case <-quit.ClickedCh:
				call(m.Quit)
			}
		}
	}()
}

// SetStatus updates the tooltip and the first (disabled) menu line.
func SetStatus(text string) {
	if !started.Load() {
		return
	}
	systray.SetTooltip("Crazy Download Manager\n" + text)
	if st := status.Load(); st != nil {
		st.SetTitle(text)
	}
}

// Stop removes the icon, so it doesn't linger in the notification area
// until the mouse passes over it. It waits briefly for the tray thread.
func Stop() {
	if !started.Load() {
		return
	}
	systray.Quit()
	select {
	case <-exited:
	case <-time.After(time.Second):
	}
}
