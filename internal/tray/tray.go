// Package tray shows the notification-area icon (Windows) or menu bar
// extra (macOS) with a small menu. Wails v2 has no tray API, so each
// platform has its own implementation:
//
//   - Windows: fyne.io/systray (pure Go) on its own locked OS thread with its
//     own message loop, separate from the Wails UI thread.
//   - macOS: a native NSStatusItem created on the main queue, which lives
//     alongside Wails' own NSApplication delegate (Wails on macOS already
//     needs cgo).
//   - Other systems: no tray; Start reports false.
package tray

import _ "embed"

//go:embed icon.ico
var iconICO []byte

//go:embed icon_template.png
var iconTemplatePNG []byte

// Menu holds the callbacks for the tray menu. They run on their own
// goroutine, never on the UI thread, so they may block.
type Menu struct {
	Show      func() // also the left-click action on Windows
	ResumeAll func()
	PauseAll  func()
	Quit      func()
}

func call(f func()) {
	if f != nil {
		go f()
	}
}
