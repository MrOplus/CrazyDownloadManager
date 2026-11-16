//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void cdmTrayStart(const void *png, int len);
void cdmTraySetStatus(const char *s);
void cdmTrayStop(void);
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	started atomic.Bool
	menuMu  sync.Mutex
	menu    Menu
)

//export cdmTrayClicked
func cdmTrayClicked(tag C.int) {
	menuMu.Lock()
	m := menu
	menuMu.Unlock()
	switch tag {
	case 1:
		call(m.Show)
	case 2:
		call(m.ResumeAll)
	case 3:
		call(m.PauseAll)
	case 4:
		call(m.Quit)
	}
}

// Start adds the menu bar extra. Call it after the Cocoa app is running
// (from Wails' OnStartup); the item is created on the main queue.
func Start(m Menu) bool {
	menuMu.Lock()
	menu = m
	menuMu.Unlock()
	if !started.CompareAndSwap(false, true) {
		return true
	}
	C.cdmTrayStart(unsafe.Pointer(&iconTemplatePNG[0]), C.int(len(iconTemplatePNG)))
	return true
}

// SetStatus updates the tooltip and the first (disabled) menu line.
func SetStatus(text string) {
	if !started.Load() {
		return
	}
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.cdmTraySetStatus(cs)
}

func Stop() {
	if started.Load() {
		C.cdmTrayStop()
	}
}
