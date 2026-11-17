package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crazydownloadmanager/internal/model"
)

// TestHLSRecording builds a small HLS stream with ffmpeg, serves it over HTTP
// and records it through the stream engine.
func TestHLSRecording(t *testing.T) {
	ff := FindTool("", "ffmpeg")
	if ff == "" {
		t.Skip("ffmpeg not installed")
	}
	src := t.TempDir()
	gen := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=4:size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=duration=4", "-c:v", "libx264", "-c:a", "aac", "-shortest",
		"-f", "hls", "-hls_time", "1", "-hls_playlist_type", "vod", filepath.Join(src, "index.m3u8"))
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate HLS (ffmpeg build lacks libx264?): %v %s", err, out)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(src)))
	defer srv.Close()

	m, dir := newTestManager(t)
	d, err := m.Add(model.AddRequest{URL: srv.URL + "/index.m3u8", StartNow: true})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != model.KindStream {
		t.Fatalf("kind %s", d.Kind)
	}
	d = waitStatus(t, m, d.ID, model.StatusCompleted, 60*time.Second)
	if d.FileName != "index.mp4" {
		t.Fatalf("file name %q", d.FileName)
	}
	st, err := os.Stat(filepath.Join(dir, d.FileName))
	if err != nil || st.Size() < 10_000 {
		t.Fatalf("output missing or tiny: %v", err)
	}
}

func TestWriteCookieFile(t *testing.T) {
	f, err := writeCookieFile("https://www.youtube.com/watch?v=x", "SID=abc; __Host-T=1; bad; PREF=f6=40000000")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f)
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] != "# Netscape HTTP Cookie File" || len(lines) != 4 {
		t.Fatalf("unexpected file:\n%s", b)
	}
	for i, want := range []string{
		".youtube.com\tTRUE\t/\tTRUE\t",
		"www.youtube.com\tFALSE\t/\tTRUE\t",
		".youtube.com\tTRUE\t/\tTRUE\t",
	} {
		if !strings.HasPrefix(lines[i+1], want) {
			t.Errorf("line %d = %q, want prefix %q", i+1, lines[i+1], want)
		}
	}
	if !strings.HasSuffix(lines[3], "\tPREF\tf6=40000000") {
		t.Errorf("value with '=' mangled: %q", lines[3])
	}
}
