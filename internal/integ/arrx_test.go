package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formes de réponses d'après le code source de Lidarr, Readarr et Prowlarr
// (src/Lidarr.Api.V1, Readarr.Api.V1, Prowlarr.Api.V1 : QueueResource,
// HistoryResource, CalendarResource, IndexerStatsResource).
func TestLidarr(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/") {
		case "queue":
			w.Write([]byte(`{"totalRecords":1,"records":[{"title":"x","status":"downloading","trackedDownloadStatus":"ok","trackedDownloadState":"downloading","size":200,"sizeleft":50,"downloadClient":"qB","artist":{"artistName":"Daft Punk"},"album":{"title":"Discovery"}}]}`))
		case "wanted/missing":
			w.Write([]byte(`{"totalRecords":12,"records":[]}`))
		case "history":
			if r.URL.Query().Get("eventType") != "3" {
				t.Errorf("eventType = %q", r.URL.Query().Get("eventType"))
			}
			w.Write([]byte(`{"totalRecords":1,"records":[{"sourceTitle":"s","date":"` + now.Add(-time.Hour).Format(time.RFC3339) + `","quality":{"quality":{"name":"FLAC"}},"artist":{"artistName":"Air"},"album":{"title":"Moon Safari"}}]}`))
		case "calendar":
			w.Write([]byte(`[{"title":"Futur","releaseDate":"` + now.Add(24*time.Hour).Format(time.RFC3339) + `","artist":{"artistName":"A"},"statistics":{"percentOfTracks":0}},{"title":"Passé","releaseDate":"` + now.Add(-2*time.Hour).Format(time.RFC3339) + `","artist":{"artistName":"B"},"statistics":{"percentOfTracks":100}}]`))
		case "artist":
			w.Write([]byte(`[{"monitored":true,"statistics":{"sizeOnDisk":1000000000,"trackFileCount":120}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	src, err := newLidarr(config.Tile{URL: srv.URL}, fakeSecret)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["calendar"].Head != nil && v["library"].Text != "" && v["history"].Head != nil
	})
	if r := v["downloads"].Rows; len(r) != 1 || r[0][0].Text != "Daft Punk – Discovery" || r[0][1].Text != "75 %" {
		t.Errorf("downloads = %+v", r)
	}
	if v["missing"].Text != "12" {
		t.Errorf("missing = %+v", v["missing"])
	}
	if h := v["history"].Rows; len(h) != 1 || h[0][0].Text != "Air – Moon Safari" || h[0][1].Text != "FLAC" {
		t.Errorf("history = %+v", h)
	}
	if c := v["calendar"]; c.Head[1] != "Artiste" || len(c.Rows) != 2 || c.Rows[0][3].Text != "Téléchargé" || c.Rows[1][3].Text != "Attendu" {
		t.Errorf("calendar = %+v", c)
	}
	if l := v["library"]; l.Text != "1" || !strings.Contains(l.Sub, "120 pistes") {
		t.Errorf("library = %+v", l)
	}
	if f := (&arr{lidarr: true}).Fields(); f[3].Label != "Artistes" {
		t.Errorf("libellé = %q", f[3].Label)
	}
}

func TestReadarr(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/") {
		case "queue":
			w.Write([]byte(`{"totalRecords":1,"records":[{"title":"x","size":100,"sizeleft":0,"trackedDownloadStatus":"warning","trackedDownloadState":"importPending","author":{"authorName":"Hugo"},"book":{"title":"Les Misérables"}}]}`))
		case "calendar":
			w.Write([]byte(`[{"title":"Livre","releaseDate":"` + now.Add(48*time.Hour).Format(time.RFC3339) + `","author":{"authorName":"Zola"},"statistics":{"bookFileCount":0}}]`))
		case "author":
			w.Write([]byte(`[{"monitored":true,"statistics":{"sizeOnDisk":5000000,"bookFileCount":7}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	src, _ := newReadarr(config.Tile{URL: srv.URL}, fakeSecret)
	v := collect(t, src, func(v tile.Values) bool {
		return v["calendar"].Head != nil && v["library"].Text != "" && v["downloads"].Head != nil
	})
	if r := v["downloads"].Rows; len(r) != 1 || r[0][0].Text != "Hugo – Les Misérables" || r[0][3].Text != "Import en attente" || r[0][3].Level != "warn" {
		t.Errorf("downloads = %+v", r)
	}
	if c := v["calendar"]; c.Head[2] != "Livre" || len(c.Rows) != 1 || c.Rows[0][1].Text != "Zola" {
		t.Errorf("calendar = %+v", c)
	}
	if l := v["library"]; !strings.Contains(l.Sub, "7 livres") {
		t.Errorf("library = %+v", l)
	}
}

func TestProwlarr(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/") {
		case "indexer":
			w.Write([]byte(`[{"id":1,"name":"Alpha","enable":true},{"id":2,"name":"Beta","enable":true},{"id":3,"name":"Off","enable":false}]`))
		case "indexerstatus":
			w.Write([]byte(`[{"indexerId":2,"disabledTill":"` + now.Add(time.Hour).Format(time.RFC3339) + `"},{"indexerId":1,"disabledTill":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}]`))
		case "health":
			w.Write([]byte(`[{"type":"error","message":"Indexeur HS"}]`))
		case "indexerstats":
			if r.URL.Query().Get("startDate") == "" {
				t.Error("startDate manquant")
			}
			w.Write([]byte(`{"indexers":[{"indexerName":"Alpha","averageResponseTime":420,"numberOfQueries":100,"numberOfGrabs":5,"numberOfFailedQueries":2,"numberOfFailedGrabs":0},{"indexerName":"Beta","averageResponseTime":900,"numberOfQueries":50,"numberOfGrabs":1,"numberOfFailedQueries":0,"numberOfFailedGrabs":0}]}`))
		case "system/status":
			w.Write([]byte(`{"version":"1.30.2","startTime":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}`))
		default:
			t.Errorf("chemin inattendu %s", r.URL)
		}
	}))
	defer srv.Close()
	src, err := newProwlarr(config.Tile{URL: srv.URL}, fakeSecret)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["version"].Text != "" && v["indexers"].Text != "" && v["health"].Text != ""
	})
	if i := v["indexers"]; i.Text != "2" || i.Level != "warn" || !strings.Contains(i.Sub, "1 indisponible") {
		t.Errorf("indexers = %+v", i)
	}
	if d := v["down"].Rows; len(d) != 1 || d[0][0].Text != "Beta" {
		t.Errorf("down = %+v", d)
	}
	if h := v["health"]; h.Level != "crit" || h.Sub != "Indexeur HS" {
		t.Errorf("health = %+v", h)
	}
	if v["queries"].Text != "150" || v["grabs"].Text != "6" || v["failed"].Text != "2" {
		t.Errorf("stats = %v %v %v", v["queries"], v["grabs"], v["failed"])
	}
	if s := v["stats"].Rows; len(s) != 2 || s[0][4].Text != "420 ms" || s[0][3].Level != "warn" {
		t.Errorf("stats table = %+v", s)
	}
}
