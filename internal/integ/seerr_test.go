package integ

import (
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestSeerr(t *testing.T) {
	srv := mediaServer(t, "X-Api-Key", "factice", map[string]string{
		"/api/v1/request/count": `{"total":362,"movie":183,"tv":179,"pending":1,"approved":137,"declined":1,"processing":123,"available":14,"completed":223}`,
		"/api/v1/issue/count":   `{"total":25,"video":10,"open":2,"closed":23}`,
		"/api/v1/status":        `{"version":"3.5.1","updateAvailable":true}`,
		"/api/v1/request?take=15&sort=added&filter=pending":    `{"results":[{"status":1,"type":"movie","createdAt":"2026-10-27T14:00:58.000Z","media":{"tmdbId":603,"status":1},"requestedBy":{"displayName":"alice","email":"ne-pas-afficher@example.com"}}]}`,
		"/api/v1/request?take=15&sort=added&filter=processing": `{"results":[{"status":2,"type":"tv","createdAt":"2026-10-25T09:51:43.000Z","media":{"tmdbId":41956,"status":3},"requestedBy":{"displayName":"bob"}}]}`,
		"/api/v1/movie/603": `{"title":"Matrix"}`,
		"/api/v1/tv/41956":  `{"name":"Meurtres au paradis"}`,
	})
	defer srv.Close()
	src, err := newSeerr(config.Tile{URL: srv.URL}, mediaSecret)
	if err != nil {
		t.Fatal(err)
	}
	s := src.(*seerr)
	v, err := s.poll(t.Context())
	if err != nil || v["pending"].Level != "warn" || v["processing"].Text != "123" || v["issues"].Text != "2" || v["issues"].Level != "warn" || v["version"].Level != "warn" {
		t.Fatalf("compteurs : %v %+v", err, v)
	}
	r := v["requests"].Rows
	if len(r) != 2 || r[0][0].Text != "Matrix" || r[0][4].Text != "En attente" || r[1][0].Text != "Meurtres au paradis" || r[1][1].Text != "Série" || r[1][2].Text != "bob" {
		t.Fatalf("demandes : %+v", r)
	}
	for _, row := range r {
		for _, c := range row {
			if strings.Contains(c.Text, "@") {
				t.Fatalf("adresse affichée : %q", c.Text)
			}
		}
	}
	if len(s.titles) != 2 {
		t.Fatalf("cache : %v", s.titles)
	}
	// Titre introuvable : repli lisible, pas de cache.
	if got := s.title(t.Context(), "movie", 1); got != "TMDB movie/1" || len(s.titles) != 2 {
		t.Fatalf("repli : %q", got)
	}
}
