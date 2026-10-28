package integ

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formes d'après le code de Bazarr (bazarr/api/badges.py, system/status.py,
// system/health.py, providers/providers.py).
func TestBazarr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/badges":
			w.Write([]byte(`{"episodes":14,"movies":3,"providers":1,"status":0,"sonarr_signalr":"LIVE","radarr_signalr":"","announcements":0}`))
		case "/api/system/health":
			w.Write([]byte(`{"data":[{"object":"Sonarr","issue":"Connexion impossible"}]}`))
		case "/api/system/status":
			w.Write([]byte(`{"data":{"bazarr_version":"1.5.1","start_time":` + strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10) + `}}`))
		case "/api/providers":
			w.Write([]byte(`{"data":[{"name":"opensubtitlescom","status":"Good","retry":"-"},{"name":"podnapisi","status":"Throttled","retry":"dans 10 min"}]}`))
		default:
			t.Errorf("chemin inattendu %s", r.URL)
		}
	}))
	defer srv.Close()
	src, err := newBazarr(config.Tile{URL: srv.URL}, fakeSecret)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["episodes"].Text != "" && v["version"].Text != "" && v["provider_list"].Head != nil && v["health"].Text != ""
	})
	if v["episodes"].Text != "14" || v["movies"].Text != "3" {
		t.Errorf("manquants = %v %v", v["episodes"], v["movies"])
	}
	if p := v["providers"]; p.Text != "1" || p.Level != "warn" {
		t.Errorf("providers = %+v", p)
	}
	if l := v["links"]; l.Level != "ok" || l.Sub != "Sonarr : LIVE · Radarr : —" {
		t.Errorf("links = %+v", l)
	}
	if h := v["health"]; h.Level != "warn" || h.Sub != "Connexion impossible" {
		t.Errorf("health = %+v", h)
	}
	if x := v["version"]; x.Text != "1.5.1" || x.Sub != "en marche depuis 2 h 0 min" {
		t.Errorf("version = %+v", x)
	}
	if r := v["provider_list"].Rows; len(r) != 2 || r[1][1].Level != "warn" {
		t.Errorf("provider_list = %+v", r)
	}
}
