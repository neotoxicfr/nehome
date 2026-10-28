package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Le sujet réel était vide lors du relevé : messages au format de la doc ntfy
// (JSON par ligne, priorité absente = 3) ; le 403 est la réponse réelle.
const ntfyFixture = `{"id":"a1","time":1793080000,"expires":1793123200,"event":"open","topic":"uptime"}
{"id":"b2","time":1793080100,"expires":1793123300,"event":"message","topic":"uptime","title":"Plex est hors ligne","message":"Timeout","priority":5,"tags":["red_circle"]}
{"id":"c3","time":1793080400,"expires":1793123600,"event":"message","topic":"uptime","message":"Plex est de nouveau en ligne","tags":["green_circle"]}
`

func TestNtfy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "homepage" || p != "factice" || r.URL.Path != "/uptime/json" || r.URL.Query().Get("poll") != "1" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"code":40301,"http":403,"error":"forbidden","link":"https://ntfy.sh/docs/publish/#authentication"}`))
			return
		}
		w.Write([]byte(ntfyFixture))
	}))
	defer srv.Close()
	secret := func(role string) (string, error) {
		return map[string]string{"user": "homepage", "pass": "factice"}[role], nil
	}
	src, err := newNtfy(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, secret)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := src.(*ntfy).poll(t.Context())
	if err != nil || len(msgs) != 2 {
		t.Fatalf("poll : %v %d", err, len(msgs))
	}
	v := ntfyValues(msgs, time.Unix(1793080400+3600, 0))
	if v["count"].Text != "2" || v["urgent"].Text != "1" || v["urgent"].Level != "warn" {
		t.Errorf("compteurs : %+v %+v", v["count"], v["urgent"])
	}
	if v["last"].Text != "Plex est de nouveau en ligne" || v["last"].Sub != "il y a 1 h 0 min" || v["last"].Level != "" {
		t.Errorf("dernier : %+v", v["last"])
	}
	rows := v["messages"].Rows
	if len(rows) != 2 || rows[1][1].Text != "Plex est hors ligne" || rows[1][1].Level != "crit" || rows[0][1].Text != "green_circle" {
		t.Errorf("table : %+v", rows)
	}

	other, _ := newNtfy(config.Tile{URL: srv.URL, Options: map[string]string{"topic": "csp"}, Secrets: map[string]string{"user": "u", "pass": "p"}}, secret)
	if _, err := other.(*ntfy).poll(t.Context()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("sujet interdit : %v", err)
	}
	if v := ntfyValues(nil, time.Now()); v["count"].Text != "0" || v["last"].Text != "aucun" {
		t.Errorf("vide : %+v", v)
	}
}
