package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestLinkContainerEtSonde(t *testing.T) {
	dk := dkFake(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.Write([]byte(`{"status":"ok"}`))
		case "/ailleurs": // redirection vers un autre hôte : à ne pas suivre
			http.Redirect(w, r, "http://127.0.0.2:1/login", http.StatusFound)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer app.Close()
	mk := func(opts map[string]string) *link {
		src, err := newLink(config.Tile{Options: opts}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return src.(*link)
	}
	l := mk(map[string]string{"container": "nehome", "docker_url": dk.URL, "health_url": app.URL + "/api/health"})
	v, err := l.poll(t.Context())
	if err != nil || v["state"].Text != "sain" || v["state"].Level != "ok" || v["health"].Text != "HTTP 200" || v["image"].Text != "nehome:local" || v["uptime"].Text == "" {
		t.Fatalf("ok : %+v %v", v, err)
	}
	if v, _ = mk(map[string]string{"health_url": app.URL + "/ailleurs"}).poll(t.Context()); v["health"].Text != "HTTP 302" || v["health"].Level != "ok" {
		t.Fatalf("redirection : %+v", v["health"])
	}
	if v, _ = mk(map[string]string{"health_url": app.URL + "/panne"}).poll(t.Context()); v["health"].Level != "crit" {
		t.Fatalf("503 : %+v", v["health"])
	}
	dead := httptest.NewServer(nil)
	dead.Close()
	if v, _ = mk(map[string]string{"health_url": dead.URL}).poll(t.Context()); v["health"].Text != "injoignable" || v["health"].Level != "crit" {
		t.Fatalf("injoignable : %+v", v["health"])
	}
	if v, _ = mk(map[string]string{"container": "absent", "docker_url": dk.URL}).poll(t.Context()); v["state"].Text != "introuvable" || v["state"].Level != "crit" {
		t.Fatalf("absent : %+v", v["state"])
	}
	if _, err = mk(map[string]string{"container": "x", "docker_url": dead.URL}).poll(t.Context()); err == nil {
		t.Fatal("Docker injoignable doit remonter une erreur")
	}
	if _, err = newLink(config.Tile{Options: map[string]string{"health_url": "ftp://x"}}, nil); err == nil {
		t.Fatal("schéma refusé attendu")
	}
}
