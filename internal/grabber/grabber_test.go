package grabber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestScan(t *testing.T) {
	mux := http.NewServeMux()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/", page(`<a href="/docs/">docs</a><img src="logo.png"><a href="https://elsewhere.example/x.pdf">x</a>`))
	mux.HandleFunc("/docs/", page(`<a href="a.pdf">a</a><a href="deep/">deep</a><img srcset="s1.jpg 1x, s2.jpg 2x">`))
	mux.HandleFunc("/docs/deep/", page(`<a href="b.pdf">b</a>`))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	items, err := Scan(context.Background(), srv.Client(), Options{
		StartURL: srv.URL + "/", Mode: ModeFiles, Depth: 1, SameHost: true, Include: []string{"*.pdf", "*.jpg"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.SaveAs)
	}
	sort.Strings(names)
	// depth 1: start page + /docs/; b.pdf lives at depth 2. x.pdf is off-host
	// but still a link *on* an in-scope page, so it is offered.
	want := []string{"a.pdf", "s1.jpg", "s2.jpg", "x.pdf"}
	if len(names) != len(want) {
		t.Fatalf("got %v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v want %v", names, want)
		}
	}

	items, _ = Scan(context.Background(), srv.Client(), Options{StartURL: srv.URL + "/", Mode: ModeMirror, Depth: 5, SameHost: true}, nil)
	found := map[string]bool{}
	for _, it := range items {
		found[it.SaveAs] = true
	}
	host := srv.Listener.Addr().String()
	for _, p := range []string{"index.html", "docs/index.html", "docs/deep/index.html", "logo.png", "docs/a.pdf", "docs/deep/b.pdf"} {
		if !found[sanitize(host)+"/"+p] {
			t.Errorf("mirror missing %s (have %v)", p, found)
		}
	}
}
