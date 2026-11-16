// Package config stores user settings as JSON in the OS config directory.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const AppName = "CrazyDownloadManager"

const DefaultUserAgent = "Mozilla/5.0 (compatible; CrazyDownloadManager/0.1)"

type Category struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
	Dir        string   `json:"dir"`
}

// SMBCredential is used for smb://host/share/... destinations.
type SMBCredential struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Domain   string `json:"domain"`
}

type Schedule struct {
	Enabled    bool   `json:"enabled"`
	StartTime  string `json:"startTime"`  // "HH:MM", empty = no automatic start
	StopTime   string `json:"stopTime"`   // "HH:MM", empty = no automatic stop
	Days       []int  `json:"days"`       // 0=Sunday … 6=Saturday, empty = every day
	OnComplete string `json:"onComplete"` // "none" | "exit" | "shutdown" | "sleep"
}

type Queue struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	MaxConcurrent int      `json:"maxConcurrent"`
	Schedule      Schedule `json:"schedule"`
}

type Settings struct {
	DefaultDir         string          `json:"defaultDir"`
	MaxConnections     int             `json:"maxConnections"`
	GlobalSpeedLimit   int64           `json:"globalSpeedLimit"` // KB/s, 0 = unlimited
	MaxRetries         int             `json:"maxRetries"`
	RetryDelaySec      int             `json:"retryDelaySec"`
	TimeoutSec         int             `json:"timeoutSec"`
	Proxy              string          `json:"proxy"` // http://, https://, socks5://user:pass@host:port; "system" = env
	UserAgent          string          `json:"userAgent"`
	BridgePort         int             `json:"bridgePort"`
	BridgeToken        string          `json:"bridgeToken"`
	ShowAddDialog      bool            `json:"showAddDialog"`
	ShowCompleteDialog bool            `json:"showCompleteDialog"`
	ClipboardMonitor   bool            `json:"clipboardMonitor"`
	BrowserIntercept   bool            `json:"browserIntercept"`
	InterceptTypes     []string        `json:"interceptTypes"`
	SkipSites          []string        `json:"skipSites"`
	FFmpegPath         string          `json:"ffmpegPath"`
	YtDlpPath          string          `json:"ytDlpPath"`
	AntivirusCmd       string          `json:"antivirusCmd"` // e.g. "C:\\av\\scan.exe" "%f"
	Theme              string          `json:"theme"`        // system | light | dark
	Categories         []Category      `json:"categories"`
	Queues             []Queue         `json:"queues"`
	SMB                []SMBCredential `json:"smb"`
}

func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, AppName)
}

func defaultDownloadDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads", AppName)
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Defaults() Settings {
	root := defaultDownloadDir()
	return Settings{
		DefaultDir:     root,
		MaxConnections: 8,
		MaxRetries:     10,
		RetryDelaySec:  5,
		TimeoutSec:     30,
		// An honest UA: CDNs with bot protection drop connections whose UA
		// claims to be Chrome while the TLS fingerprint is Go's.
		UserAgent:          DefaultUserAgent,
		BridgePort:         47100,
		BridgeToken:        randomToken(),
		ShowAddDialog:      true,
		ShowCompleteDialog: true,
		ClipboardMonitor:   false,
		BrowserIntercept:   true,
		InterceptTypes:     strings.Fields(`3gp 7z aac ace aif apk arj asf avi bin bz2 deb dmg exe flac flv gz gzip img iso lzh m4a m4v mkv mov mp3 mp4 mpeg mpg msi msu ogg ogv pdf pkg rar rpm sit tar tgz tif tiff wav webm wma wmv xz z zip zst`),
		Theme:              "system",
		Categories: []Category{
			{Name: "Compressed", Extensions: strings.Fields("zip rar 7z gz bz2 xz tar tgz zst arj lzh ace z iso img"), Dir: filepath.Join(root, "Compressed")},
			{Name: "Documents", Extensions: strings.Fields("pdf doc docx xls xlsx ppt pptx txt rtf odt ods epub csv"), Dir: filepath.Join(root, "Documents")},
			{Name: "Music", Extensions: strings.Fields("mp3 wav wma aac flac ogg m4a aif opus"), Dir: filepath.Join(root, "Music")},
			{Name: "Programs", Extensions: strings.Fields("exe msi msu dmg pkg apk deb rpm appimage bin"), Dir: filepath.Join(root, "Programs")},
			{Name: "Video", Extensions: strings.Fields("mp4 mkv avi mov wmv flv webm m4v mpg mpeg 3gp ts asf ogv"), Dir: filepath.Join(root, "Video")},
		},
		Queues: []Queue{
			{ID: "main", Name: "Main Download Queue", MaxConcurrent: 3, Schedule: Schedule{OnComplete: "none"}},
			{ID: "sync", Name: "Synchronization Queue", MaxConcurrent: 1, Schedule: Schedule{OnComplete: "none"}},
		},
	}
}

