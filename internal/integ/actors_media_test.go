package integ

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// hit : une requête reçue par le faux service.
type hit struct {
	method, path, body string
	header             http.Header
}

// recorder note chaque requête ; reply choisit le code et le corps renvoyés.
type recorder struct {
	mu   sync.Mutex
	hits []hit
}

func (rc *recorder) server(t *testing.T, reply func(r *http.Request) (int, string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.hits = append(rc.hits, hit{r.Method, r.URL.Path, string(b), r.Header.Clone()})
		rc.mu.Unlock()
		code, body := http.StatusOK, ""
		if reply != nil {
			code, body = reply(r)
		}
		w.WriteHeader(code)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (rc *recorder) last(t *testing.T) hit {
	t.Helper()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.hits) == 0 {
		t.Fatal("aucune requête reçue")
	}
	return rc.hits[len(rc.hits)-1]
}

func actionIDs(a tile.Actor) []string {
	var ids []string
	for _, x := range a.Actions() {
		ids = append(ids, x.ID)
	}
	return ids
}

func TestArrActions(t *testing.T) {
	cases := []struct {
		mk      func(config.Tile, func(string) (string, error)) (tile.Source, error)
		api     string
		missing string
	}{
		{newSonarr, "/api/v3", "MissingEpisodeSearch"},
		{newRadarr, "/api/v3", "MissingMoviesSearch"},
		{newLidarr, "/api/v1", "MissingAlbumSearch"},
		{newReadarr, "/api/v1", "MissingBookSearch"},
	}
	for _, c := range cases {
		var rc recorder
		srv := rc.server(t, func(*http.Request) (int, string) { return http.StatusCreated, `{"id":1}` })
		src, _ := c.mk(config.Tile{URL: srv.URL}, fakeSecret)
		a := src.(tile.Actor)
		if ids := actionIDs(a); !slices.Equal(ids, []string{"rss", "refresh", "missing"}) || a.Actions()[2].Confirm == "" {
			t.Fatalf("actions : %v", a.Actions())
		}
		for id, name := range map[string]string{"rss": "RssSync", "refresh": "RefreshMonitoredDownloads", "missing": c.missing} {
			msg, err := a.Do(t.Context(), id, "")
			h := rc.last(t)
			if err != nil || msg == "" || h.method != http.MethodPost || h.path != c.api+"/command" || h.body != `{"name":"`+name+`"}` ||
				h.header.Get("X-Api-Key") != "factice" || h.header.Get("Content-Type") != "application/json" {
				t.Fatalf("%s %s : %q %v %+v", c.api, id, msg, err, h)
			}
		}
		if _, err := a.Do(t.Context(), "nope", ""); err == nil {
			t.Fatal("action inconnue acceptée")
		}
	}
	// Erreur HTTP : ni clé ni chaîne de requête dans le message.
	var rc recorder
	srv := rc.server(t, func(*http.Request) (int, string) { return http.StatusUnauthorized, "" })
	src, _ := newSonarr(config.Tile{URL: srv.URL}, fakeSecret)
	if _, err := src.(tile.Actor).Do(t.Context(), "rss", ""); err == nil || strings.Contains(err.Error(), "factice") {
		t.Fatalf("erreur : %v", err)
	}
	if _, ok := any(&prowlarr{}).(tile.Actor); ok {
		t.Fatal("prowlarr ne doit pas proposer d'action")
	}
}

func TestQbitAltSpeed(t *testing.T) {
	var rc recorder
	var logged atomic.Bool
	srv := rc.server(t, func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			logged.Store(true)
			return http.StatusOK, "Ok."
		case "/api/v2/transfer/toggleSpeedLimitsMode":
			if !logged.Load() {
				return http.StatusForbidden, "Forbidden"
			}
			return http.StatusOK, ""
		}
		return http.StatusNotFound, ""
	})
	src, _ := newQbit(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "x", "pass": "x"}}, fakeSecret)
	q := src.(*qbit)
	q.user = "admin" // fakeSecret ne connaît pas « user »
	if ids := actionIDs(q); !slices.Equal(ids, []string{"altspeed"}) {
		t.Fatalf("actions : %v", ids)
	}
	msg, err := q.Do(t.Context(), "altspeed", "")
	if err != nil || msg == "" {
		t.Fatalf("Do : %q %v", msg, err)
	}
	rc.mu.Lock()
	var seq []string
	for _, h := range rc.hits {
		seq = append(seq, h.method+" "+h.path)
	}
	login := rc.hits[1]
	rc.mu.Unlock()
	want := []string{"POST /api/v2/transfer/toggleSpeedLimitsMode", "POST /api/v2/auth/login", "POST /api/v2/transfer/toggleSpeedLimitsMode"}
	if !slices.Equal(seq, want) || !strings.Contains(login.body, "username=admin") {
		t.Fatalf("séquence : %v / %q", seq, login.body)
	}

	// Mauvais mot de passe : bannissement local, pas de second login.
	var rc2 recorder
	srv2 := rc2.server(t, func(r *http.Request) (int, string) {
		if r.URL.Path == "/api/v2/auth/login" {
			return http.StatusOK, "Fails."
		}
		return http.StatusForbidden, ""
	})
	src2, _ := newQbit(config.Tile{URL: srv2.URL, Secrets: map[string]string{"user": "x", "pass": "x"}}, fakeSecret)
	q2 := src2.(*qbit)
	q2.user = "admin"
	for range 2 {
		if _, err := q2.Do(t.Context(), "altspeed", ""); err == nil {
			t.Fatal("refus attendu")
		}
	}
	rc2.mu.Lock()
	n := 0
	for _, h := range rc2.hits {
		if h.path == "/api/v2/auth/login" {
			n++
		}
	}
	rc2.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d logins, 1 attendu", n)
	}
}

