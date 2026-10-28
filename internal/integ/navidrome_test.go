package integ

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formes d'après la spec Subsonic 1.16.1 et OpenSubsonic (getNowPlaying,
// getScanStatus, getArtists) ; authentification : t = md5(mot de passe + s).
func TestNavidrome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		sum := md5.Sum([]byte("factice" + q.Get("s")))
		if q.Get("u") != "neo" || q.Get("t") != hex.EncodeToString(sum[:]) || len(q.Get("s")) < 6 || q.Get("f") != "json" || q.Get("v") == "" || q.Get("c") == "" {
			w.Write([]byte(`{"subsonic-response":{"status":"failed","version":"1.16.1","error":{"code":40,"message":"Wrong username or password"}}}`))
			return
		}
		switch r.URL.Path {
		case "/rest/getNowPlaying":
			w.Write([]byte(`{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome","serverVersion":"0.54.5","openSubsonic":true,"nowPlaying":{"entry":[{"title":"One More Time","artist":"Daft Punk","album":"Discovery","username":"neo","minutesAgo":2,"playerName":"Symfonium"}]}}}`))
		case "/rest/getScanStatus":
			w.Write([]byte(`{"subsonic-response":{"status":"ok","version":"1.16.1","scanStatus":{"scanning":false,"count":5230,"folderCount":410,"lastScan":"2025-01-02T03:04:05Z"}}}`))
		case "/rest/getArtists":
			w.Write([]byte(`{"subsonic-response":{"status":"ok","version":"1.16.1","artists":{"ignoredArticles":"The","index":[{"name":"A","artist":[{"id":"1","name":"Air","albumCount":3},{"id":"2","name":"Alt-J","albumCount":2}]},{"name":"D","artist":[{"id":"3","name":"Daft Punk","albumCount":4}]}]}}}`))
		default:
			t.Errorf("chemin inattendu %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	src, err := newNavidrome(config.Tile{URL: srv.URL}, secretsF1)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["artists"].Text != "" && v["scan"].Text != "" && v["playing"].Text != ""
	})
	if v["playing"].Text != "1" || v["version"].Text != "0.54.5" {
		t.Errorf("playing/version = %v %v", v["playing"], v["version"])
	}
	if r := v["now_playing"].Rows; len(r) != 1 || r[0][1].Text != "Daft Punk – One More Time" || r[0][4].Text != "2 min" {
		t.Errorf("now_playing = %+v", r)
	}
	if v["tracks"].Text != "5230" || v["scan"].Text != "Terminé" || v["artists"].Text != "3" || v["albums"].Text != "9" {
		t.Errorf("compteurs = %v", v)
	}
}

// Une réponse « failed » devient une erreur, sans le jeton.
func TestNavidromeAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"subsonic-response":{"status":"failed","error":{"code":40,"message":"Wrong username or password"}}}`))
	}))
	defer srv.Close()
	n, _ := newNavidrome(config.Tile{URL: srv.URL, Options: map[string]string{"user": "neo"}}, secretsF1)
	_, err := n.(*navidrome).get(t.Context(), "ping")
	if err == nil || !strings.Contains(err.Error(), "Wrong username") || strings.Contains(err.Error(), "t=") {
		t.Errorf("err = %v", err)
	}
}

// secretsF1 : tous les rôles ont une valeur (user « neo », le reste « factice »).
func secretsF1(role string) (string, error) {
	if role == "user" {
		return "neo", nil
	}
	return "factice", nil
}
