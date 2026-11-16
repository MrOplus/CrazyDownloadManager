package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"crazydownloadmanager/internal/config"
	"crazydownloadmanager/internal/model"
	"crazydownloadmanager/internal/storage"
)

// Events emitted to the GUI.
const (
	EventDownloads = "downloads"         // []model.Download, ~2x per second while anything changes
	EventComplete  = "download:complete" // model.Download
	EventFailed    = "download:failed"   // model.Download
	EventQueueDone = "queue:done"        // QueueDone
)

type QueueDone struct {
	Queue     config.Queue `json:"queue"`
	Scheduled bool         `json:"scheduled"`
}

type runJob struct {
	cancel  context.CancelFunc
	done    chan struct{}
	counter atomic.Int64
	prog    atomic.Pointer[func() Progress]
	limiter *rate.Limiter
	speed   float64
}

type Manager struct {
	settings *config.Store
	emit     func(name string, data any)
	dataPath string

	mu           sync.Mutex
	downloads    map[string]*model.Download
	jobs         map[string]*runJob
	queueOn      map[string]bool
	queueBySched map[string]bool
	schedFired   map[string]string
	dirty        bool
	changed      bool

	global *rate.Limiter
	client atomic.Pointer[clientHolder]
}

type clientHolder struct {
	key    string
	client *http.Client
}

func NewManager(settings *config.Store, emit func(string, any)) (*Manager, error) {
	m := &Manager{
		settings:     settings,
		emit:         emit,
		dataPath:     filepath.Join(config.Dir(), "downloads.json"),
		downloads:    map[string]*model.Download{},
		jobs:         map[string]*runJob{},
		queueOn:      map[string]bool{},
		queueBySched: map[string]bool{},
		schedFired:   map[string]string{},
		global:       rate.NewLimiter(rate.Inf, bufSize),
	}
	m.ApplySettings()
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// ApplySettings pushes runtime-relevant settings (speed limit) into the engine.
func (m *Manager) ApplySettings() {
	s := m.settings.Get()
	setLimit(m.global, s.GlobalSpeedLimit)
}

func setLimit(l *rate.Limiter, kbps int64) {
	if kbps <= 0 {
		l.SetLimit(rate.Inf)
		return
	}
	bps := kbps * 1024
	l.SetLimit(rate.Limit(bps))
	burst := int(bps)
	if burst < bufSize {
		burst = bufSize
	}
	l.SetBurst(burst)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------- persistence

func (m *Manager) load() error {
	data, err := os.ReadFile(m.dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []*model.Download
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("read %s: %w", m.dataPath, err)
	}
	for _, d := range list {
		if d.Status.Active() {
			d.Status = model.StatusPaused
		}
		d.Speed, d.ActiveConns, d.ETA = 0, 0, -1
		m.downloads[d.ID] = d
	}
	return nil
}

func (m *Manager) save() error {
	m.mu.Lock()
	list := m.listLocked()
	m.dirty = false
	m.mu.Unlock()
	data, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.dataPath), 0o755); err != nil {
		return err
	}
	tmp := m.dataPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.dataPath)
}

func (m *Manager) listLocked() []model.Download {
	list := make([]model.Download, 0, len(m.downloads))
	for _, d := range m.downloads {
		list = append(list, *d)
	}
	sort.Slice(list, func(i, k int) bool { return list[i].Added.Before(list[k].Added) })
	return list
}

func (m *Manager) List() []model.Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) Get(id string) (model.Download, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return model.Download{}, false
	}
	return *d, true
}

func (m *Manager) touch() { m.dirty, m.changed = true, true }

// ---------------------------------------------------------------- loop

