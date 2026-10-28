package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponse d'après la doc de l'API Healthchecks (GET /api/v3/checks/).
func TestHealthchecks(t *testing.T) {
	srv := f4Serve(t, f4Hdr("X-Api-Key", "factice"), nil, map[string]string{
		"/api/v3/checks/": `{"checks":[
{"name":"Sauvegarde","slug":"sauv","tags":"nas","status":"up","last_ping":"2026-10-27T03:00:00+00:00","next_ping":"2026-10-28T03:00:00+00:00"},
{"name":"Renouvellement","slug":"cert","tags":"","status":"down","last_ping":"2026-10-20T03:00:00+00:00","next_ping":null},
{"name":"Test","slug":"t","tags":"","status":"paused","last_ping":null,"next_ping":null}]}`,
	})
	defer srv.Close()
	src, _ := newHealthchecks(config.Tile{URL: srv.URL}, mediaSecret)
	v, err := src.(*healthchecks).poll(t.Context())
	if err != nil || v["state"].Text != "1 en panne" || v["state"].Level != "crit" || v["up"].Text != "1" || v["paused"].Text != "1" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	if r := v["checks"].Rows; len(r) != 3 || r[0][0].Text != "Renouvellement" || r[2][0].Text != "Sauvegarde" || r[1][2].Text != "jamais" {
		t.Fatalf("ordre : %+v", r)
	}
}
