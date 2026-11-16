package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crazydownloadmanager/internal/model"
)

// processJob tracks an external tool (ffmpeg / yt-dlp) for progress reports.
type processJob struct {
	mu         sync.Mutex
	size       int64
	downloaded int64
	counter    *atomic.Int64
	live       bool
}

func (p *processJob) progress() Progress {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Progress{Size: p.size, Downloaded: p.downloaded, Active: 1}
}

func (p *processJob) set(downloaded, size int64) {
	p.mu.Lock()
	if d := downloaded - p.downloaded; d > 0 {
		p.counter.Add(d)
	}
	p.downloaded = downloaded
	p.size = size
	p.mu.Unlock()
}

var durationRe = regexp.MustCompile(`Duration: (\d+):(\d+):(\d+(?:\.\d+)?)`)

func parseClock(h, m, s string) float64 {
	hh, _ := strconv.ParseFloat(h, 64)
	mm, _ := strconv.ParseFloat(m, 64)
	ss, _ := strconv.ParseFloat(s, 64)
	return hh*3600 + mm*60 + ss
}

// streamExt picks an output container for a stream URL.
func streamExt(raw string) string {
	u, _ := url.Parse(raw)
	if u != nil {
		switch strings.ToLower(u.Scheme) {
		case "mms", "mmsh", "mmst":
			return ".asf" // WMV/WMA codecs do not fit into MP4
		}
	}
	return ".mp4"
}

// ffmpegInputs expands mms:// into the transports ffmpeg actually speaks.
func ffmpegInputs(raw string) []string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "mms") {
		return []string{raw}
	}
	rest := strings.TrimPrefix(raw[len(u.Scheme):], "://")
	return []string{"mmsh://" + rest, "mmst://" + rest}
}

type stopper struct {
	stdin io.WriteCloser
	cmd   *exec.Cmd
}

// runFFmpeg records url into out (a local path) using stream copy. When ctx is
// cancelled it asks ffmpeg to finish cleanly ("q") so the file stays playable.
// It returns live=true when the input had no known duration.
func runFFmpeg(ctx context.Context, ffmpeg string, d model.Download, input, out string, pj *processJob) (live bool, err error) {
	args := []string{"-hide_banner", "-y", "-loglevel", "info", "-stats_period", "0.5", "-progress", "pipe:1"}
	if u, _ := url.Parse(input); u != nil && strings.HasPrefix(strings.ToLower(u.Scheme), "http") {
		ua := d.UserAgent
		if ua != "" {
			args = append(args, "-user_agent", ua)
		}
		if d.Referer != "" {
			args = append(args, "-referer", d.Referer)
		}
		var hdr strings.Builder
		if d.Cookies != "" {
			hdr.WriteString("Cookie: " + d.Cookies + "\r\n")
		}
		for k, v := range d.Headers {
			hdr.WriteString(k + ": " + v + "\r\n")
		}
		if hdr.Len() > 0 {
			args = append(args, "-headers", hdr.String())
		}
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "10")
	}
	args = append(args, "-i", input, "-map", "0:v?", "-map", "0:a?", "-c", "copy", out)

	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return false, fatal(fmt.Errorf("start ffmpeg: %w", err))
	}

	var duration atomic.Uint64 // milliseconds
	var tail []string
	var tailMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			if m := durationRe.FindStringSubmatch(line); m != nil && duration.Load() == 0 {
				duration.Store(uint64(parseClock(m[1], m[2], m[3]) * 1000))
			}
			tailMu.Lock()
			tail = append(tail, line)
			if len(tail) > 12 {
				tail = tail[1:]
			}
			tailMu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		var total, outUs int64
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if !ok {
				continue
			}
			switch k {
			case "total_size":
				total, _ = strconv.ParseInt(v, 10, 64)
			case "out_time_us":
				outUs, _ = strconv.ParseInt(v, 10, 64)
			case "progress":
				size := int64(-1)
				if dur := int64(duration.Load()); dur > 0 && outUs > 0 && total > 0 {
					size = int64(float64(total) * float64(dur*1000) / float64(outUs))
					if size < total {
						size = total
					}
				}
				if v == "end" && total > 0 {
					size = total
				}
				pj.set(total, size)
			}
		}
	}()

	exited := make(chan error, 1)
	go func() { wg.Wait(); exited <- cmd.Wait() }()

	stopped := false
	select {
	case err = <-exited:
	case <-ctx.Done():
		stopped = true
		_, _ = io.WriteString(stdin, "q\n")
		select {
		case err = <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			err = <-exited
		}
	}
	live = duration.Load() == 0
	if stopped {
		return live, ctx.Err()
	}
	if err != nil {
		tailMu.Lock()
		msg := strings.Join(tail, "\n")
		tailMu.Unlock()
		return live, fmt.Errorf("ffmpeg failed: %s", lastLines(msg, 3))
	}
	return live, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// Convert runs an ffmpeg post-processing preset on a finished local file and
