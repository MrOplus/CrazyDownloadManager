package engine

import (
	"context"
	"io"
	"net"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"

	"crazydownloadmanager/internal/model"
)

// ftpSource opens one control+data connection per segment and uses REST
// (RetrFrom) to start each segment at its offset.
type ftpSource struct {
	d       model.Download
	u       *url.URL
	timeout time.Duration
}

func newFTPSource(d model.Download, timeout time.Duration) (*ftpSource, error) {
	u, err := url.Parse(d.URL)
	if err != nil {
		return nil, fatal(err)
	}
	return &ftpSource{d: d, u: u, timeout: timeout}, nil
}

func (s *ftpSource) dial(ctx context.Context) (*ftp.ServerConn, error) {
	host := s.u.Host
	if s.u.Port() == "" {
		host = net.JoinHostPort(s.u.Hostname(), "21")
	}
	opts := []ftp.DialOption{ftp.DialWithContext(ctx), ftp.DialWithTimeout(s.timeout)}
	if strings.EqualFold(s.u.Scheme, "ftps") {
		opts = append(opts, ftp.DialWithExplicitTLS(nil))
	}
	c, err := ftp.Dial(host, opts...)
	if err != nil {
		return nil, err
	}
	user, pass := "anonymous", "anonymous@"
	if s.u.User != nil {
		user = s.u.User.Username()
		pass, _ = s.u.User.Password()
	}
	if s.d.Username != "" {
		user, pass = s.d.Username, s.d.Password
	}
	if err := c.Login(user, pass); err != nil {
		c.Quit()
		return nil, fatal(err)
	}
	return c, nil
}

func (s *ftpSource) filePath() string {
	p, err := url.PathUnescape(s.u.Path)
	if err != nil {
		return s.u.Path
	}
	return p
}

func (s *ftpSource) Probe(ctx context.Context) (model.ProbeResult, error) {
	c, err := s.dial(ctx)
	if err != nil {
		return model.ProbeResult{}, err
	}
	defer c.Quit()
	r := model.ProbeResult{URL: s.d.URL, FinalURL: s.d.URL, Size: -1, Kind: model.KindFile}
	if size, err := c.FileSize(s.filePath()); err == nil {
		r.Size = size
		r.Resumable = true
	}
	r.FileName = SanitizeFileName(path.Base(s.filePath()))
	return r, nil
}

type ftpReader struct {
	io.Reader
	resp *ftp.Response
	conn *ftp.ServerConn
}

func (f *ftpReader) Close() error {
	// Closing a data connection mid-transfer makes some servers slow to
	// answer; don't let that block the worker.
	done := make(chan struct{})
	go func() { f.resp.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	return f.conn.Quit()
}

func (s *ftpSource) Open(ctx context.Context, start, end int64) (io.ReadCloser, error) {
	c, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.RetrFrom(s.filePath(), uint64(start))
	if err != nil {
		c.Quit()
		if strings.Contains(err.Error(), "550") {
			return nil, fatal(err)
		}
		return nil, err
	}
	var r io.Reader = resp
	if end >= 0 {
		r = io.LimitReader(resp, end-start)
	}
	return &ftpReader{Reader: r, resp: resp, conn: c}, nil
}
