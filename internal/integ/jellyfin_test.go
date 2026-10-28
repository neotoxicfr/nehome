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

// Formes de réponses d'après l'OpenAPI de Jellyfin (SessionInfoDto,
// ItemCounts, SystemInfo, ActivityLogEntryQueryResult) ; Emby : même API
// sous /emby.
func TestJellyfinAndEmby(t *testing.T) {
	now := time.Now().UTC()
	for _, emby := range []bool{false, true} {
		prefix := ""
		if emby {
			prefix = "/emby"
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok := r.Header.Get("Authorization") == `MediaBrowser Token="factice"` && r.Header.Get("X-Emby-Token") == ""
			if emby {
				ok = r.Header.Get("X-Emby-Token") == "factice"
			}
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch strings.TrimPrefix(r.URL.Path, prefix) {
			case "/Sessions":
				w.Write([]byte(`[{"UserName":"alice","Client":"Jellyfin Web","DeviceName":"Firefox","PlayState":{"PositionTicks":300,"IsPaused":false,"PlayMethod":"Transcode"},"NowPlayingItem":{"Name":"Pilote","SeriesName":"Show","RunTimeTicks":1200}},
{"UserName":"bob","Client":"Kodi","DeviceName":"Salon","PlayState":{"PlayMethod":"DirectPlay"}}]`))
			case "/Items/Counts":
				w.Write([]byte(`{"MovieCount":120,"SeriesCount":30,"EpisodeCount":900,"SongCount":0,"BookCount":0}`))
			case "/System/Info":
				w.Write([]byte(`{"ServerName":"salon","Version":"10.10.7","HasUpdateAvailable":true}`))
			case "/System/ActivityLog/Entries":
				w.Write([]byte(`{"Items":[{"Name":"alice s'est connectée","Severity":"Information","Date":"` + now.Add(-time.Minute).Format(time.RFC3339) + `"},{"Name":"Échec","Severity":"Error","Date":"` + now.Format(time.RFC3339) + `"}]}`))
			default:
				t.Errorf("chemin inattendu %s", r.URL)
			}
		}))
		mk := newJellyfin
		if emby {
			mk = newEmby
		}
		src, err := mk(config.Tile{URL: srv.URL}, fakeSecret)
		if err != nil {
			t.Fatal(err)
		}
		v := collect(t, src, func(v tile.Values) bool {
			return v["activity"].Head != nil && v["streams"].Text != "" && v["movies"].Text != ""
		})
		srv.Close()
		if s := v["streams"]; s.Text != "1" || s.Sub != "1 transcodage" || v["transcodes"].Text != "1" {
			t.Errorf("emby=%v streams = %+v", emby, s)
		}
		if r := v["sessions"].Rows; len(r) != 1 || r[0][1].Text != "Show – Pilote" || r[0][2].Text != "25 %" || r[0][3].Text != "Transcodage" {
			t.Errorf("sessions = %+v", r)
		}
		if v["movies"].Text != "120" || v["episodes"].Text != "900" {
			t.Errorf("compteurs = %v", v)
		}
		if _, ok := v["music"]; ok {
			t.Error("music ne doit pas être émis à zéro morceau")
		}
		if x := v["version"]; x.Text != "10.10.7" || x.Level != "warn" {
			t.Errorf("version = %+v", x)
		}
		if a := v["activity"].Rows; len(a) != 2 || a[1][0].Level != "crit" {
			t.Errorf("activity = %+v", a)
		}
	}
}
