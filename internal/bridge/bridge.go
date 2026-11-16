// Package bridge exposes a small HTTP API on 127.0.0.1 for the browser
// extension. Security model:
//
//   - it listens on loopback only;
//   - every request must carry X-CDM-Token (shown in Options → Browser
//     Integration and pasted once into the extension). A web page cannot read
//     the token, and a custom header forces a CORS preflight that is only
//     answered for extension origins.
//   - requests from http(s):// origins are rejected outright.
package bridge

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type Item struct {
	URL       string            `json:"url"`
	FileName  string            `json:"fileName"`
	Referer   string            `json:"referer"`
	Cookies   string            `json:"cookies"`
	UserAgent string            `json:"userAgent"`
	Headers   map[string]string `json:"headers"`
	Kind      string            `json:"kind"` // "", "file", "stream", "media"
	Size      int64             `json:"size"`
	Mime      string            `json:"mime"`
}

type AddRequest struct {
	Items  []Item `json:"items"`
	Batch  bool   `json:"batch"`  // "Download all links" – show the selection dialog
	Silent bool   `json:"silent"` // add without dialog
	Page   string `json:"page"`   // title of the page, used as description
}

type Config struct {
	Enabled bool     `json:"enabled"`
	Types   []string `json:"types"`
	Skip    []string `json:"skip"`
	Version string   `json:"version"`
}

type Handler interface {
	BridgeAdd(req AddRequest) error
	BridgeConfig() Config
	Token() string
}

type Server struct {
	h   Handler
	srv *http.Server
}

func Start(port int, h Handler) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("browser bridge: %w", err)
	}
	s := &Server{h: h}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", s.wrap(s.ping))
	mux.HandleFunc("/api/config", s.wrap(s.config))
	mux.HandleFunc("/api/add", s.wrap(s.add))
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *Server) Close() {
	if s == nil || s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

func extensionOrigin(o string) bool {
	return strings.HasPrefix(o, "chrome-extension://") ||
		strings.HasPrefix(o, "moz-extension://") ||
		strings.HasPrefix(o, "safari-web-extension://") ||
		strings.HasPrefix(o, "extension://") // Edge / Opera variants
}

func (s *Server) wrap(fn func(w http.ResponseWriter, r *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !extensionOrigin(origin) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CDM-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		tok := r.Header.Get("X-CDM-Token")
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.h.Token())) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "bad token: copy it from Options → Browser"})
			return
		}
		out, err := fn(w, r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) (any, error) {
	return map[string]string{"app": "CrazyDownloadManager", "status": "ok"}, nil
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.h.BridgeConfig(), nil
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method != http.MethodPost {
		return nil, errors.New("POST required")
	}
	var req AddRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errors.New("no items")
	}
	if err := s.h.BridgeAdd(req); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": len(req.Items)}, nil
}
