// Package storage abstracts where downloaded bytes are written: the local
// file system (including Windows UNC paths and macOS-mounted shares) or an
// SMB2/3 share reached directly with smb://host/share/path.
package storage

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hirochachacha/go-smb2"

	"crazydownloadmanager/internal/config"
)

// File is an open destination file that supports random-access writes, which
// segmented downloading needs.
type File interface {
	io.WriterAt
	Truncate(size int64) error
	Close() error
}

// FS is a destination file system rooted at a download directory.
type FS interface {
	// Open opens (creating if needed) name for writing without truncating it.
	Open(name string) (File, error)
	Rename(oldName, newName string) error
	Remove(name string) error
	Exists(name string) bool
	// Upload copies a local file into the FS (used for ffmpeg output on SMB).
	Upload(localPath, name string) error
	// Local returns the local path of name, or "" if the FS is remote.
	Local(name string) string
	Close() error
}

// IsSMB reports whether dir is an smb:// URL.
func IsSMB(dir string) bool {
	return strings.HasPrefix(strings.ToLower(dir), "smb://")
}

// Open returns an FS rooted at dir, creating the directory if needed.
func Open(dir string, settings config.Settings) (FS, error) {
	if IsSMB(dir) {
		return openSMB(dir, settings)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return localFS{root: dir}, nil
}

// ---------------------------------------------------------------- local

type localFS struct{ root string }

func (l localFS) p(name string) string { return filepath.Join(l.root, name) }

func (l localFS) Open(name string) (File, error) {
	if err := os.MkdirAll(filepath.Dir(l.p(name)), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(l.p(name), os.O_RDWR|os.O_CREATE, 0o644)
}
func (l localFS) Rename(o, n string) error { return os.Rename(l.p(o), l.p(n)) }
func (l localFS) Remove(name string) error { return os.Remove(l.p(name)) }
func (l localFS) Exists(name string) bool {
	_, err := os.Stat(l.p(name))
	return err == nil
}
func (l localFS) Local(name string) string { return l.p(name) }
func (l localFS) Close() error             { return nil }
func (l localFS) Upload(src, name string) error {
	if filepath.Clean(src) == filepath.Clean(l.p(name)) {
		return nil
	}
	if err := os.Rename(src, l.p(name)); err == nil {
		return nil
	}
	return copyFile(src, l.p(name))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// ---------------------------------------------------------------- SMB

type smbFS struct {
	conn    net.Conn
	session *smb2.Session
	share   *smb2.Share
	root    string // path inside the share, backslash separated
}

// ParseSMB splits smb://[user[:pass]@]host[:port]/share/dir into parts.
func ParseSMB(raw string) (u *url.URL, share, dir string, err error) {
	u, err = url.Parse(raw)
	if err != nil {
		return nil, "", "", err
	}
	parts := strings.SplitN(strings.Trim(u.Path, "/"), "/", 2)
	if parts[0] == "" {
		return nil, "", "", fmt.Errorf("smb url %q has no share name", raw)
	}
	share = parts[0]
	if len(parts) == 2 {
		dir = parts[1]
	}
	return u, share, dir, nil
}

func openSMB(raw string, settings config.Settings) (FS, error) {
	u, share, dir, err := ParseSMB(raw)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	cred, _ := settings.SMBFor(host)
	port := u.Port()
	if port == "" {
		port = "445"
		if cred.Port > 0 {
			port = strconv.Itoa(cred.Port)
		}
	}
	user, pass, domain := cred.User, cred.Password, cred.Domain
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}
	if i := strings.IndexAny(user, `\;`); i > 0 {
		domain, user = user[:i], user[i+1:]
	}
	if user == "" {
		user = "guest"
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("smb connect %s: %w", host, err)
	}
	d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: pass, Domain: domain}}
	s, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smb login %s: %w", host, err)
	}
	sh, err := s.Mount(share)
	if err != nil {
		s.Logoff()
		conn.Close()
		return nil, fmt.Errorf("smb mount %s: %w", share, err)
	}
	fs := &smbFS{conn: conn, session: s, share: sh, root: toBackslash(dir)}
	if fs.root != "" {
		if err := sh.MkdirAll(fs.root, 0o755); err != nil {
			fs.Close()
			return nil, fmt.Errorf("smb mkdir %s: %w", dir, err)
		}
	}
	return fs, nil
}

func toBackslash(p string) string {
	return strings.Trim(strings.ReplaceAll(path.Clean("/"+p), "/", `\`), `\`)
}

func (f *smbFS) p(name string) string {
	if f.root == "" {
		return toBackslash(name)
	}
	return f.root + `\` + toBackslash(name)
}

// smbFile serialises writes; a single SMB session is shared by all segments.
type smbFile struct {
	mu sync.Mutex
	f  *smb2.File
}

func (s *smbFile) WriteAt(b []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.WriteAt(b, off)
}
func (s *smbFile) Truncate(n int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Truncate(n)
}
func (s *smbFile) Close() error { return s.f.Close() }

func (f *smbFS) Open(name string) (File, error) {
	p := f.p(name)
	if i := strings.LastIndex(p, `\`); i > 0 {
		_ = f.share.MkdirAll(p[:i], 0o755)
	}
	h, err := f.share.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	return &smbFile{f: h}, nil
}
func (f *smbFS) Rename(o, n string) error { return f.share.Rename(f.p(o), f.p(n)) }
func (f *smbFS) Remove(name string) error { return f.share.Remove(f.p(name)) }
func (f *smbFS) Exists(name string) bool {
	_, err := f.share.Stat(f.p(name))
	return err == nil
}
func (f *smbFS) Local(string) string { return "" }
func (f *smbFS) Upload(src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := f.share.Create(f.p(name))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}
func (f *smbFS) Close() error {
	f.share.Umount()
	f.session.Logoff()
	return f.conn.Close()
}

// TestSMB checks that dir is reachable and writable.
func TestSMB(dir string, settings config.Settings) error {
	fs, err := openSMB(dir, settings)
	if err != nil {
		return err
	}
	defer fs.Close()
	const probe = ".cdm-write-test"
	file, err := fs.Open(probe)
	if err != nil {
		return fmt.Errorf("share is not writable: %w", err)
	}
	file.Close()
	return fs.Remove(probe)
}