// Run drives progress reporting, queues, the scheduler and autosave until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	last := time.Now()
	lastSave := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			dt := now.Sub(last).Seconds()
			last = now
			m.refresh(dt)
			m.pumpQueues()
			m.runScheduler(now)
			if now.Sub(lastSave) > 3*time.Second {
				m.mu.Lock()
				dirty := m.dirty
				m.mu.Unlock()
				if dirty {
					_ = m.save()
				}
				lastSave = now
			}
		}
	}
}

func (m *Manager) refresh(dt float64) {
	m.mu.Lock()
	for id, rj := range m.jobs {
		d := m.downloads[id]
		if d == nil {
			continue
		}
		if pf := rj.prog.Load(); pf != nil {
			p := (*pf)()
			d.Size, d.Downloaded, d.ActiveConns = p.Size, p.Downloaded, p.Active
			if p.Segments != nil {
				d.Segments = p.Segments
			}
		}
		inst := float64(rj.counter.Swap(0)) / dt
		if rj.speed == 0 {
			rj.speed = inst
		} else {
			rj.speed = 0.7*rj.speed + 0.3*inst
		}
		d.Speed = int64(rj.speed)
		d.ETA = -1
		if d.Size > 0 && d.Speed > 0 {
			d.ETA = (d.Size - d.Downloaded) / d.Speed
		}
		m.touch()
	}
	changed := m.changed
	m.changed = false
	var list []model.Download
	if changed {
		list = m.listLocked()
	}
	m.mu.Unlock()
	if changed && m.emit != nil {
		m.emit(EventDownloads, list)
	}
}

// ---------------------------------------------------------------- probing / adding

func (m *Manager) httpClient() *http.Client {
	s := m.settings.Get()
	key := fmt.Sprintf("%s|%d", s.Proxy, s.TimeoutSec)
	if h := m.client.Load(); h != nil && h.key == key {
		return h.client
	}
	c := NewHTTPClient(s)
	m.client.Store(&clientHolder{key: key, client: c})
	return c
}

