//go:build !windows

package engine

import (
	"os"
	"path/filepath"
)

// extraToolDirs lists folders to search for ffmpeg/yt-dlp beyond the
// process PATH. Apps launched from Finder get a minimal PATH, so the
// Homebrew, MacPorts and pip/pipx folders are checked explicitly.
func extraToolDirs() []string {
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "Library", "Python", "*", "bin"),
		)
	}
	return dirs
}
