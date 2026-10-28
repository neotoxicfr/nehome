package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formes d'après la doc de l'API Audiobookshelf (api.audiobookshelf.org :
// GET /api/libraries, /api/libraries/{id}/stats, /api/users/online, /status).
func TestAudiobookshelf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/status":
			w.Write([]byte(`{"app":"audiobookshelf","serverVersion":"2.17.5","isInit":true}`))
		case "/api/libraries":
			w.Write([]byte(`{"libraries":[{"id":"lib1","name":"Livres audio","mediaType":"book"},{"id":"lib2","name":"Podcasts","mediaType":"podcast"}]}`))
		case "/api/libraries/lib1/stats":
			w.Write([]byte(`{"totalItems":40,"totalAuthors":12,"totalDuration":360000,"totalSize":8000000000}`))
		case "/api/libraries/lib2/stats":
			w.Write([]byte(`{"totalItems":10,"totalAuthors":0,"totalDuration":36000,"totalSize":1000000000}`))
		case "/api/users/online":
			w.Write([]byte(`{"usersOnline":[{"id":"u1","username":"neo"}],"openSessions":[{"displayTitle":"Dune","displayAuthor":"Frank Herbert","currentTime":900,"duration":3600,"userId":"u1","deviceInfo":{"clientName":"Abs Web","deviceName":"Firefox"}}]}`))
		default:
			t.Errorf("chemin inattendu %s", r.URL)
		}
	}))
	defer srv.Close()
	src, err := newAudiobookshelf(config.Tile{URL: srv.URL}, secretsF1)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["libraries"].Head != nil && v["streams"].Text != "" && v["version"].Text != ""
	})
	if v["items"].Text != "50" || v["duration"].Text != "110 h" || v["size"].Text != "9 Go" || v["authors"].Text != "12" {
		t.Errorf("totaux = %v", v)
	}
	if r := v["sessions"].Rows; v["streams"].Text != "1" || len(r) != 1 || r[0][0].Text != "neo" || r[0][1].Text != "Frank Herbert – Dune" || r[0][2].Text != "25 %" || r[0][3].Text != "Abs Web · Firefox" {
		t.Errorf("sessions = %+v", r)
	}
	if r := v["libraries"].Rows; len(r) != 2 || r[1][1].Text != "Podcasts" {
		t.Errorf("libraries = %+v", r)
	}
}
