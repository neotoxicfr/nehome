package integ

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// hits : faux service qui note « MÉTHODE chemin?requête en-tête corps »
// et répond le code donné.
func hits(t *testing.T, header string, code int) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get(header)+" "+string(b)))
		mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }
}

func secrets(m map[string]string) func(string) (string, error) {
	return func(r string) (string, error) { return m[r], nil }
}

func TestCrossSeedJob(t *testing.T) {
	src, _ := newCrossSeed(config.Tile{URL: "http://x"}, nil)
	if len(src.(*crossSeed).Actions()) != 0 {
		t.Fatal("sans clé, aucune action")
	}
	srv, got := hits(t, "X-Api-Key", http.StatusOK)
	src, err := newCrossSeed(config.Tile{URL: srv.URL, Secrets: map[string]string{"key": "s"}}, secrets(map[string]string{"key": "k1"}))
	if err != nil {
		t.Fatal(err)
	}
	c := src.(*crossSeed)
	if _, err := c.Do(context.Background(), "rss", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(context.Background(), "inject", ""); err == nil {
		t.Error("job hors liste accepté")
	}
	if g := got(); len(g) != 1 || g[0] != `POST /api/job k1 {"name":"rss"}` {
		t.Errorf("requêtes %q", g)
	}
}

func TestCrowdSecUnban(t *testing.T) {
	srv, got := hits(t, "Authorization", http.StatusOK)
	c := &crowdsec{url: srv.URL + "/v1", hc: http.DefaultClient, token: "jwt", exp: time.Now().Add(time.Hour), bans: []string{"203.0.113.7"}}
	if _, err := c.Do(context.Background(), "unban", "198.51.100.1"); err == nil {
		t.Error("IP non bannie acceptée")
	}
	if _, err := c.Do(context.Background(), "unban", "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != "DELETE /v1/decisions?ip=203.0.113.7 Bearer jwt" {
		t.Errorf("requêtes %q", g)
	}
}

func TestCleanuparrTrigger(t *testing.T) {
	srv, got := hits(t, "X-Api-Key", http.StatusOK)
	c := &cleanuparr{url: srv.URL, key: "k", hc: http.DefaultClient, jobs: []string{"QueueCleaner"}}
	if _, err := c.Do(context.Background(), "run", "DownloadCleaner"); err == nil {
		t.Error("tâche non planifiée acceptée")
	}
	if _, err := c.Do(context.Background(), "run", "QueueCleaner"); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != "POST /api/jobs/QueueCleaner/trigger k" {
		t.Errorf("requêtes %q", g)
	}
}

func TestJackettTest(t *testing.T) {
	srv, got := hits(t, "", http.StatusOK)
	j := &jackett{url: srv.URL, hc: http.DefaultClient, ids: map[string]string{"TrackerA": "trackera", "TrackerB": "trackerb"}, bad: []string{"TrackerB"}}
	if _, err := j.Do(context.Background(), "test", "Inconnu"); err == nil {
		t.Error("indexeur inconnu accepté")
	}
	if _, err := j.Do(context.Background(), "testbad", ""); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != "POST /api/v2.0/indexers/trackerb/test" {
		t.Errorf("requêtes %q", g)
	}
}

func TestLinkPost(t *testing.T) {
	srv, got := hits(t, "", http.StatusOK)
	src, err := newLink(config.Tile{Options: map[string]string{"post_url": srv.URL + "/wake", "post_label": "Réveiller"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := src.(*link)
	if a := l.Actions(); len(a) != 1 || a[0].Label != "Réveiller" {
		t.Fatalf("actions %+v", a)
	}
	if _, err := l.Do(context.Background(), "post", ""); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != "POST /wake" {
		t.Errorf("requêtes %q", g)
	}
	if _, err := newLink(config.Tile{Options: map[string]string{"post_url": "file:///etc/passwd"}}, nil); err == nil {
		t.Error("post_url non http accepté")
	}
}

func TestGluetunKey(t *testing.T) {
	srv, got := hits(t, "X-API-Key", http.StatusOK)
	src, _ := newGluetun(config.Tile{URL: srv.URL, Secrets: map[string]string{"key": "s"}}, secrets(map[string]string{"key": "gk"}))
	if err := src.(*gluetun).setVPN(context.Background(), "running"); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != `PUT /v1/vpn/status gk {"status":"running"}` {
		t.Errorf("requêtes %q", g)
	}
}

func TestGluetunNoKeyNoAction(t *testing.T) {
	src, _ := newGluetun(config.Tile{URL: "http://x"}, nil)
	if a := src.(*gluetun).Actions(); len(a) != 0 {
		t.Errorf("sans clé : %+v", a)
	}
}

func TestCustomAPIPost(t *testing.T) {
	srv, got := hits(t, "Authorization", http.StatusAccepted)
	tl := config.Tile{URL: srv.URL, Fields: []config.FieldMap{{ID: "v", Label: "V", Path: "v"}},
		Options: map[string]string{"post_url": srv.URL + "/api/updates/check", "post_label": "Vérifier"},
		Secrets: map[string]string{"key": "a", "post_key": "b"}}
	src, err := newCustomAPI(tl, secrets(map[string]string{"key": "lecture", "post_key": "action"}))
	if err != nil {
		t.Fatal(err)
	}
	c := src.(*customAPI)
	if a := c.Actions(); len(a) != 1 || a[0].Label != "Vérifier" {
		t.Fatalf("actions %+v", a)
	}
	if _, err := c.Do(context.Background(), "post", ""); err != nil {
		t.Fatal(err)
	}
	if g := got(); len(g) != 1 || g[0] != "POST /api/updates/check Bearer action" {
		t.Errorf("requêtes %q", g)
	}
}

func TestCustomAPIPostOtherHost(t *testing.T) {
	tl := config.Tile{URL: "http://dockerr:8080/api/status", Fields: []config.FieldMap{{ID: "v", Label: "V", Path: "v"}},
		Options: map[string]string{"post_url": "http://evil.example:8080/x"}, Secrets: map[string]string{"post_key": "b"}}
	if _, err := newCustomAPI(tl, secrets(map[string]string{"post_key": "action"})); err == nil {
		t.Error("post_url vers un autre hôte accepté")
	}
}
