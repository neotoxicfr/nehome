package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

type actor struct {
	fake
	done *atomic.Value
}

func (actor) Actions() []tile.Action {
	return []tile.Action{{ID: "restart", Label: "Redémarrer", Confirm: "Sûr ?", Choices: []string{"plex", "sonarr"}}}
}
func (a actor) Do(_ context.Context, id, arg string) (string, error) {
	a.done.Store(id + ":" + arg)
	return arg + " redémarré", nil
}

func TestActions(t *testing.T) {
	st, _ := settings.Open(filepath.Join(t.TempDir(), "s.json"))
	done := &atomic.Value{}
	src := actor{done: done}
	b := board.New([]*board.Tile{{Cfg: config.Tile{ID: "dk", Title: "Docker"}, Src: src, Fields: src.Fields()}}, st)
	s := New(&config.Config{Auth: config.Auth{Header: "Remote-User", TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Admins: []string{"admin"}}}, b, st)
	b.Start(t.Context())
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	call := func(user, site string, form url.Values) (int, map[string]string) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/action/dk/restart", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Remote-User", user)
		req.Header.Set("Sec-Fetch-Site", site)
		req.Header.Set("X-Requested-With", "fetch")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		m := map[string]string{}
		json.NewDecoder(r.Body).Decode(&m)
		return r.StatusCode, m
	}
	if c, _ := call("invite", "same-origin", url.Values{"arg": {"plex"}, "confirmed": {"1"}}); c != http.StatusForbidden {
		t.Fatalf("non admin : %d", c)
	}
	if c, _ := call("admin", "cross-site", url.Values{"arg": {"plex"}, "confirmed": {"1"}}); c != http.StatusForbidden {
		t.Fatalf("autre site : %d", c)
	}
	if c, m := call("admin", "same-origin", url.Values{"arg": {"plex"}}); c != http.StatusBadRequest || !strings.Contains(m["error"], "confirmation") {
		t.Fatalf("sans confirmation : %d %v", c, m)
	}
	if c, _ := call("admin", "same-origin", url.Values{"arg": {"traefik"}, "confirmed": {"1"}}); c != http.StatusBadRequest {
		t.Fatalf("élément non proposé : %d", c)
	}
	if done.Load() != nil {
		t.Fatal("action exécutée malgré un refus")
	}
	if c, m := call("admin", "same-origin", url.Values{"arg": {"plex"}, "confirmed": {"1"}}); c != http.StatusOK || m["message"] != "plex redémarré" || done.Load() != "restart:plex" {
		t.Fatalf("action : %d %v %v", c, m, done.Load())
	}
	_, page := get(t, srv.URL+"/", http.Header{"Remote-User": {"admin"}})
	if !strings.Contains(page, `action="/action/dk/restart"`) {
		t.Fatal("bouton absent du détail")
	}
}

func TestFlashNotReflected(t *testing.T) {
	srv, _ := setup(t, trusted)
	_, page := get(t, srv.URL+"/?msg=Votre+session+a+expir%C3%A9", http.Header{"Remote-User": {"neo"}})
	if strings.Contains(page, "session a expiré") {
		t.Fatal("texte de l'URL affiché")
	}
}