func (m *Manager) sourceFor(d model.Download) (Source, error) {
	s := m.settings.Get()
	u, err := url.Parse(d.URL)
	if err != nil {
		return nil, fatal(err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return newHTTPSource(m.httpClient(), d, s.UserAgent), nil
	case "ftp", "ftps":
		return newFTPSource(d, time.Duration(s.TimeoutSec)*time.Second)
	}
	return nil, fatal(fmt.Errorf("unsupported protocol %q", u.Scheme))
}

// Probe inspects a URL for the "Download File Info" dialog.
func (m *Manager) Probe(ctx context.Context, req model.AddRequest) (model.ProbeResult, error) {
	s := m.settings.Get()
	d := requestToDownload(req)
	kind := req.Kind
	if kind == "" {
		kind = DetectKind(req.URL, "")
	}
	var r model.ProbeResult
	switch kind {
	case model.KindStream:
		name := req.FileName
		if name == "" {
			name = strings.TrimSuffix(FileNameFromURL(req.URL), filepath.Ext(FileNameFromURL(req.URL))) + streamExt(req.URL)
		}
		r = model.ProbeResult{URL: req.URL, FinalURL: req.URL, FileName: name, Size: -1, Kind: kind}
	case model.KindMedia:
		r = model.ProbeResult{URL: req.URL, FinalURL: req.URL, FileName: req.FileName, Size: -1, Kind: kind}
		if FindTool(s.YtDlpPath, "yt-dlp") == "" {
			// No yt-dlp: treat it as an ordinary file.
			kind = model.KindFile
		} else {
			break
		}
		fallthrough
	default:
		src, err := m.sourceFor(d)
		if err != nil {
			return model.ProbeResult{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, time.Duration(s.TimeoutSec)*time.Second)
		defer cancel()
		r, err = src.Probe(ctx)
		if err != nil {
			return model.ProbeResult{}, err
		}
		if req.FileName != "" {
			r.FileName = SanitizeFileName(req.FileName)
		}
		if r.Kind == "" || (r.Kind == model.KindMedia && FindTool(s.YtDlpPath, "yt-dlp") == "") {
			r.Kind = model.KindFile
		}
	}
	r.Dir, r.Category = s.DirFor(r.FileName)
	return r, nil
}

func requestToDownload(req model.AddRequest) model.Download {
	return model.Download{
		URL: strings.TrimSpace(req.URL), FileName: req.FileName, Dir: req.Dir,
		Category: req.Category, QueueID: req.QueueID, Kind: req.Kind,
		Referer: req.Referer, UserAgent: req.UserAgent, Cookies: req.Cookies,
		Headers: req.Headers, Username: req.Username, Password: req.Password,
		Description: req.Description, Connections: req.Connections,
		SpeedLimit: req.SpeedLimit, PostProcess: req.PostProcess, Size: -1,
	}
}

// Add creates a download. It starts immediately if req.StartNow, otherwise it
// waits in its queue.
func (m *Manager) Add(req model.AddRequest) (model.Download, error) {
	if _, err := url.ParseRequestURI(strings.TrimSpace(req.URL)); err != nil {
		return model.Download{}, fmt.Errorf("invalid URL: %w", err)
	}
	s := m.settings.Get()
	d := requestToDownload(req)
	d.ID = newID()
	d.Added = time.Now()
	d.ETA = -1
	if req.Size > 0 {
		d.Size = req.Size
	}
	if d.Kind == "" {
		d.Kind = DetectKind(d.URL, "")
		if d.Kind == model.KindMedia && FindTool(s.YtDlpPath, "yt-dlp") == "" {
			d.Kind = model.KindFile
		}
	}
	if d.FileName != "" {
		d.FileName = SanitizeFileName(d.FileName)
	}
	guess := d.FileName
	if guess == "" {
		guess = FileNameFromURL(d.URL)
	}
	if d.Dir == "" || d.Category == "" {
		dir, cat := s.DirFor(guess)
		if d.Dir == "" {
			d.Dir = dir
		}
		if d.Category == "" {
			d.Category = cat
		}
	}
	if d.QueueID == "" {
		d.QueueID = "main"
	}
	if d.Connections <= 0 {
		d.Connections = s.MaxConnections
	}
	d.Status = model.StatusQueued

	m.mu.Lock()
	m.downloads[d.ID] = &d
	m.touch()
	m.mu.Unlock()
	if req.StartNow {
		m.Resume(d.ID)
	}
	return m.mustGet(d.ID), nil
}

func (m *Manager) mustGet(id string) model.Download {
	d, _ := m.Get(id)
	return d
}

// ---------------------------------------------------------------- control

// Resume starts (or restarts) a download right away, outside queue limits.
func (m *Manager) Resume(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startLocked(id)
}

func (m *Manager) startLocked(id string) {
	d, ok := m.downloads[id]
	if !ok || m.jobs[id] != nil || d.Status == model.StatusCompleted {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	rj := &runJob{cancel: cancel, done: make(chan struct{}), limiter: rate.NewLimiter(rate.Inf, bufSize)}
	setLimit(rj.limiter, d.SpeedLimit)
	m.jobs[id] = rj
	d.Status = model.StatusConnecting
	d.Error = ""
	d.LastTry = time.Now()
	m.touch()
	go m.run(ctx, id, rj)
}

// Pause stops a running download (or holds a queued one).
func (m *Manager) Pause(id string) {
	m.mu.Lock()
	rj := m.jobs[id]
	if d := m.downloads[id]; d != nil && rj == nil && d.Status == model.StatusQueued {
		d.Status = model.StatusPaused
		m.touch()
	}
	m.mu.Unlock()
	if rj != nil {
		rj.cancel()
	}
}

func (m *Manager) PauseAll() {
	m.mu.Lock()
	jobs := make([]*runJob, 0, len(m.jobs))
	for _, rj := range m.jobs {
		jobs = append(jobs, rj)
	}
	for id := range m.queueOn {
		m.queueOn[id] = false
	}
	m.mu.Unlock()
	for _, rj := range jobs {
		rj.cancel()
	}
}

// ResumeAll restarts every paused or failed download.
func (m *Manager) ResumeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.downloads {
		if d.Status == model.StatusPaused || d.Status == model.StatusError {
			m.startLocked(id)
		}
	}
}

// Shutdown stops all transfers and saves state; it waits up to timeout.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	var dones []chan struct{}
	for _, rj := range m.jobs {
		rj.cancel()
		dones = append(dones, rj.done)
	}
	m.mu.Unlock()
	deadline := time.After(timeout)
	for _, d := range dones {
		select {
		case <-d:
		case <-deadline:
		}
	}
	_ = m.save()
}

func (m *Manager) stopAndWait(id string) {
	m.mu.Lock()
	rj := m.jobs[id]
	m.mu.Unlock()
	if rj != nil {
		rj.cancel()
		select {
		case <-rj.done:
		case <-time.After(15 * time.Second):
		}
	}
}

// Remove deletes downloads from the list, optionally with their files.
func (m *Manager) Remove(ids []string, deleteFiles bool) {
	for _, id := range ids {
		m.stopAndWait(id)
		m.mu.Lock()
		d := m.downloads[id]
		delete(m.downloads, id)
		m.touch()
		m.mu.Unlock()
		if d != nil {
			m.removeFiles(*d, deleteFiles)
		}
	}
}

func (m *Manager) removeFiles(d model.Download, includeFinished bool) {
	if d.FileName == "" {
		return
	}
	fs, err := storage.Open(d.Dir, m.settings.Get())
	if err != nil {
		return
	}
	defer fs.Close()
	_ = fs.Remove(d.FileName + PartSuffix)
	if includeFinished && d.Status == model.StatusCompleted {
		_ = fs.Remove(d.FileName)
	}
}

// RemoveCompleted clears finished entries (files are kept).
func (m *Manager) RemoveCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, d := range m.downloads {
		if d.Status == model.StatusCompleted {
			delete(m.downloads, id)
		}
	}
	m.touch()
}

