package integ

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

func fakeSecret(role string) (string, error) {
	return map[string]string{"key": "factice", "pass": "factice"}[role], nil
}

// collect lance Run jusqu'à ce que check accepte les valeurs cumulées.
func collect(t *testing.T, src tile.Source, check func(tile.Values) bool) tile.Values {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex // emit est appelé depuis deux goroutines
	got := tile.Values{}
	done := make(chan struct{})
	go src.Run(ctx, func(v tile.Values, err error) {
		if err != nil {
			t.Errorf("emit: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		for k, x := range v {
			got[k] = x
		}
		if check(got) {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("valeurs incomplètes : %v", got)
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	return maps.Clone(got)
}

func TestSonarr(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/v3/") {
		case "queue":
			w.Write([]byte(`{"page":1,"pageSize":50,"totalRecords":2,"records":[
{"title":"Show.S01E02.1080p","status":"downloading","trackedDownloadStatus":"ok","trackedDownloadState":"downloading","size":1000,"sizeleft":250,"downloadClient":"qBittorrent","series":{"title":"Show"},"episode":{"seasonNumber":1,"episodeNumber":2}},
{"title":"Show.S01E03.1080p","status":"completed","trackedDownloadStatus":"warning","trackedDownloadState":"importBlocked","size":1000,"sizeleft":0,"downloadClient":"qBittorrent","series":{"title":"Show"},"episode":{"seasonNumber":1,"episodeNumber":3},"statusMessages":[{"messages":["Pas de fichier vidéo"]}]}]}`))
		case "wanted/missing":
			w.Write([]byte(`{"page":1,"pageSize":1,"totalRecords":895,"records":[{"seriesId":2049}]}`))
		case "wanted/cutoff":
			w.Write([]byte(`{"page":1,"pageSize":1,"totalRecords":8725,"records":[{"seriesId":2043}]}`))
		case "health":
			w.Write([]byte(`[{"source":"IndexerStatusCheck","type":"warning","message":"Indexeurs indisponibles"}]`))
		case "diskspace":
			w.Write([]byte(`[{"path":"/","label":"","freeSpace":842080321536,"totalSpace":867922870272},{"path":"/config","label":"","freeSpace":842080321536,"totalSpace":867922870272},{"path":"/downloads","label":"","freeSpace":2264326471680,"totalSpace":16417903607808}]`))
		case "history":
			w.Write([]byte(`{"totalRecords":1,"records":[{"sourceTitle":"Death.in.Paradise.S01E08.FRENCH.1080p","quality":{"quality":{"name":"WEBDL-1080p"}},"date":"` + now.Add(-time.Hour).Format(time.RFC3339) + `","eventType":"downloadFolderImported","series":{"title":"Death in Paradise"},"episode":{"seasonNumber":1,"episodeNumber":8}}]}`))
		case "calendar":
			w.Write([]byte(`[{"seasonNumber":1,"episodeNumber":4,"airDateUtc":"` + now.Add(24*time.Hour).Format(time.RFC3339) + `","hasFile":false,"series":{"title":"Love in Disguise"}},{"seasonNumber":1,"episodeNumber":3,"airDateUtc":"` + now.Add(-2*time.Hour).Format(time.RFC3339) + `","hasFile":false,"series":{"title":"Autre"}}]`))
		case "series":
			w.Write([]byte(`[{"title":"A","monitored":true,"statistics":{"episodeFileCount":39,"sizeOnDisk":78737948638}},{"title":"B","monitored":false,"statistics":{"episodeFileCount":1,"sizeOnDisk":1000000000}}]`))
		case "system/status":
			w.Write([]byte(`{"appName":"Sonarr","version":"4.0.20.3014","startTime":"` + now.Add(-26*time.Hour).Format(time.RFC3339) + `"}`))
		case "system/task":
			w.Write([]byte(`[{"name":"Rss Sync","lastExecution":"` + now.Add(-10*time.Minute).Format(time.RFC3339) + `","nextExecution":"` + now.Add(time.Hour).Format(time.RFC3339) + `"}]`))
		default:
			t.Errorf("chemin inattendu %s", r.URL)
		}
	}))
	defer srv.Close()
	src, err := newSonarr(config.Tile{URL: srv.URL}, fakeSecret)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool { return v["tasks"].Head != nil && v["queue"].Text != "" })
	if q := v["queue"]; q.Text != "2" || q.Level != "warn" || !strings.Contains(q.Sub, "250 o restants") || !strings.Contains(q.Sub, "1 à vérifier") {
		t.Errorf("queue = %+v", q)
	}
	if r := v["downloads"].Rows; len(r) != 2 || r[0][0].Text != "Show S01E02" || r[0][1].Text != "75 %" || r[1][3].Text != "Import bloqué" || r[1][3].Level != "warn" {
		t.Errorf("downloads = %+v", r)
	}
	if v["missing"].Text != "895" || v["cutoff"].Text != "8725" {
		t.Errorf("missing/cutoff = %v %v", v["missing"], v["cutoff"])
	}
	if h := v["health"]; h.Level != "warn" || h.Text != "1 alerte" || h.Sub != "Indexeurs indisponibles" {
		t.Errorf("health = %+v", h)
	}
	if l := v["library"]; l.Text != "2" || !strings.Contains(l.Sub, "40 épisodes") || !strings.Contains(l.Sub, "1 suivis") {
		t.Errorf("library = %+v", l)
	}
	if len(v["disks"].Bars) != 2 { // le doublon de volume est écarté
		t.Errorf("disks = %+v", v["disks"].Bars)
	}
	if h := v["history"].Rows; len(h) != 1 || h[0][0].Text != "Death in Paradise S01E08" || h[0][1].Text != "WEBDL-1080p" {
		t.Errorf("history = %+v", h)
	}
	if c := v["calendar"].Rows; len(c) != 2 || c[0][1].Text != "Autre" || c[0][3].Text != "Manquant" || c[1][2].Text != "S01E04" {
		t.Errorf("calendar = %+v", c)
	}
	if v["version"].Text != "4.0.20.3014" || !strings.Contains(v["version"].Sub, "1 j 2 h") {
		t.Errorf("version = %+v", v["version"])
	}
}