// Store is a thread-safe holder of Settings backed by a JSON file.
type Store struct {
	mu   sync.RWMutex
	path string
	s    Settings
}

func Load() (*Store, error) {
	st := &Store{path: filepath.Join(Dir(), "settings.json"), s: Defaults()}
	data, err := os.ReadFile(st.path)
	if err == nil {
		if err := json.Unmarshal(data, &st.s); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	st.normalize()
	return st, st.save()
}

// Fallback is used when settings.json is unreadable: defaults are saved to a
// separate file so the broken one is kept for the user to inspect.
func Fallback() *Store {
	st := &Store{path: filepath.Join(Dir(), "settings.recovered.json"), s: Defaults()}
	_ = st.save()
	return st
}

func (st *Store) normalize() {
	d := Defaults()
	s := &st.s
	if s.DefaultDir == "" {
		s.DefaultDir = d.DefaultDir
	}
	if s.MaxConnections <= 0 {
		s.MaxConnections = d.MaxConnections
	}
	if s.MaxConnections > 32 {
		s.MaxConnections = 32
	}
	if s.TimeoutSec <= 0 {
		s.TimeoutSec = d.TimeoutSec
	}
	if s.BridgePort == 0 {
		s.BridgePort = d.BridgePort
	}
	if s.BridgeToken == "" {
		s.BridgeToken = d.BridgeToken
	}
	if s.UserAgent == "" || strings.Contains(s.UserAgent, "Chrome/131.0 Safari") {
		s.UserAgent = d.UserAgent
	}
	if len(s.Queues) == 0 {
		s.Queues = d.Queues
	}
	for i := range s.Queues {
		if s.Queues[i].MaxConcurrent <= 0 {
			s.Queues[i].MaxConcurrent = 1
		}
	}
}

func (st *Store) Get() Settings {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s
}

func (st *Store) Set(s Settings) error {
	st.mu.Lock()
	st.s = s
	st.normalize()
	st.mu.Unlock()
	return st.save()
}

func (st *Store) save() error {
	st.mu.RLock()
	data, err := json.MarshalIndent(st.s, "", "  ")
	st.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// CategoryFor picks the category whose extension list matches name.
func (s Settings) CategoryFor(name string) (Category, bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if ext == "" {
		return Category{}, false
	}
	for _, c := range s.Categories {
		for _, e := range c.Extensions {
			if strings.EqualFold(e, ext) {
				return c, true
			}
		}
	}
	return Category{}, false
}

// DirFor returns the save directory for a file name based on categories.
func (s Settings) DirFor(name string) (dir, category string) {
	if c, ok := s.CategoryFor(name); ok && c.Dir != "" {
		return c.Dir, c.Name
	}
	return s.DefaultDir, "General"
}

func (s Settings) Queue(id string) (Queue, bool) {
	for _, q := range s.Queues {
		if q.ID == id {
			return q, true
		}
	}
	return Queue{}, false
}

func (s Settings) SMBFor(host string) (SMBCredential, bool) {
	for _, c := range s.SMB {
		if strings.EqualFold(c.Host, host) {
			return c, true
		}
	}
	return SMBCredential{}, false
}