// Redownload discards progress and starts over.
func (m *Manager) Redownload(id string) {
	m.stopAndWait(id)
	m.mu.Lock()
	d := m.downloads[id]
	if d == nil {
		m.mu.Unlock()
		return
	}
	old := *d
	d.Segments, d.Downloaded, d.Status, d.Error = nil, 0, model.StatusPaused, ""
	if old.Status == model.StatusCompleted {
		d.FileName = "" // keep the finished copy, pick a new unique name
	}
	m.mu.Unlock()
	if old.Status != model.StatusCompleted {
		m.removeFiles(old, false)
	}
	m.Resume(id)
}

// Update edits user-editable fields (URL refresh, name, dir, credentials…).
func (m *Manager) Update(id string, req model.AddRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.downloads[id]
	if d == nil {
		return errors.New("download not found")
	}
	if m.jobs[id] != nil {
		return errors.New("stop the download before editing it")
	}
	if req.URL != "" {
		d.URL = strings.TrimSpace(req.URL)
	}
	if req.FileName != "" && d.Downloaded == 0 {
		d.FileName = SanitizeFileName(req.FileName)
	}
	if req.Dir != "" && d.Downloaded == 0 {
		d.Dir = req.Dir
	}
	d.Referer, d.Cookies, d.Description = req.Referer, req.Cookies, req.Description
	d.Username, d.Password = req.Username, req.Password
	if req.Connections > 0 {
		d.Connections = req.Connections
	}
	d.SpeedLimit = req.SpeedLimit
	d.PostProcess = req.PostProcess
	if req.QueueID != "" {
		d.QueueID = req.QueueID
	}
	m.touch()
	return nil
}

