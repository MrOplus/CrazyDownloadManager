// Package grabber is the site spider: it crawls a site from a start page and
// collects files to download, either matching file-type filters or the whole
// site for offline browsing (mirror mode).
package grabber

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

type Mode string

const (
	ModeFiles  Mode = "files"  // only files matching Include
	ModeMirror Mode = "mirror" // pages + resources, saved in a folder tree
)

type Options struct {
	StartURL    string   `json:"startUrl"`
	Mode        Mode     `json:"mode"`
	Depth       int      `json:"depth"`       // link levels to follow from the start page
	SameHost    bool     `json:"sameHost"`    // stay on the start host
	StayBelow   bool     `json:"stayBelow"`   // do not go above the start directory
	Include     []string `json:"include"`     // glob on file name, e.g. *.jpg, *.pdf
	Exclude     []string `json:"exclude"`     // glob on file name or URL substring
	MaxPages    int      `json:"maxPages"`    // crawl budget
	Concurrency int      `json:"concurrency"` // parallel page fetches
	Cookies     string   `json:"cookies"`
	UserAgent   string   `json:"userAgent"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
}

type Item struct {
	URL     string `json:"url"`
	SaveAs  string `json:"saveAs"` // relative path (mirror mode) or file name
	Referer string `json:"referer"`
	Type    string `json:"type"` // page | image | file | media | style | script
}

type Progress struct {
	Pages   int    `json:"pages"`
	Found   int    `json:"found"`
	Current string `json:"current"`
}

var linkAttrs = map[string][]string{
	"a": {"href"}, "area": {"href"}, "link": {"href"}, "img": {"src", "data-src"},
	"script": {"src"}, "source": {"src"}, "video": {"src", "poster"}, "audio": {"src"},
	"iframe": {"src"}, "frame": {"src"}, "embed": {"src"}, "object": {"data"},
	"track": {"src"},
}

// Scan crawls and returns the items to download.
func Scan(ctx context.Context, client *http.Client, opt Options, report func(Progress)) ([]Item, error) {
	start, err := url.Parse(strings.TrimSpace(opt.StartURL))
	if err != nil {
		return nil, err
	}
	if opt.MaxPages <= 0 {
		opt.MaxPages = 500
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 4
	}
	if opt.Mode == "" {
		opt.Mode = ModeFiles
	}
	baseDir := path.Dir(start.Path)
	if strings.HasSuffix(start.Path, "/") {
		baseDir = strings.TrimSuffix(start.Path, "/")
	}

	type task struct {
		u     *url.URL
		depth int
	}
	var (
		mu    sync.Mutex
		seen  = map[string]bool{}
		items []Item
		pages int
		wg    sync.WaitGroup
		sem   = make(chan struct{}, opt.Concurrency)
	)
	addItem := func(it Item) {
		if seen["item:"+it.URL] {
			return
		}
		seen["item:"+it.URL] = true
		items = append(items, it)
	}

	inScope := func(u *url.URL) bool {
		if u.Scheme != "http" && u.Scheme != "https" {
			return false
		}
		if opt.SameHost && !strings.EqualFold(u.Hostname(), start.Hostname()) {
			return false
		}
		if opt.StayBelow && !strings.HasPrefix(u.Path, baseDir) {
			return false
		}
		return !matchAny(opt.Exclude, u)
	}

	var crawl func(t task)
	crawl = func(t task) {
		defer wg.Done()
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		links, isHTML, err := fetchPage(ctx, client, opt, t.u)
		<-sem
		if err != nil || !isHTML {
			return
		}
		mu.Lock()
		pages++
		if opt.Mode == ModeMirror {
			addItem(Item{URL: t.u.String(), SaveAs: mirrorPath(t.u, true), Type: "page"})
		}
		if report != nil {
			report(Progress{Pages: pages, Found: len(items), Current: t.u.String()})
		}
		var next []task
		for _, l := range links {
			u, err := t.u.Parse(l.href)
			if err != nil {
				continue
			}
			u.Fragment = ""
			typ := classify(l.tag, u)
			follow := typ == "page" && (l.tag == "a" || l.tag == "area" || l.tag == "iframe" || l.tag == "frame")
			if follow {
				key := "page:" + u.String()
				if !seen[key] && t.depth < opt.Depth && pages+len(next) < opt.MaxPages && inScope(u) {
					seen[key] = true
					next = append(next, task{u: u, depth: t.depth + 1})
				}
				continue
			}
			if typ == "page" {
				continue
			}
			switch opt.Mode {
			case ModeMirror:
				if inScope(u) || (typ != "page" && !matchAny(opt.Exclude, u)) {
					addItem(Item{URL: u.String(), SaveAs: mirrorPath(u, false), Referer: t.u.String(), Type: typ})
				}
			default:
				if matchInclude(opt.Include, u) && !matchAny(opt.Exclude, u) {
					addItem(Item{URL: u.String(), SaveAs: path.Base(u.Path), Referer: t.u.String(), Type: typ})
				}
			}
		}
		mu.Unlock()
		for _, n := range next {
			wg.Add(1)
			go crawl(n)
		}
	}

	seen["page:"+start.String()] = true
	wg.Add(1)
	go crawl(task{u: start, depth: 0})
	wg.Wait()

	// In "files" mode a page link with a matching extension (e.g. *.html) is a file too.
	return items, ctx.Err()
}

type link struct{ tag, href string }

func fetchPage(ctx context.Context, client *http.Client, opt Options, u *url.URL) ([]link, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, err
	}
	if opt.UserAgent != "" {
		req.Header.Set("User-Agent", opt.UserAgent)
	}
	if opt.Cookies != "" {
		req.Header.Set("Cookie", opt.Cookies)
	}
	if opt.Username != "" {
		req.SetBasicAuth(opt.Username, opt.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "html") {
		return nil, false, nil
	}
	return extractLinks(io.LimitReader(resp.Body, 8<<20)), true, nil
}

func extractLinks(r io.Reader) []link {
	var out []link
	z := html.NewTokenizer(r)
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			attrs, ok := linkAttrs[t.Data]
			for _, a := range t.Attr {
				if ok {
					for _, want := range attrs {
						if a.Key == want && a.Val != "" && !strings.HasPrefix(a.Val, "javascript:") && !strings.HasPrefix(a.Val, "data:") && !strings.HasPrefix(a.Val, "mailto:") {
							out = append(out, link{tag: t.Data, href: strings.TrimSpace(a.Val)})
						}
					}
				}
				if a.Key == "srcset" {
					for _, part := range strings.Split(a.Val, ",") {
						if f := strings.Fields(part); len(f) > 0 {
							out = append(out, link{tag: "img", href: f[0]})
						}
					}
				}
			}
		}
	}
}

var pageExt = map[string]bool{"": true, ".html": true, ".htm": true, ".php": true, ".asp": true, ".aspx": true, ".jsp": true, ".shtml": true, ".cgi": true}
var imageExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".svg": true, ".ico": true, ".avif": true, ".tif": true, ".tiff": true}
var mediaExt = map[string]bool{".mp4": true, ".mkv": true, ".webm": true, ".mp3": true, ".m4a": true, ".ogg": true, ".flac": true, ".wav": true, ".avi": true, ".mov": true, ".wmv": true, ".flv": true}

func classify(tag string, u *url.URL) string {
	ext := strings.ToLower(path.Ext(u.Path))
	switch {
	case tag == "img" || imageExt[ext]:
		return "image"
	case tag == "video" || tag == "audio" || tag == "source" || mediaExt[ext]:
		return "media"
	case ext == ".css":
		return "style"
	case ext == ".js" || tag == "script":
		return "script"
	case pageExt[ext]:
		if tag == "link" {
			return "style"
		}
		return "page"
	}
	return "file"
}

func matchInclude(patterns []string, u *url.URL) bool {
	if len(patterns) == 0 {
		return classify("", u) != "page"
	}
	return matchAny(patterns, u)
}

func matchAny(patterns []string, u *url.URL) bool {
	name := strings.ToLower(path.Base(u.Path))
	full := strings.ToLower(u.String())
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if ok, _ := path.Match(p, name); ok {
			return true
		}
		if !strings.ContainsAny(p, "*?[") && strings.Contains(full, p) {
			return true
		}
	}
	return false
}

// mirrorPath maps a URL to host/dir/file inside the project folder.
func mirrorPath(u *url.URL, page bool) string {
	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") {
		p += "index.html"
	} else if page && path.Ext(p) == "" {
		p += "/index.html"
	}
	if u.RawQuery != "" {
		ext := path.Ext(p)
		p = strings.TrimSuffix(p, ext) + "_" + sanitize(u.RawQuery) + ext
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i := range parts {
		parts[i] = sanitize(parts[i])
	}
	return path.Join(append([]string{sanitize(u.Host)}, parts...)...)
}

func sanitize(s string) string {
	r := strings.NewReplacer("<", "_", ">", "_", ":", "_", `"`, "_", "\\", "_", "|", "_", "?", "_", "*", "_", "&", "_", "=", "-")
	s = r.Replace(s)
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
