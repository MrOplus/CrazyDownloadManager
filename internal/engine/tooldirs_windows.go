package engine

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// extraToolDirs lists folders to search for ffmpeg/yt-dlp beyond the
// process PATH: the PATH as currently stored in the registry (so tools
// installed while the app runs are found), then WinGet, Scoop and
// Chocolatey locations. Entries with wildcards are globbed.
func extraToolDirs() []string {
	var dirs []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.CURRENT_USER, `Environment`},
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
		for _, d := range strings.Split(v, ";") {
			if d = strings.TrimSpace(d); d != "" && !strings.ContainsAny(d, "*?[") {
				dirs = append(dirs, d)
			}
		}
	}
	local := os.Getenv("LOCALAPPDATA")
	home, _ := os.UserHomeDir()
	if local != "" {
		winget := filepath.Join(local, "Microsoft", "WinGet")
		dirs = append(dirs,
			filepath.Join(winget, "Links"),
			// Portable packages that WinGet doesn't link, e.g. yt-dlp.FFmpeg
			// (…\Packages\yt-dlp.FFmpeg_…\ffmpeg-N-…\bin\ffmpeg.exe).
			filepath.Join(winget, "Packages", "*", "*", "bin"),
			filepath.Join(winget, "Packages", "*"),
		)
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "scoop", "shims"))
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		dirs = append(dirs, filepath.Join(pd, "chocolatey", "bin"))
	}
	return dirs
}
