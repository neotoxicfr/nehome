package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses d'après la doc HTTP API de Grafana (health, admin/stats) et l'API
// Alertmanager v2 (alertes actives).
func TestGrafana(t *testing.T) {
	srv := f4Serve(t, f4Hdr("Authorization", "Bearer factice"), nil, map[string]string{
		"/api/health":      `{"commit":"abc","database":"ok","version":"11.3.0"}`,
		"/api/admin/stats": `{"orgs":1,"dashboards":23,"datasources":4,"users":6,"activeSessions":2}`,
		"/api/alertmanager/grafana/api/v2/alerts?active=true&silenced=false&inhibited=false": `[{"labels":{"alertname":"DisqueP","severity":"critical"},"annotations":{"summary":"Disque plein"},"startsAt":"2026-10-27T08:00:00Z","status":{"state":"active"}}]`,
	})
	defer srv.Close()
	src, _ := newGrafana(config.Tile{URL: srv.URL}, mediaSecret)
	v, err := src.(*grafana).poll(t.Context())
	if err != nil || v["health"].Text != "OK" || v["dashboards"].Text != "23" || v["alerts"].Text != "1" || v["alerts"].Level != "crit" || v["version"].Text != "11.3.0" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	if r := v["alert_list"].Rows; len(r) != 1 || r[0][0].Text != "DisqueP" {
		t.Fatalf("alertes : %+v", r)
	}
	// Sans droit admin ni alertmanager : la santé reste affichée.
	srv2 := f4Serve(t, nil, nil, map[string]string{"/api/health": `{"database":"failing","version":"11"}`})
	defer srv2.Close()
	src, _ = newGrafana(config.Tile{URL: srv2.URL}, mediaSecret)
	v, err = src.(*grafana).poll(t.Context())
	if err != nil || v["health"].Level != "crit" || v["users"].Text != "" {
		t.Fatalf("dégradé : %v %+v", err, v)
	}
}