// SetSpeedLimit changes a download's own limit (KB/s), live.
func (m *Manager) SetSpeedLimit(id string, kbps int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d := m.downloads[id]; d != nil {
		d.SpeedLimit = kbps
		m.touch()
	}
	if rj := m.jobs[id]; rj != nil {
		setLimit(rj.limiter, kbps)
	}
}

// ---------------------------------------------------------------- queues

func (m *Manager) StartQueue(id string) { m.setQueue(id, true, false) }
func (m *Manager) StopQueue(id string)  { m.setQueue(id, false, false) }

func (m *Manager) setQueue(id string, on, scheduled bool) {
	m.mu.Lock()
	m.queueOn[id] = on
	m.queueBySched[id] = on && scheduled
	var stop []*runJob
	if !on {
		for jid, rj := range m.jobs {
			if d := m.downloads[jid]; d != nil && d.QueueID == id {
				stop = append(stop, rj)
			}
		}
	}
	m.touch()
	m.mu.Unlock()
	for _, rj := range stop {
		rj.cancel()
	}
	// Stopped queue items go back to waiting so the next start picks them up.
	if !on {
		go func() {
			time.Sleep(time.Second)
			m.mu.Lock()
			for _, d := range m.downloads {
				if d.QueueID == id && d.Status == model.StatusPaused && m.jobs[d.ID] == nil {
					d.Status = model.StatusQueued
				}
			}
			m.touch()
			m.mu.Unlock()
		}()
	}
}

func (m *Manager) QueueStates() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for k, v := range m.queueOn {
		out[k] = v
	}
	return out
}

// MoveToQueue assigns downloads to a queue and marks them waiting.
func (m *Manager) MoveToQueue(ids []string, queueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if d := m.downloads[id]; d != nil {
			d.QueueID = queueID
			if d.Status == model.StatusPaused || d.Status == model.StatusError {
				d.Status = model.StatusQueued
			}
		}
	}
	m.touch()
}

func (m *Manager) pumpQueues() {
	s := m.settings.Get()
	var finished []QueueDone
	m.mu.Lock()
	for _, q := range s.Queues {
		if !m.queueOn[q.ID] {
			continue
		}
		active := 0
		var waiting []*model.Download
		for id, d := range m.downloads {
			if d.QueueID != q.ID {
				continue
			}
			if m.jobs[id] != nil {
				active++
			} else if d.Status == model.StatusQueued {
				waiting = append(waiting, d)
			}
		}
		sort.Slice(waiting, func(i, k int) bool { return waiting[i].Added.Before(waiting[k].Added) })
		for _, d := range waiting {
			if active >= q.MaxConcurrent {
				break
			}
			m.startLocked(d.ID)
			active++
		}
		if active == 0 && len(waiting) == 0 {
			m.queueOn[q.ID] = false
			finished = append(finished, QueueDone{Queue: q, Scheduled: m.queueBySched[q.ID]})
			m.queueBySched[q.ID] = false
			m.touch()
		}
	}
	m.mu.Unlock()
	for _, f := range finished {
		if m.emit != nil {
			m.emit(EventQueueDone, f)
		}
	}
}

