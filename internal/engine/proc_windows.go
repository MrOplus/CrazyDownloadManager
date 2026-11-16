//go:build windows

package engine

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideWindow stops ffmpeg / yt-dlp from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

const fsctlSetSparse = 0x000900c4

// makeSparse marks a file sparse so NTFS does not zero-fill the gap when a
// segment writes far beyond the current end of file.
func makeSparse(f any) {
	file, ok := f.(*os.File)
	if !ok {
		return
	}
	var ret uint32
	_ = windows.DeviceIoControl(windows.Handle(file.Fd()), fsctlSetSparse, nil, 0, nil, 0, &ret, nil)
}
