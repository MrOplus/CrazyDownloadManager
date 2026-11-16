package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"crazydownloadmanager/internal/config"
	"crazydownloadmanager/internal/model"
)

func payload(t *testing.T, n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
	st, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s := st.Get()
	s.DefaultDir = filepath.Join(tmp, "dl")
	s.Categories = nil
	s.RetryDelaySec = 0
	s.TimeoutSec = 5
	s.Proxy = "direct"
	if err := st.Set(s); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return m, s.DefaultDir
}

func waitStatus(t *testing.T, m *Manager, id string, want model.Status, timeout time.Duration) model.Download {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d, _ := m.Get(id)
		if d.Status == want {
			return d
		}
		if d.Status == model.StatusError && want != model.StatusError {
			t.Fatalf("download failed: %s", d.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := m.Get(id)
	t.Fatalf("timeout waiting for %s, status=%s err=%s", want, d.Status, d.Error)
	return d
}

func checkFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// slowReader throttles a ReadSeeker so pauses can happen mid-transfer.
type slowReader struct {
	io.ReadSeeker
	delay time.Duration
}

func (s slowReader) Read(p []byte) (int, error) {
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	time.Sleep(s.delay)
	return s.ReadSeeker.Read(p)
}

func TestSegmentedDownload(t *testing.T) {
	data := payload(t, 24<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "big.bin", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()

	m, dir := newTestManager(t)
	d, err := m.Add(model.AddRequest{URL: srv.URL + "/files/big.bin", StartNow: true, Connections: 8})
	if err != nil {
		t.Fatal(err)
	}
	d = waitStatus(t, m, d.ID, model.StatusCompleted, 30*time.Second)
	if d.FileName != "big.bin" || d.Size != int64(len(data)) || !d.Resumable {
		t.Fatalf("unexpected result %+v", d)
	}
	checkFile(t, filepath.Join(dir, "big.bin"), data)
}

func TestDynamicSplitting(t *testing.T) {
	data := payload(t, 16<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x", time.Now(), slowReader{bytes.NewReader(data), time.Millisecond})
	}))
	defer srv.Close()
	m, _ := newTestManager(t)
	d := model.Download{URL: srv.URL, Size: -1}
	src, _ := m.sourceFor(d)
	pr, err := src.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	j := newSegJob(context.Background(), src, f, pr.Size, true, []model.Segment{{Start: 0, End: pr.Size}})
	j.maxConn, j.timeout, j.maxRetries = 8, 5*time.Second, 3
	j.counter = new(atomic.Int64)
	if err := j.run(); err != nil {
		t.Fatal(err)
	}
	p := j.progress()
	if len(p.Segments) < 8 {
		t.Fatalf("expected at least 8 segments, got %d", len(p.Segments))
	}
	var covered int64
	for _, s := range p.Segments {
		if s.Pos != s.End {
			t.Fatalf("segment not finished: %+v", s)
		}
		covered += s.End - s.Start
	}
	if covered != int64(len(data)) {
		t.Fatalf("segments cover %d bytes, want %d", covered, len(data))
	}
	f.Close()
	checkFile(t, f.Name(), data)
}

func TestPauseResume(t *testing.T) {
	data := payload(t, 8<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "p.bin", time.Now(), slowReader{bytes.NewReader(data), 40 * time.Millisecond})
	}))
	defer srv.Close()

	m, dir := newTestManager(t)
	d, _ := m.Add(model.AddRequest{URL: srv.URL + "/p.bin", StartNow: true, Connections: 4})
	deadline := time.Now().Add(10 * time.Second)
	for {
		cur, _ := m.Get(d.ID)
		if cur.Downloaded > 1<<20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no progress")
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.Pause(d.ID)
	paused := waitStatus(t, m, d.ID, model.StatusPaused, 10*time.Second)
	if paused.Downloaded == 0 || paused.Downloaded >= int64(len(data)) || len(paused.Segments) == 0 {
		t.Fatalf("bad paused state: downloaded=%d segments=%d", paused.Downloaded, len(paused.Segments))
	}
	if _, err := os.Stat(filepath.Join(dir, "p.bin"+PartSuffix)); err != nil {
		t.Fatalf("part file missing: %v", err)
	}
	m.Resume(d.ID)
	waitStatus(t, m, d.ID, model.StatusCompleted, 60*time.Second)
	checkFile(t, filepath.Join(dir, "p.bin"), data)
}

