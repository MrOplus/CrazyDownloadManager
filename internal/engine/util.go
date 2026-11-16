package engine

import (
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"crazydownloadmanager/internal/model"
	"crazydownloadmanager/internal/storage"
)

// PartSuffix marks an unfinished download on disk.
const PartSuffix = ".cdmpart"

// FatalError stops retries (404, 403, disk full, …).
type FatalError struct{ Err error }

func (e *FatalError) Error() string { return e.Err.Error() }
func (e *FatalError) Unwrap() error { return e.Err }

func fatal(err error) error { return &FatalError{Err: err} }

func isFatal(err error) bool {
	var f *FatalError
	return errors.As(err, &f)
}

// errNoRange means the server ignored a Range request: restart from zero.
var errNoRange = errors.New("server does not support resuming")

var errStalled = errors.New("connection stalled (timeout)")

var invalidChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// SanitizeFileName makes name safe on Windows, macOS and SMB shares.
func SanitizeFileName(name string) string {
	name = invalidChars.ReplaceAllString(name, "_")
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		return "download"
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "LPT1", "LPT2", "LPT3":
		name = "_" + name
	}
	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = name[:200-len(ext)] + ext
	}
	return name
}

// FileNameFromURL derives a file name from the last path element of u.
func FileNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "download"
	}
	base := path.Base(u.Path)
	if base == "/" || base == "." || base == "" {
		base = u.Hostname()
		if base == "" {
			base = "download"
		}
		base += ".html"
	}
	if s, err := url.PathUnescape(base); err == nil {
		base = s
	}
	return SanitizeFileName(base)
}

// FileNameFromDisposition parses a Content-Disposition header.
func FileNameFromDisposition(h string) string {
	if h == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(h)
	if err != nil {
		// Some servers send unquoted names with spaces; be lenient.
		if i := strings.Index(strings.ToLower(h), "filename="); i >= 0 {
			return SanitizeFileName(strings.Trim(h[i+9:], `"; `))
		}
		return ""
	}
	if n := params["filename"]; n != "" {
		return SanitizeFileName(n)
	}
	return ""
}

// UniqueName returns name, or "name (n).ext", that exists neither as a
// finished file nor as a part file in fs.
func UniqueName(fs storage.FS, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; i < 10000; i++ {
		if !fs.Exists(candidate) && !fs.Exists(candidate+PartSuffix) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return candidate
}

var mediaHosts = []string{
	"youtube.com", "youtu.be", "vimeo.com", "dailymotion.com", "twitch.tv",
	"tiktok.com", "twitter.com", "x.com", "facebook.com", "instagram.com",
	"soundcloud.com", "bilibili.com", "reddit.com", "aparat.com",
}

// DetectKind decides which engine handles a URL.
func DetectKind(raw, contentType string) model.Kind {
	u, err := url.Parse(raw)
	if err != nil {
		return model.KindFile
	}
	switch strings.ToLower(u.Scheme) {
	case "mms", "mmsh", "mmst", "rtmp", "rtmps", "rtmpe", "rtsp", "rtsps", "rtp", "srt", "udp":
		return model.KindStream
	}
	p := strings.ToLower(u.Path)
	if strings.HasSuffix(p, ".m3u8") || strings.HasSuffix(p, ".mpd") || strings.HasSuffix(p, ".ism/manifest") {
		return model.KindStream
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "dash+xml") {
		return model.KindStream
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	for _, h := range mediaHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return model.KindMedia
		}
	}
	return model.KindFile
}

// FindTool resolves a configured binary path or looks it up on PATH and next
// to the executable.
func FindTool(configured, name string) string {
	if configured != "" {
		if p, err := exec.LookPath(configured); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if exe, err := executableDir(); err == nil {
		for _, c := range []string{filepath.Join(exe, name), filepath.Join(exe, name+".exe"), filepath.Join(exe, "bin", name), filepath.Join(exe, "bin", name+".exe")} {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}
	for _, c := range []string{"/opt/homebrew/bin/" + name, "/usr/local/bin/" + name} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}