func (m *Manager) runScheduler(now time.Time) {
	s := m.settings.Get()
	hm := now.Format("15:04")
	day := int(now.Weekday())
	stamp := now.Format("2006-01-02 15:04")
	for _, q := range s.Queues {
		sc := q.Schedule
		if !sc.Enabled {
			continue
		}
		if len(sc.Days) > 0 {
			ok := false
			for _, d := range sc.Days {
				if d == day {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		m.mu.Lock()
		fireStart := sc.StartTime == hm && m.schedFired[q.ID+"/start"] != stamp
		fireStop := sc.StopTime == hm && m.schedFired[q.ID+"/stop"] != stamp
		if fireStart {
			m.schedFired[q.ID+"/start"] = stamp
		}
		if fireStop {
			m.schedFired[q.ID+"/stop"] = stamp
		}
		m.mu.Unlock()
		if fireStart {
			m.setQueue(q.ID, true, true)
		}
		if fireStop {
			m.setQueue(q.ID, false, false)
		}
	}
}

// ---------------------------------------------------------------- running

func (m *Manager) update(id string, fn func(d *model.Download)) {
	m.mu.Lock()
	if d := m.downloads[id]; d != nil {
		fn(d)
		m.touch()
	}
	m.mu.Unlock()
}

func (m *Manager) run(ctx context.Context, id string, rj *runJob) {
	defer close(rj.done)
	d, ok := m.Get(id)
	if !ok {
		return
	}
	var err error
	switch d.Kind {
	case model.KindStream:
		err = m.runStream(ctx, id, rj, d)
	case model.KindMedia:
		err = m.runMedia(ctx, id, rj, d)
	default:
		err = m.runFile(ctx, id, rj, d)
	}

	m.mu.Lock()
	cur := m.downloads[id]
	delete(m.jobs, id)
	if cur == nil { // removed while running
		m.mu.Unlock()
		return
	}
	if pf := rj.prog.Load(); pf != nil {
		p := (*pf)()
		cur.Size, cur.Downloaded = p.Size, p.Downloaded
		if p.Segments != nil {
			cur.Segments = p.Segments
		}
	}
	cur.Speed, cur.ActiveConns, cur.ETA = 0, 0, -1
	event := ""
	switch {
	case err == nil:
		cur.Status = model.StatusCompleted
		cur.Finished = time.Now()
		cur.Segments = nil
		if cur.Size < 0 || cur.Size < cur.Downloaded {
			cur.Size = cur.Downloaded
		}
		cur.Downloaded = cur.Size
		event = EventComplete
	case ctx.Err() != nil:
		cur.Status = model.StatusPaused
	default:
		cur.Status = model.StatusError
		cur.Error = err.Error()
		event = EventFailed
	}
	snapshot := *cur
	m.touch()
	m.mu.Unlock()
	_ = m.save()
	if event != "" && m.emit != nil {
		m.emit(event, snapshot)
	}
	if event == EventComplete {
		m.antivirus(snapshot)
	}
}

func (m *Manager) runFile(ctx context.Context, id string, rj *runJob, d model.Download) error {
	s := m.settings.Get()
	src, err := m.sourceFor(d)
	if err != nil {
		return err
	}
	var pr model.ProbeResult
	for attempt := 0; ; attempt++ {
		pr, err = src.Probe(ctx)
		if err == nil || isFatal(err) || ctx.Err() != nil || attempt >= s.MaxRetries {
			break
		}
		select {
		case <-time.After(time.Duration(s.RetryDelaySec) * time.Second):
		case <-ctx.Done():
		}
	}
	if err != nil {
		return err
	}
	// A browser link can turn out to be an HLS/DASH manifest.
	if pr.Kind == model.KindStream && d.Kind == model.KindFile && d.Downloaded == 0 {
		m.update(id, func(x *model.Download) { x.Kind = model.KindStream })
		d.Kind = model.KindStream
		return m.runStream(ctx, id, rj, d)
	}

	fs, err := storage.Open(d.Dir, s)
	if err != nil {
		return fatal(err)
	}
	defer fs.Close()

	fresh := len(d.Segments) == 0 && d.Downloaded == 0
	name := d.FileName
	if name == "" {
		name = pr.FileName
	}
	if fresh || d.FileName == "" {
		name = UniqueName(fs, name)
	}
	reset := fresh
	if !fresh {
		if !pr.Resumable || (d.Size > 0 && pr.Size > 0 && d.Size != pr.Size) || !fs.Exists(name+PartSuffix) {
			reset = true // server changed the file, or the part file is gone
		}
	}

	file, err := fs.Open(name + PartSuffix)
	if err != nil {
		return fatal(err)
	}
	segs := d.Segments
	if reset {
		if err := file.Truncate(0); err != nil {
			file.Close()
			return fatal(err)
		}
		end := pr.Size
		if end <= 0 {
			end = -1
		}
		segs = []model.Segment{{Start: 0, End: end, Pos: 0}}
	}
	makeSparse(file)

	m.update(id, func(x *model.Download) {
		x.FileName = name
		x.Size = pr.Size
		x.Resumable = pr.Resumable
		x.Status = model.StatusDownloading
		if x.Category == "" || x.Category == "General" {
			_, x.Category = s.DirFor(name)
		}
	})

	job := newSegJob(ctx, src, file, pr.Size, pr.Resumable, segs)
	job.maxConn = d.Connections
	job.timeout = time.Duration(s.TimeoutSec) * time.Second
	job.maxRetries = s.MaxRetries
	job.retryDelay = time.Duration(s.RetryDelaySec) * time.Second
	job.limiter = rj.limiter
	job.global = m.global
	job.counter = &rj.counter
	pf := job.progress
	rj.prog.Store(&pf)

	err = job.run()
	cerr := file.Close()
	if err != nil {
		return err
	}
	if cerr != nil {
		return fatal(cerr)
	}

	m.update(id, func(x *model.Download) { x.Status = model.StatusProcessing })
	final := name
	if fs.Exists(final) {
		final = UniqueName(fs, final)
	}
	if err := fs.Rename(name+PartSuffix, final); err != nil {
		return fatal(fmt.Errorf("rename: %w", err))
	}
	if final != name {
		m.update(id, func(x *model.Download) { x.FileName = final })
	}
	return m.postProcess(ctx, id, d, fs, final)
}

func (m *Manager) postProcess(ctx context.Context, id string, d model.Download, fs storage.FS, name string) error {
	if d.PostProcess == "" {
		return nil
	}
	ff := FindTool(m.settings.Get().FFmpegPath, "ffmpeg")
	local := fs.Local(name)
	if ff == "" || local == "" {
		return nil // conversion is best-effort; the download itself succeeded
	}
	m.update(id, func(x *model.Download) { x.Status = model.StatusProcessing })
	out, err := Convert(ctx, ff, local, d.PostProcess)
	if err != nil {
		return err
	}
	m.update(id, func(x *model.Download) { x.FileName = filepath.Base(out) })
	return nil
}

// workDir returns a local directory for tools that cannot write to SMB.
func workDir(fs storage.FS) (dir string, remote bool, err error) {
	if l := fs.Local("x"); l != "" {
		return filepath.Dir(l), false, nil
	}
	dir = filepath.Join(os.TempDir(), "CrazyDownloadManager")
	return dir, true, os.MkdirAll(dir, 0o755)
}

func (m *Manager) runStream(ctx context.Context, id string, rj *runJob, d model.Download) error {
	s := m.settings.Get()
	ff := FindTool(s.FFmpegPath, "ffmpeg")
	if ff == "" {
		return fatal(errors.New("ffmpeg not found: install it or set its path in Options → Media"))
	}
	fs, err := storage.Open(d.Dir, s)
	if err != nil {
		return fatal(err)
	}
	defer fs.Close()
	name := d.FileName
	if name == "" {
		base := FileNameFromURL(d.URL)
		name = strings.TrimSuffix(base, filepath.Ext(base)) + streamExt(d.URL)
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext == ".m3u8" || ext == ".mpd" || ext == "" {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + streamExt(d.URL)
	}
	name = UniqueName(fs, name)
	dir, remote, err := workDir(fs)
	if err != nil {
		return fatal(err)
	}
	ext := filepath.Ext(name)
	tmp := filepath.Join(dir, strings.TrimSuffix(name, ext)+PartSuffix+ext)
	_ = os.Remove(tmp) // streams restart from the beginning

	m.update(id, func(x *model.Download) {
		x.FileName, x.Status, x.Resumable, x.Segments = name, model.StatusDownloading, false, nil
	})
	pj := &processJob{counter: &rj.counter, size: -1}
	pf := pj.progress
	rj.prog.Store(&pf)

	var live bool
	for _, in := range ffmpegInputs(d.URL) {
		live, err = runFFmpeg(ctx, ff, d, in, tmp, pj)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil {
		// Stopping a live recording is how it ends: keep what we have.
		if st, e := os.Stat(tmp); !(live && e == nil && st.Size() > 0) {
			_ = os.Remove(tmp)
			return ctx.Err()
		}
		err = nil
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	m.update(id, func(x *model.Download) { x.Status = model.StatusProcessing })
	if remote {
		err = fs.Upload(tmp, name)
	} else {
		err = os.Rename(tmp, filepath.Join(dir, name))
	}
	if err != nil {
		return fatal(err)
	}
	return m.postProcess(context.Background(), id, d, fs, name)
}

func (m *Manager) runMedia(ctx context.Context, id string, rj *runJob, d model.Download) error {
	s := m.settings.Get()
	yt := FindTool(s.YtDlpPath, "yt-dlp")
	if yt == "" {
		return fatal(errors.New("yt-dlp not found: install it or set its path in Options → Media"))
	}
	fs, err := storage.Open(d.Dir, s)
	if err != nil {
		return fatal(err)
	}
	defer fs.Close()
	dir, remote, err := workDir(fs)
	if err != nil {
		return fatal(err)
	}
	m.update(id, func(x *model.Download) { x.Status = model.StatusDownloading })
	pj := &processJob{counter: &rj.counter, size: -1}
	pf := pj.progress
	rj.prog.Store(&pf)
	out, err := runYtDlp(ctx, yt, FindTool(s.FFmpegPath, "ffmpeg"), d, dir, pj)
	if err != nil {
		return err
	}
	name := filepath.Base(out)
	if remote {
		m.update(id, func(x *model.Download) { x.Status = model.StatusProcessing })
		if err := fs.Upload(out, name); err != nil {
			return fatal(err)
		}
	}
	m.update(id, func(x *model.Download) { x.FileName = name })
	return nil
}

// antivirus runs the configured scanner on a finished local file.
func (m *Manager) antivirus(d model.Download) {
	cmdline := strings.TrimSpace(m.settings.Get().AntivirusCmd)
	if cmdline == "" || storage.IsSMB(d.Dir) {
		return
	}
	path := filepath.Join(d.Dir, d.FileName)
	args := splitArgs(cmdline)
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "%f", path)
	}
	if !strings.Contains(cmdline, "%f") {
		args = append(args, path)
	}
	cmd := exec.Command(args[0], args[1:]...)
	hideWindow(cmd)
	_ = cmd.Start()
	go func() { _ = cmd.Wait() }()
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// LocalPath returns a path the OS can open for a download's file, converting
// smb:// destinations to UNC (Windows) or leaving them as URLs (macOS).
func (m *Manager) LocalPath(id string) (file, dir string, err error) {
	d, ok := m.Get(id)
	if !ok {
		return "", "", errors.New("download not found")
	}
	dir = d.Dir
	if storage.IsSMB(dir) {
		u, share, sub, perr := storage.ParseSMB(dir)
		if perr != nil {
			return "", "", perr
		}
		if runtime.GOOS == "windows" {
			dir = `\\` + u.Hostname() + `\` + share
			if sub != "" {
				dir += `\` + strings.ReplaceAll(sub, "/", `\`)
			}
		}
	}
	name := d.FileName
	if d.Status != model.StatusCompleted && name != "" {
		name += PartSuffix
	}
	if storage.IsSMB(dir) {
		return strings.TrimRight(dir, "/") + "/" + url.PathEscape(name), dir, nil
	}
	return filepath.Join(dir, name), dir, nil
}
