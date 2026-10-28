package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

type fake struct{}

func (fake) Fields() []tile.Field {
	return []tile.Field{tile.F("a", "Champ A", tile.Stat, tile.OnTile), tile.F("b", "Champ B", tile.Stat, tile.InDetail)}
}
func (fake) Run(ctx context.Context, emit func(tile.Values, error)) {
	emit(tile.Values{"a": {Text: "valeur-a"}, "b": {Text: "valeur-b"}}, nil)
}

func setup(t *testing.T, auth config.Auth) (*httptest.Server, *settings.Store) {
	t.Helper()
	st, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Auth: auth}
	tc := config.Tile{ID: "x", Title: "X", Section: "S"}
	b := board.New([]*board.Tile{{Cfg: tc, Src: fake{}, Fields: fake{}.Fields()}}, st)
	s := New(cfg, b, st)
	b.Start(t.Context())
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

var trusted = config.Auth{Header: "Remote-User", TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}

func get(t *testing.T, u string, h http.Header) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	for k, v := range h {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 1<<16)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp, sb.String()
}

func TestAuth(t *testing.T) {
	srv, _ := setup(t, trusted)
	if r, _ := get(t, srv.URL+"/", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sans en-tête : %d, attendu 401", r.StatusCode)
	}
	r, body := get(t, srv.URL+"/", http.Header{"Remote-User": {"neo"}})
	if r.StatusCode != http.StatusOK || !strings.Contains(body, "valeur-a") {
		t.Fatalf("avec en-tête : %d", r.StatusCode)
	}
	if csp := r.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("CSP absente : %q", csp)
	}
	if r, _ := get(t, srv.URL+"/healthz", nil); r.StatusCode != http.StatusOK {
		t.Fatalf("healthz : %d", r.StatusCode)
	}

	// L'en-tête n'est pas cru quand la connexion ne vient pas d'un proxy déclaré.
	other, _ := setup(t, config.Auth{Header: "Remote-User", TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}})
	if r, _ := get(t, other.URL+"/", http.Header{"Remote-User": {"neo"}}); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("proxy non déclaré : %d, attendu 401", r.StatusCode)
	}
}

func post(t *testing.T, u string, form url.Values, h http.Header) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range h {
		req.Header[k] = v
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestSaveSettings(t *testing.T) {
	srv, st := setup(t, trusted)
	form := url.Values{"visible": {"1"}, "f.a": {"detail"}, "f.b": {"tile"}, "o.a": {"20"}, "o.b": {"10"}}

	if r := post(t, srv.URL+"/settings/x", form, http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"cross-site"}}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("POST d'un autre site : %d, attendu 403", r.StatusCode)
	}
	if r := post(t, srv.URL+"/settings/x", form, http.Header{"Remote-User": {"neo"}}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("POST sans origine : %d, attendu 403", r.StatusCode)
	}
	r := post(t, srv.URL+"/settings/x", form, http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}})
	if r.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST légitime : %d, attendu 303", r.StatusCode)
	}
	got := st.Get("x")
	if got.Hidden || got.Fields["a"] != tile.InDetail || got.Fields["b"] != tile.OnTile || got.Order[0] != "b" {
		t.Fatalf("réglages enregistrés : %+v", got)
	}
	_, body := get(t, srv.URL+"/", http.Header{"Remote-User": {"neo"}})
	tileHTML, _, _ := strings.Cut(body, "<template")
	if !strings.Contains(tileHTML, "valeur-b") || strings.Contains(tileHTML, "valeur-a") {
		t.Fatal("la tuile ne reflète pas le nouveau placement")
	}

	// Case décochée : tuile masquée de l'accueil.
	post(t, srv.URL+"/settings/x", url.Values{}, http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}})
	if _, body := get(t, srv.URL+"/", http.Header{"Remote-User": {"neo"}}); strings.Contains(body, `id="t-x"`) {
		t.Fatal("tuile masquée encore affichée")
	}
}

func TestSpark(t *testing.T) {
	if spark([]float64{1}) != "" {
		t.Fatal("un seul point : pas de courbe")
	}
	if s := string(spark([]float64{0, 5, 10})); !strings.Contains(s, "<polyline points=\"0.0,27.0 60.0,14.0 120.0,1.0\"") {
		t.Fatalf("courbe : %s", s)
	}
}
