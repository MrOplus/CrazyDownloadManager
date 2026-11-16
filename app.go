package main

import (
	"context"
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"crazydownloadmanager/internal/bridge"
	"crazydownloadmanager/internal/config"
	"crazydownloadmanager/internal/engine"
	"crazydownloadmanager/internal/grabber"
	"crazydownloadmanager/internal/model"
	"crazydownloadmanager/internal/storage"
)

const Version = "0.1.0"

// App is bound to the frontend: every exported method is callable from JS.
type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	settings *config.Store
	mgr      *engine.Manager
	initErr  error

	bridgeMu  sync.Mutex
	bridge    *bridge.Server
	bridgeErr string

	grabMu     sync.Mutex
	grabCancel context.CancelFunc

	ready chan struct{}
}

func NewApp() *App {
	a := &App{ready: make(chan struct{})}
	st, err := config.Load()
	if err != nil {
		a.initErr = err
		st = config.Fallback()
	}
	a.settings = st
	mgr, err := engine.NewManager(st, a.emit)
	if err != nil {
		a.initErr = err
	}
	a.mgr = mgr
	return a
}

func (a *App) emit(name string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
	if name == engine.EventQueueDone {
		if qd, ok := data.(engine.QueueDone); ok && qd.Scheduled {
			a.afterQueue(qd.Queue.Schedule.OnComplete)
		}
	}
}

func (a *App) afterQueue(action string) {
	switch action {
	case "exit":
		go func() { time.Sleep(2 * time.Second); runtime.Quit(a.ctx) }()
	case "shutdown":
		a.mgr.Shutdown(10 * time.Second)
		_ = systemAction("shutdown")
	case "sleep":
		_ = systemAction("sleep")
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runCtx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	if a.mgr == nil {
		close(a.ready)
		return
	}
	go a.mgr.Run(runCtx)
	a.restartBridge()
	go a.clipboardLoop(runCtx)
	close(a.ready)
}

func (a *App) domReady(ctx context.Context) {
	if a.mgr == nil {
		return
	}
	a.handleArgs(osArgs())
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	a.bridgeMu.Lock()
	a.bridge.Close()
	a.bridgeMu.Unlock()
	if a.mgr != nil {
		a.mgr.Shutdown(5 * time.Second)
	}
}

func (a *App) onSecondInstance(data options.SecondInstanceData) {
	<-a.ready
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	a.handleArgs(data.Args)
}

// ---------------------------------------------------------------- command line

// handleArgs accepts IDM-compatible switches (/d URL /p dir /f name /n /a)
// and the dash forms (-d, --download …). A bare URL also works.
func (a *App) handleArgs(args []string) {
	var req model.AddRequest
	silent, queueOnly := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key := strings.ToLower(strings.TrimLeft(arg, "/-"))
		val := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && (key == "d" || key == "download"):
			req.URL = val()
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && (key == "p" || key == "path"):
			req.Dir = val()
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && (key == "f" || key == "file"):
			req.FileName = val()
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && (key == "n" || key == "silent"):
			silent = true
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && (key == "a" || key == "queue"):
			queueOnly = true
		case (strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "-")) && key == "s":
			a.mgr.StartQueue("main")
		case looksLikeURL(arg):
			req.URL = arg
		}
	}
	if req.URL == "" {
		return
	}
	if silent || queueOnly {
		req.StartNow = !queueOnly
		_, _ = a.mgr.Add(req)
		return
	}
	runtime.EventsEmit(a.ctx, "ui:add", []bridge.Item{{URL: req.URL, FileName: req.FileName}})
}

var urlRe = regexp.MustCompile(`^(https?|ftps?|mms[ht]?|rtmps?|rtsp)://\S+$`)

func looksLikeURL(s string) bool { return urlRe.MatchString(strings.TrimSpace(s)) }

// ---------------------------------------------------------------- clipboard

func (a *App) clipboardLoop(ctx context.Context) {
	last, _ := runtime.ClipboardGetText(a.ctx)
	t := time.NewTicker(1200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s := a.settings.Get()
		if !s.ClipboardMonitor {
			continue
		}
		text, err := runtime.ClipboardGetText(a.ctx)
		if err != nil || text == last {
			continue
		}
		last = text
		text = strings.TrimSpace(text)
		if !looksLikeURL(text) {
			continue
		}
		if engine.DetectKind(text, "") == model.KindFile {
			ext := strings.TrimPrefix(strings.ToLower(path.Ext(urlPath(text))), ".")
			if !contains(s.InterceptTypes, ext) {
				continue
			}
		}
		runtime.EventsEmit(a.ctx, "ui:add", []bridge.Item{{URL: text}})
	}
}

func urlPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- browser bridge

func (a *App) restartBridge() {
	a.bridgeMu.Lock()
	defer a.bridgeMu.Unlock()
	a.bridge.Close()
	srv, err := bridge.Start(a.settings.Get().BridgePort, a)
	a.bridge = srv
	a.bridgeErr = ""
	if err != nil {
		a.bridgeErr = err.Error()
	}
}

func (a *App) Token() string { return a.settings.Get().BridgeToken }

func (a *App) BridgeConfig() bridge.Config {
	s := a.settings.Get()
	return bridge.Config{Enabled: s.BrowserIntercept, Types: s.InterceptTypes, Skip: s.SkipSites, Version: Version}
}

func (a *App) BridgeAdd(req bridge.AddRequest) error {
	s := a.settings.Get()
	if req.Silent || (!s.ShowAddDialog && !req.Batch) {
		for _, it := range req.Items {
			if _, err := a.mgr.Add(itemToRequest(it, req.Page, true)); err != nil {
				return err
			}
		}
		return nil
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	if req.Batch {
		runtime.EventsEmit(a.ctx, "ui:batch", req)
	} else {
		runtime.EventsEmit(a.ctx, "ui:add", req.Items)
	}
	return nil
}

func itemToRequest(it bridge.Item, desc string, start bool) model.AddRequest {
	return model.AddRequest{
		URL: it.URL, FileName: it.FileName, Referer: it.Referer, Cookies: it.Cookies,
		UserAgent: it.UserAgent, Headers: it.Headers, Kind: model.Kind(it.Kind),
		Description: desc, StartNow: start, Size: it.Size,
	}
}

// ================================================================ bound API

type Info struct {
	Version   string `json:"version"`
	InitError string `json:"initError"`
	BridgeErr string `json:"bridgeError"`
	FFmpeg    string `json:"ffmpeg"`
	YtDlp     string `json:"ytdlp"`
	ConfigDir string `json:"configDir"`
	OS        string `json:"os"`
}

func (a *App) GetInfo() Info {
	s := a.settings.Get()
	i := Info{
		Version: Version, FFmpeg: engine.FindTool(s.FFmpegPath, "ffmpeg"),
		YtDlp: engine.FindTool(s.YtDlpPath, "yt-dlp"), ConfigDir: config.Dir(),
		OS: goos(),
	}
	if a.initErr != nil {
		i.InitError = a.initErr.Error()
	}
	a.bridgeMu.Lock()
	i.BridgeErr = a.bridgeErr
	a.bridgeMu.Unlock()
	return i
}

func (a *App) GetSettings() config.Settings { return a.settings.Get() }

func (a *App) SaveSettings(s config.Settings) error {
	old := a.settings.Get()
	if err := a.settings.Set(s); err != nil {
		return err
	}
	a.mgr.ApplySettings()
	if old.BridgePort != s.BridgePort {
		a.restartBridge()
	}
	return nil
}

func (a *App) RegenerateToken() (string, error) {
	s := a.settings.Get()
	s.BridgeToken = config.Defaults().BridgeToken
	return s.BridgeToken, a.settings.Set(s)
}

func (a *App) ListDownloads() []model.Download { return a.mgr.List() }

func (a *App) ProbeURL(req model.AddRequest) (model.ProbeResult, error) {
	return a.mgr.Probe(a.ctx, req)
}

func (a *App) AddDownload(req model.AddRequest) (model.Download, error) {
	return a.mgr.Add(req)
}

func (a *App) AddBatch(reqs []model.AddRequest) (int, error) {
	n := 0
	for _, r := range reqs {
		if _, err := a.mgr.Add(r); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (a *App) Resume(ids []string) {
	for _, id := range ids {
		a.mgr.Resume(id)
	}
}

func (a *App) Pause(ids []string) {
	for _, id := range ids {
		a.mgr.Pause(id)
	}
}

func (a *App) ResumeAll()                            { a.mgr.ResumeAll() }
func (a *App) PauseAll()                             { a.mgr.PauseAll() }
func (a *App) Remove(ids []string, deleteFiles bool) { go a.mgr.Remove(ids, deleteFiles) }
func (a *App) RemoveCompleted()                      { a.mgr.RemoveCompleted() }
func (a *App) Redownload(id string)                  { go a.mgr.Redownload(id) }
func (a *App) SetSpeedLimit(id string, kbps int64)   { a.mgr.SetSpeedLimit(id, kbps) }
func (a *App) StartQueue(id string)                  { a.mgr.StartQueue(id) }
func (a *App) StopQueue(id string)                   { a.mgr.StopQueue(id) }
func (a *App) QueueStates() map[string]bool          { return a.mgr.QueueStates() }
func (a *App) MoveToQueue(ids []string, q string)    { a.mgr.MoveToQueue(ids, q) }

func (a *App) UpdateDownload(id string, req model.AddRequest) error {
	return a.mgr.Update(id, req)
}

func (a *App) OpenFile(id string) error {
	file, _, err := a.mgr.LocalPath(id)
	if err != nil {
		return err
	}
	return openPath(file)
}

func (a *App) OpenFolder(id string) error {
	file, dir, err := a.mgr.LocalPath(id)
	if err != nil {
		return err
	}
	return revealPath(file, dir)
}

func (a *App) ChooseDirectory(current string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: "Choose folder", CanCreateDirectories: true}
	if current != "" && !storage.IsSMB(current) {
		opts.DefaultDirectory = current
	}
	return runtime.OpenDirectoryDialog(a.ctx, opts)
}

func (a *App) ChooseFile(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

func (a *App) TestSMB(dir string) error {
	if !storage.IsSMB(dir) {
		return errors.New("enter an smb://host/share/folder path")
	}
	return storage.TestSMB(dir, a.settings.Get())
}

func (a *App) CopyText(s string) { _ = runtime.ClipboardSetText(a.ctx, s) }

// ---------------------------------------------------------------- site grabber

func (a *App) GrabberScan(opt grabber.Options) ([]grabber.Item, error) {
	a.grabMu.Lock()
	if a.grabCancel != nil {
		a.grabCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	a.grabCancel = cancel
	a.grabMu.Unlock()
	defer cancel()
	s := a.settings.Get()
	if opt.UserAgent == "" {
		opt.UserAgent = s.UserAgent
	}
	var last time.Time
	items, err := grabber.Scan(ctx, engine.NewHTTPClient(s), opt, func(p grabber.Progress) {
		if time.Since(last) > 200*time.Millisecond {
			last = time.Now()
			runtime.EventsEmit(a.ctx, "grabber:progress", p)
		}
	})
	if errors.Is(err, context.Canceled) {
		err = nil // user pressed Stop: return what was found
	}
	return items, err
}

func (a *App) GrabberCancel() {
	a.grabMu.Lock()
	defer a.grabMu.Unlock()
	if a.grabCancel != nil {
		a.grabCancel()
	}
}

type GrabberAddRequest struct {
	Items    []grabber.Item `json:"items"`
	Dir      string         `json:"dir"`
	QueueID  string         `json:"queueId"`
	StartNow bool           `json:"startNow"`
	Cookies  string         `json:"cookies"`
	Username string         `json:"username"`
	Password string         `json:"password"`
}

// GrabberAdd queues grabbed items under one project folder, recreating the
// site's folder tree in mirror mode.
func (a *App) GrabberAdd(r GrabberAddRequest) (int, error) {
	if r.QueueID == "" {
		r.QueueID = "main"
	}
	n := 0
	for _, it := range r.Items {
		dir := r.Dir
		sub := path.Dir(it.SaveAs)
		if sub != "." && sub != "/" {
			if storage.IsSMB(dir) {
				dir = strings.TrimRight(dir, "/") + "/" + sub
			} else {
				dir = filepath.Join(dir, filepath.FromSlash(sub))
			}
		}
		_, err := a.mgr.Add(model.AddRequest{
			URL: it.URL, FileName: path.Base(it.SaveAs), Dir: dir, Category: "Grabber",
			QueueID: r.QueueID, Referer: it.Referer, Cookies: r.Cookies,
			Username: r.Username, Password: r.Password, Description: "Site grabber",
			StartNow: false, Kind: model.KindFile,
		})
		if err != nil {
			return n, err
		}
		n++
	}
	if r.StartNow {
		a.mgr.StartQueue(r.QueueID)
	}
	return n, nil
}
