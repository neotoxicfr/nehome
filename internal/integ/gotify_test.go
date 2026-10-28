package integ

import (
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses d'après le swagger de Gotify (GET /message, /application, /version).
func TestGotify(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	srv := f4Serve(t, f4Hdr("X-Gotify-Key", "factice"), nil, map[string]string{
		"/application":       `[{"id":5,"name":"Backup","description":"sauvegardes","token":"SECRETAPP"}]`,
		"/version":           `{"version":"2.6.1","commit":"x","buildDate":"2026-01-01"}`,
		"/message?limit=100": `{"messages":[{"id":2,"appid":5,"message":"CORPSSECRET","title":"Terminé","priority":9,"date":"` + now + `"},{"id":1,"appid":5,"message":"m","title":"Ancien","priority":1,"date":"` + old + `"}],"paging":{"size":2,"limit":100}}`,
	})
	defer srv.Close()
	src, _ := newGotify(config.Tile{URL: srv.URL}, mediaSecret)
	v, err := src.(*gotify).poll(t.Context())
	if err != nil || v["day"].Text != "1" || v["high"].Text != "1" || v["apps"].Text != "1" || v["version"].Text != "2.6.1" || v["last"].Sub != "Backup" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	for _, f := range []string{"messages", "app_list"} {
		for _, row := range v[f].Rows {
			for _, c := range row {
				if strings.Contains(c.Text, "SECRET") {
					t.Fatal("secret ou corps de message affiché")
				}
			}
		}
	}
	bad, _ := newGotify(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*gotify).poll(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue : %v", err)
	}
}