func TestNoRangeServer(t *testing.T) {
	data := payload(t, 3<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Content-Disposition", `attachment; filename="report final.pdf"`)
		_, _ = w.Write(data) // ignores Range
	}))
	defer srv.Close()
	m, dir := newTestManager(t)
	d, _ := m.Add(model.AddRequest{URL: srv.URL + "/dl?id=1", StartNow: true})
	d = waitStatus(t, m, d.ID, model.StatusCompleted, 20*time.Second)
	if d.Resumable || d.FileName != "report final.pdf" {
		t.Fatalf("unexpected %+v", d)
	}
	checkFile(t, filepath.Join(dir, "report final.pdf"), data)
}

func TestUnknownSize(t *testing.T) {
	data := payload(t, 1<<20+123)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := 0; i < len(data); i += 100 << 10 {
			end := min(i+100<<10, len(data))
			_, _ = w.Write(data[i:end])
			fl.Flush() // chunked, no Content-Length
		}
	}))
	defer srv.Close()
	m, dir := newTestManager(t)
	d, _ := m.Add(model.AddRequest{URL: srv.URL + "/stream.dat", StartNow: true})
	d = waitStatus(t, m, d.ID, model.StatusCompleted, 20*time.Second)
	if d.Size != int64(len(data)) {
		t.Fatalf("size %d, want %d", d.Size, len(data))
	}
	checkFile(t, filepath.Join(dir, "stream.dat"), data)
}

func TestUniqueNames(t *testing.T) {
	data := payload(t, 100<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "a.txt", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()
	m, dir := newTestManager(t)
	a, _ := m.Add(model.AddRequest{URL: srv.URL + "/a.txt", StartNow: true})
	waitStatus(t, m, a.ID, model.StatusCompleted, 10*time.Second)
	b, _ := m.Add(model.AddRequest{URL: srv.URL + "/a.txt", StartNow: true})
	b = waitStatus(t, m, b.ID, model.StatusCompleted, 10*time.Second)
	if b.FileName != "a (1).txt" {
		t.Fatalf("second copy named %q", b.FileName)
	}
	checkFile(t, filepath.Join(dir, "a (1).txt"), data)
}

func TestQueue(t *testing.T) {
	data := payload(t, 200<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "q", time.Now(), bytes.NewReader(data))
	}))
	defer srv.Close()
	m, _ := newTestManager(t)
	var ids []string
	for i := 0; i < 5; i++ {
		d, _ := m.Add(model.AddRequest{URL: srv.URL + "/q" + strconv.Itoa(i) + ".bin"})
		ids = append(ids, d.ID)
	}
	if d, _ := m.Get(ids[0]); d.Status != model.StatusQueued {
		t.Fatalf("expected queued, got %s", d.Status)
	}
	m.StartQueue("main")
	for _, id := range ids {
		waitStatus(t, m, id, model.StatusCompleted, 20*time.Second)
	}
	time.Sleep(time.Second)
	if m.QueueStates()["main"] {
		t.Fatal("queue should stop when empty")
	}
}

func TestDetectKind(t *testing.T) {
	cases := map[string]model.Kind{
		"https://a.com/x/master.m3u8?t=1":   model.KindStream,
		"mms://media.example.com/live":      model.KindStream,
		"rtmp://x/live/key":                 model.KindStream,
		"https://www.youtube.com/watch?v=1": model.KindMedia,
		"https://a.com/file.zip":            model.KindFile,
		"ftp://a.com/pub/file.iso":          model.KindFile,
	}
	for u, want := range cases {
		if got := DetectKind(u, ""); got != want {
			t.Errorf("%s: got %s want %s", u, got, want)
		}
	}
	if got := ffmpegInputs("mms://h/p"); len(got) != 2 || got[0] != "mmsh://h/p" || got[1] != "mmst://h/p" {
		t.Errorf("mms expansion: %v", got)
	}
}

func TestSanitize(t *testing.T) {
	if got := SanitizeFileName(`a<b>:c"d/e\f|g?h*.zip`); got != "a_b__c_d_e_f_g_h_.zip" {
		t.Errorf("got %q", got)
	}
	if got := SanitizeFileName("CON.txt"); got != "_CON.txt" {
		t.Errorf("got %q", got)
	}
}
