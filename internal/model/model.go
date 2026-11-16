// Package model holds the data types shared between the engine, the
// persistence layer and the GUI bindings.
package model

import "time"

type Status string

const (
	StatusQueued      Status = "queued"      // waiting for its queue to pick it up
	StatusConnecting  Status = "connecting"  // probing / opening connections
	StatusDownloading Status = "downloading" // transferring data
	StatusPaused      Status = "paused"      // stopped by the user, resumable
	StatusProcessing  Status = "processing"  // post-processing (ffmpeg, antivirus, SMB upload)
	StatusCompleted   Status = "completed"
	StatusError       Status = "error"
)

// Active reports whether a download currently owns a worker.
func (s Status) Active() bool {
	return s == StatusConnecting || s == StatusDownloading || s == StatusProcessing
}

type Kind string

const (
	KindFile   Kind = "file"   // HTTP/HTTPS/FTP, segmented
	KindStream Kind = "stream" // HLS/DASH/MMS/RTMP/RTSP via ffmpeg
	KindMedia  Kind = "media"  // page URLs (YouTube etc.) via yt-dlp
)

// Segment is one byte range of a segmented download. End is exclusive and
// -1 while the total size is unknown.
type Segment struct {
	Start  int64 `json:"start"`
	End    int64 `json:"end"`
	Pos    int64 `json:"pos"`
	Active bool  `json:"active,omitempty"` // a connection is working on it right now
}

type Download struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	FileName    string            `json:"fileName"`
	Dir         string            `json:"dir"` // local path, UNC path or smb://host/share/path
	Category    string            `json:"category"`
	QueueID     string            `json:"queueId"`
	Kind        Kind              `json:"kind"`
	Status      Status            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Size        int64             `json:"size"` // -1 = unknown
	Downloaded  int64             `json:"downloaded"`
	Resumable   bool              `json:"resumable"`
	Connections int               `json:"connections"`
	Segments    []Segment         `json:"segments,omitempty"`
	Referer     string            `json:"referer,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Cookies     string            `json:"cookies,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Username    string            `json:"username,omitempty"`
	Password    string            `json:"password,omitempty"`
	Description string            `json:"description,omitempty"`
	SpeedLimit  int64             `json:"speedLimit"`            // KB/s, 0 = unlimited
	PostProcess string            `json:"postProcess,omitempty"` // "", "mp3", "mp4", "m4a"
	Added       time.Time         `json:"added"`
	LastTry     time.Time         `json:"lastTry"`
	Finished    time.Time         `json:"finished"`

	// Runtime values, refreshed by the manager while running.
	Speed       int64 `json:"speed"` // bytes/s
	ETA         int64 `json:"eta"`   // seconds, -1 unknown
	ActiveConns int   `json:"activeConns"`
}

// AddRequest is what the GUI, the browser extension and the command line
// send to create a download.
type AddRequest struct {
	URL         string            `json:"url"`
	FileName    string            `json:"fileName"`
	Dir         string            `json:"dir"`
	Category    string            `json:"category"`
	QueueID     string            `json:"queueId"`
	Kind        Kind              `json:"kind"`
	Referer     string            `json:"referer"`
	UserAgent   string            `json:"userAgent"`
	Cookies     string            `json:"cookies"`
	Headers     map[string]string `json:"headers"`
	Username    string            `json:"username"`
	Password    string            `json:"password"`
	Description string            `json:"description"`
	Connections int               `json:"connections"`
	SpeedLimit  int64             `json:"speedLimit"`
	PostProcess string            `json:"postProcess"`
	StartNow    bool              `json:"startNow"`
	Size        int64             `json:"size"`
}

// ProbeResult is returned to the "Download File Info" dialog.
type ProbeResult struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	Resumable   bool   `json:"resumable"`
	ContentType string `json:"contentType"`
	Kind        Kind   `json:"kind"`
	Category    string `json:"category"`
	Dir         string `json:"dir"`
}