func TestRadarrCalendarAndLibrary(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/v3/") {
		case "calendar":
			w.Write([]byte(`[{"title":"La Bataille de Gaulle","inCinemas":"2026-06-26T00:00:00Z","digitalRelease":"` + now.Add(48*time.Hour).Format(time.RFC3339) + `","physicalRelease":"` + now.Add(6*24*time.Hour).Format(time.RFC3339) + `","hasFile":false}]`))
		case "movie":
			w.Write([]byte(`[{"title":"Zootopie 2","monitored":true,"hasFile":true,"sizeOnDisk":4307707941},{"title":"Absent","monitored":true,"hasFile":false,"sizeOnDisk":0}]`))
		default:
			w.WriteHeader(http.StatusNotFound) // les requêtes secondaires en échec n'empêchent rien
		}
	}))
	defer srv.Close()
	src, _ := newRadarr(config.Tile{URL: srv.URL}, fakeSecret)
	a := src.(*arr)
	cal, err := a.calendar(context.Background())
	if err != nil || len(cal.Rows) != 1 || cal.Rows[0][2].Text != "Numérique" || cal.Rows[0][3].Text != "Attendu" {
		t.Errorf("calendar = %+v %v", cal, err)
	}
	if lib := a.slow(context.Background())["library"]; lib.Text != "2" || !strings.Contains(lib.Sub, "4,3 Go · 1 avec fichier · 2 suivis") {
		t.Errorf("library = %+v", lib)
	}
}

func TestArrFieldsZeroValue(t *testing.T) {
	if len((&arr{}).Fields()) == 0 || (&arr{radarr: true}).Fields()[3].Label != "Films" {
		t.Error("champs")
	}
	if _, err := newSonarr(config.Tile{}, func(string) (string, error) { return "", http.ErrNoCookie }); err == nil {
		t.Error("secret absent accepté")
	}
}

func TestJackettLogin(t *testing.T) {
	logins := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/UI/Login":
			http.SetCookie(w, &http.Cookie{Name: "TestCookie", Value: "1", Path: "/"})
			w.WriteHeader(http.StatusFound)
		case "/UI/Dashboard":
			if r.FormValue("password") != "factice" {
				w.WriteHeader(http.StatusFound)
				return
			}
			logins++
			http.SetCookie(w, &http.Cookie{Name: "Jackett", Value: "s", Path: "/"})
			w.WriteHeader(http.StatusFound)
		case "/api/v2.0/indexers":
			if c, err := r.Cookie("Jackett"); err != nil || c.Value != "s" {
				w.Header().Set("Location", "/UI/Login?ReturnUrl=%2Fapi")
				w.WriteHeader(http.StatusFound)
				return
			}
			w.Write([]byte(`[{"id":"trackera","name":"TrackerA","type":"private","configured":true,"language":"fr-FR","last_error":""},{"id":"trackerb","name":"TrackerB","type":"semi-private","language":"fr-FR","last_error":"Request timed out"}]`))
		}
	}))
	defer srv.Close()
	src, _ := newJackett(config.Tile{URL: srv.URL}, fakeSecret)
	j := src.(*jackett)
	v, err := j.poll(context.Background())
	if err != nil || logins != 1 {
		t.Fatalf("poll: %v, logins=%d", err, logins)
	}
	if e := v["errors"]; e.Text != "1" || e.Level != "crit" || e.Sub != "sur 2" {
		t.Errorf("errors = %+v", e)
	}
	if r := v["list"].Rows; len(r) != 2 || r[1][1].Text != "semi-privé" || r[1][3].Level != "crit" || r[0][3].Text != "OK" {
		t.Errorf("list = %+v", r)
	}
	if _, err := j.poll(context.Background()); err != nil || logins != 1 { // session conservée
		t.Errorf("deuxième poll: %v, logins=%d", err, logins)
	}
	j.pass = "mauvais"
	u, _ := url.Parse(srv.URL)
	j.hc.Jar.SetCookies(u, []*http.Cookie{{Name: "Jackett", Value: "perime", Path: "/"}})
	if _, err := j.poll(context.Background()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Errorf("mauvais mot de passe : %v", err)
	}
}

func TestCrossSeedPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ping" {
			w.Write([]byte("OK"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	src, _ := newCrossSeed(config.Tile{URL: srv.URL}, nil)
	v := collect(t, src, func(v tile.Values) bool { return v["state"].Text != "" })
	if v["state"].Text != "En ligne" {
		t.Errorf("state = %+v", v["state"])
	}
	srv.Close()
	if err := src.(*crossSeed).ping(context.Background()); err == nil {
		t.Error("serveur arrêté : erreur attendue")
	}
}