// returns the new path.
func Convert(ctx context.Context, ffmpeg, in, preset string) (string, error) {
	base := strings.TrimSuffix(in, filepath.Ext(in))
	var args []string
	var out string
	switch preset {
	case "mp3":
		out = base + ".mp3"
		args = []string{"-vn", "-c:a", "libmp3lame", "-q:a", "2"}
	case "m4a":
		out = base + ".m4a"
		args = []string{"-vn", "-c:a", "aac", "-b:a", "192k"}
	case "mp4":
		out = base + ".mp4"
		if strings.EqualFold(filepath.Ext(in), ".mp4") {
			out = base + ".converted.mp4"
		}
		args = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "22", "-c:a", "aac", "-movflags", "+faststart"}
	default:
		return in, nil
	}
	full := append([]string{"-hide_banner", "-y", "-i", in}, args...)
	full = append(full, out)
	cmd := exec.CommandContext(ctx, ffmpeg, full...)
	hideWindow(cmd)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("convert to %s failed: %s", preset, lastLines(string(b), 3))
	}
	return out, nil
}

var ytProgress = regexp.MustCompile(`^CDM (\S+) (\S+)`)

// runYtDlp downloads a media page with yt-dlp into dir and returns the final path.
func runYtDlp(ctx context.Context, ytdlp, ffmpeg string, d model.Download, dir string, pj *processJob) (string, error) {
	tmpl := "%(title).150B [%(id)s].%(ext)s"
	if d.FileName != "" {
		tmpl = strings.TrimSuffix(d.FileName, filepath.Ext(d.FileName)) + ".%(ext)s"
	}
	args := []string{
		"--newline", "--no-colors", "--progress", "--continue", "--no-playlist",
		"--progress-template", "download:CDM %(progress.downloaded_bytes)s %(progress.total_bytes,progress.total_bytes_estimate)s",
		"--print", "after_move:filepath",
		"-P", dir, "-o", tmpl,
	}
	if ffmpeg != "" {
		args = append(args, "--ffmpeg-location", ffmpeg)
	}
	if d.Referer != "" {
		args = append(args, "--referer", d.Referer)
	}
	if d.UserAgent != "" {
		args = append(args, "--user-agent", d.UserAgent)
	}
	if d.Cookies != "" {
		args = append(args, "--add-header", "Cookie:"+d.Cookies)
	}
	if d.PostProcess == "mp3" || d.PostProcess == "m4a" {
		args = append(args, "-x", "--audio-format", d.PostProcess)
	} else {
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4")
	}
	args = append(args, d.URL)

	cmd := exec.CommandContext(ctx, ytdlp, args...)
	hideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return "", fatal(fmt.Errorf("start yt-dlp: %w", err))
	}
	var final string
	var errTail []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			errTail = append(errTail, sc.Text())
			if len(errTail) > 5 {
				errTail = errTail[1:]
			}
		}
	}()
	// yt-dlp downloads video and audio separately; keep a running base so the
	// total keeps growing across parts.
	var base, lastDone, lastTotal int64
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := ytProgress.FindStringSubmatch(line); m != nil {
			done, _ := strconv.ParseFloat(m[1], 64)
			total, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				total = -1
			}
			if int64(done) < lastDone { // next format started
				base += lastTotal
			}
			lastDone, lastTotal = int64(done), int64(total)
			size := int64(-1)
			if total > 0 {
				size = base + int64(total)
			}
			pj.set(base+int64(done), size)
			continue
		}
		if line != "" && !strings.HasPrefix(line, "[") {
			if _, err := os.Stat(line); err == nil {
				final = line
			}
		}
	}
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("yt-dlp failed: %s", strings.Join(errTail, " | "))
	}
	if final == "" {
		return "", errors.New("yt-dlp finished but reported no output file")
	}
	return final, nil
}
