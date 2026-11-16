package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func goos() string { return runtime.GOOS }

func osArgs() []string {
	if len(os.Args) > 1 {
		return os.Args[1:]
	}
	return nil
}

// openPath opens a file (or smb:// URL) with its default application.
func openPath(p string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
	case "darwin":
		return exec.Command("open", p).Start()
	default:
		return exec.Command("xdg-open", p).Start()
	}
}

// revealPath shows the file selected in Explorer/Finder, falling back to the folder.
func revealPath(file, dir string) error {
	_, statErr := os.Stat(file)
	switch runtime.GOOS {
	case "windows":
		if statErr == nil {
			return exec.Command("explorer", "/select,", file).Start()
		}
		return exec.Command("explorer", dir).Start()
	case "darwin":
		if statErr == nil {
			return exec.Command("open", "-R", file).Start()
		}
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// systemAction performs the scheduler's "when done" action.
func systemAction(action string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS + "/" + action {
	case "windows/shutdown":
		cmd = exec.Command("shutdown", "/s", "/t", "60", "/c", "Crazy Download Manager: queue finished")
	case "windows/sleep":
		cmd = exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
	case "darwin/shutdown":
		cmd = exec.Command("osascript", "-e", `tell application "System Events" to shut down`)
	case "darwin/sleep":
		cmd = exec.Command("pmset", "sleepnow")
	case "linux/shutdown":
		cmd = exec.Command("systemctl", "poweroff")
	case "linux/sleep":
		cmd = exec.Command("systemctl", "suspend")
	default:
		return fmt.Errorf("unsupported action %s on %s", action, runtime.GOOS)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
