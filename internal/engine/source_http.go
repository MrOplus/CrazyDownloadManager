package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/go-ntlmssp"

	"crazydownloadmanager/internal/config"
	"crazydownloadmanager/internal/model"
)

// Source is a byte-range capable origin (HTTP, HTTPS, FTP).
type Source interface {
	Probe(ctx context.Context) (model.ProbeResult, error)
	// Open streams [start, end). end == -1 means "until EOF".
	Open(ctx context.Context, start, end int64) (io.ReadCloser, error)
}

// NewHTTPClient builds the client used for downloads and the site grabber.
//
// HTTP/2 is deliberately disabled: it multiplexes every request to a host
// over one TCP connection, which defeats multi-connection acceleration.
// Basic auth credentials also enable NTLM/Negotiate via go-ntlmssp.
func NewHTTPClient(s config.Settings) *http.Client {
	timeout := time.Duration(s.TimeoutSec) * time.Second
	tr := &http.Transport{
		Proxy:                 proxyFunc(s.Proxy),
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
		DisableCompression:    true, // byte ranges must address the raw entity
	}
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: ntlmssp.Negotiator{RoundTripper: tr},
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return fmt.Errorf("too many redirects")
			}
			// Keep browser-supplied headers across redirects (CDNs often redirect).
			for k, v := range via[0].Header {
				if _, ok := req.Header[k]; !ok {
					req.Header[k] = v
				}
			}
			return nil
		},
	}
}

func proxyFunc(p string) func(*http.Request) (*url.URL, error) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "system":
		return http.ProxyFromEnvironment
	case "direct", "none":
		return nil
	}
	u, err := url.Parse(p)
	if err != nil {
		return http.ProxyFromEnvironment
	}
	return http.ProxyURL(u)
}

type httpSource struct {
	client *http.Client
	d      model.Download
	ua     string // the app's own user agent
	url    string // final URL after redirects, used for segment requests
	ownUA  bool   // browser UA was rejected; use ua instead
}

func newHTTPSource(client *http.Client, d model.Download, ua string) *httpSource {
	return &httpSource{client: client, d: d, ua: ua, url: d.URL}
}

func (s *httpSource) request(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fatal(err)
	}
	ua := s.d.UserAgent
	if ua == "" || s.ownUA {
		ua = s.ua
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	if s.d.Referer != "" {
		req.Header.Set("Referer", s.d.Referer)
	}
	if s.d.Cookies != "" {
		req.Header.Set("Cookie", s.d.Cookies)
	}
	for k, v := range s.d.Headers {
		req.Header.Set(k, v)
	}
	user, pass := s.d.Username, s.d.Password
	if user == "" && req.URL.User != nil {
		user = req.URL.User.Username()
		pass, _ = req.URL.User.Password()
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	return req, nil
}

func statusError(resp *http.Response) error {
	err := fmt.Errorf("server returned %s", resp.Status)
	switch resp.StatusCode {
	case 400, 401, 403, 404, 405, 410, 451:
		return fatal(err)
	}
	return err
}

func (s *httpSource) Probe(ctx context.Context) (model.ProbeResult, error) {
	r, err := s.probe(ctx)
	if err != nil && !isFatal(err) && ctx.Err() == nil && s.d.UserAgent != "" && s.d.UserAgent != s.ua && !s.ownUA {
		// Bot protection often drops a browser UA sent with a non-browser TLS
		// fingerprint; retry once as ourselves.
		s.ownUA = true
		if r2, err2 := s.probe(ctx); err2 == nil {
			return r2, nil
		}
		s.ownUA = false
	}
	return r, err
}

func (s *httpSource) probe(ctx context.Context) (model.ProbeResult, error) {
	req, err := s.request(ctx, s.d.URL)
	if err != nil {
		return model.ProbeResult{}, err
	}
	req.Header.Set("Range", "bytes=0-")
	resp, err := s.client.Do(req)
	if err != nil {
		return model.ProbeResult{}, err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return model.ProbeResult{}, statusError(resp)
	}
	final := resp.Request.URL.String()
	s.url = final
	r := model.ProbeResult{
		URL:         s.d.URL,
		FinalURL:    final,
		Size:        -1,
		ContentType: resp.Header.Get("Content-Type"),
	}
	if resp.StatusCode == http.StatusPartialContent {
		r.Resumable = true
		if total := parseContentRangeTotal(resp.Header.Get("Content-Range")); total >= 0 {
			r.Size = total
		}
	} else if resp.ContentLength >= 0 {
		r.Size = resp.ContentLength
	}
	if r.Size < 0 {
		r.Resumable = false // dynamic segmentation needs a known length
	}
	r.FileName = FileNameFromDisposition(resp.Header.Get("Content-Disposition"))
	if r.FileName == "" {
		r.FileName = FileNameFromURL(final)
	}
	r.Kind = DetectKind(final, r.ContentType)
	return r, nil
}

func parseContentRangeTotal(h string) int64 {
	// "bytes 0-1023/4096"
	i := strings.LastIndex(h, "/")
	if i < 0 || h[i+1:] == "*" {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimSpace(h[i+1:]), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (s *httpSource) Open(ctx context.Context, start, end int64) (io.ReadCloser, error) {
	req, err := s.request(ctx, s.url)
	if err != nil {
		return nil, err
	}
	ranged := start > 0 || end >= 0
	if ranged {
		if end >= 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return nil, errNoRange
		}
		return nil, statusError(resp)
	}
	if ranged && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		if start == 0 {
			// Whole body from zero is still usable for the first segment.
			return s.Open(ctx, 0, -1)
		}
		return nil, errNoRange
	}
	return resp.Body, nil
}
