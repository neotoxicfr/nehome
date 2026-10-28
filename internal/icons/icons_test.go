package icons

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServe(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)
	hits := 0
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/png/sonarr.png":
			hits++
			w.Write(png)
		case "/png/faux.png":
			w.Write([]byte("<svg onload=alert(1)>"))
		}
	}))
	defer cdn.Close()
	s := New(t.TempDir())
	s.base = cdn.URL
	s.load([]byte(`{"png":["sonarr.png","faux.png","truenas-scale.png"]}`))

	if g := s.Guess("", "truenas", "", "nas", "NAS"); g != "truenas-scale" {
		t.Fatalf("devinette par type : %q", g)
	}
	if g := s.Guess("", "link", "sonarr", "x", "X"); g != "sonarr" {
		t.Fatalf("devinette par conteneur : %q", g)
	}
	if g := s.Guess("", "link", "", "pilot", "Pilot"); g != "" {
		t.Fatalf("icône inventée : %q", g)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /icon/{file}", s)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for i := 0; i < 2; i++ {
		r, _ := http.Get(srv.URL + "/icon/sonarr.png")
		if r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("icône : %d %s", r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	if hits != 1 {
		t.Fatalf("icône téléchargée %d fois, attendu 1 (cache)", hits)
	}
	for _, p := range []string{"/icon/faux.png", "/icon/inconnue.png", "/icon/..%2Fx.png", "/icon/sonarr.svg"} {
		if r, _ := http.Get(srv.URL + p); r.StatusCode == 200 {
			t.Fatalf("%s servi", p)
		}
	}
}

func TestLocalIcon(t *testing.T) {
	s := New(t.TempDir())
	if s.Has("pilot") {
		t.Fatal("icône absente vue")
	}
	os.WriteFile(filepath.Join(s.dir, "pilot.png"), []byte("\x89PNG\r\n\x1a\n"), 0o644)
	if g := s.Guess("", "link", "pilot", "pilot", "Pilot"); g != "pilot" {
		t.Fatalf("icône déposée à la main ignorée : %q", g)
	}
}