func TestAdGuardActions(t *testing.T) {
	var rc recorder
	srv := rc.server(t, nil)
	src, _ := newAdGuard(config.Tile{URL: srv.URL}, mediaSecret)
	a := src.(tile.Actor)
	if ids := actionIDs(a); !slices.Equal(ids, []string{"pause10", "pause60", "resume"}) {
		t.Fatalf("actions : %v", ids)
	}
	for id, body := range map[string]string{
		"pause10": `{"duration":600000,"enabled":false}`,
		"pause60": `{"duration":3600000,"enabled":false}`,
		"resume":  `{"enabled":true}`,
	} {
		msg, err := a.Do(t.Context(), id, "")
		h := rc.last(t)
		u, p, ok := (&http.Request{Header: h.header}).BasicAuth()
		if err != nil || msg == "" || h.method != http.MethodPost || h.path != "/control/protection" || h.body != body || !ok || u != "factice" || p != "factice" {
			t.Fatalf("%s : %q %v %+v", id, msg, err, h)
		}
	}
	var rc2 recorder
	srv2 := rc2.server(t, func(*http.Request) (int, string) { return http.StatusNotFound, "" })
	src2, _ := newAdGuard(config.Tile{URL: srv2.URL}, mediaSecret)
	if _, err := src2.(tile.Actor).Do(t.Context(), "resume", ""); err == nil || !strings.Contains(err.Error(), "trop ancien") {
		t.Fatalf("ancienne version : %v", err)
	}
}

func TestPlexScan(t *testing.T) {
	var rc recorder
	srv := rc.server(t, func(r *http.Request) (int, string) {
		if r.URL.Path == "/library/sections" {
			return http.StatusOK, `{"MediaContainer":{"Directory":[{"key":"1"},{"key":"7"}]}}`
		}
		return http.StatusOK, ""
	})
	src, _ := newPlex(config.Tile{URL: srv.URL}, mediaSecret)
	a := src.(tile.Actor)
	msg, err := a.Do(t.Context(), "scan", "")
	if err != nil || !strings.Contains(msg, "2 bibliothèque") {
		t.Fatalf("Do : %q %v", msg, err)
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var seq []string
	for _, h := range rc.hits {
		if h.header.Get("X-Plex-Token") != "factice" {
			t.Fatalf("jeton absent : %+v", h)
		}
		seq = append(seq, h.method+" "+h.path)
	}
	want := []string{"GET /library/sections", "GET /library/sections/1/refresh", "GET /library/sections/7/refresh"}
	if !slices.Equal(seq, want) {
		t.Fatalf("séquence : %v", seq)
	}
}

func TestMaintainerrRules(t *testing.T) {
	var rc recorder
	var code atomic.Int32
	code.Store(http.StatusCreated)
	srv := rc.server(t, func(*http.Request) (int, string) { return int(code.Load()), "" })
	src, _ := newMaintainerr(config.Tile{URL: srv.URL}, nil)
	a := src.(tile.Actor)
	msg, err := a.Do(t.Context(), "rules", "")
	h := rc.last(t)
	if err != nil || msg == "" || h.method != http.MethodPost || h.path != "/api/rules/execute" || h.body != "" {
		t.Fatalf("Do : %q %v %+v", msg, err, h)
	}
	code.Store(http.StatusConflict)
	if _, err := a.Do(t.Context(), "rules", ""); err == nil || !strings.Contains(err.Error(), "déjà") {
		t.Fatalf("409 : %v", err)
	}
}

func TestSeerrActions(t *testing.T) {
	var rc recorder
	srv := rc.server(t, func(r *http.Request) (int, string) {
		switch {
		case r.URL.Path == "/api/v1/request/count":
			return http.StatusOK, `{"pending":1}`
		case r.URL.Path == "/api/v1/request" && r.URL.Query().Get("filter") == "pending":
			return http.StatusOK, `{"results":[{"id":42,"status":1,"type":"movie","media":{"tmdbId":603}}]}`
		case r.URL.Path == "/api/v1/movie/603":
			return http.StatusOK, `{"title":"Matrix"}`
		}
		return http.StatusOK, `{}`
	})
	src, _ := newSeerr(config.Tile{URL: srv.URL}, mediaSecret)
	s := src.(*seerr)
	if len(s.Actions()) != 0 {
		t.Fatal("actions sans demande en attente")
	}
	if _, err := s.poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	acts := s.Actions()
	if len(acts) != 2 || acts[0].ID != "approve" || acts[1].ID != "decline" || acts[1].Confirm == "" || !slices.Equal(acts[0].Choices, []string{"#42 Matrix"}) {
		t.Fatalf("actions : %+v", acts)
	}
	for _, id := range []string{"approve", "decline"} {
		msg, err := s.Do(t.Context(), id, "#42 Matrix")
		h := rc.last(t)
		if err != nil || msg == "" || h.method != http.MethodPost || h.path != "/api/v1/request/42/"+id || h.header.Get("X-Api-Key") != "factice" {
			t.Fatalf("%s : %q %v %+v", id, msg, err, h)
		}
	}
	for _, bad := range []string{"", "#abc", "Matrix", "#-1 x"} {
		if _, err := s.Do(t.Context(), "approve", bad); err == nil {
			t.Fatalf("argument %q accepté", bad)
		}
	}
}
