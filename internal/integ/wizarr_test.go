package integ

import (
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses construites d'après le swagger de Wizarr (pas de clé pour les relever).
func TestWizarr(t *testing.T) {
	in3 := time.Now().Add(72 * time.Hour).UTC().Format("2006-01-02T15:04:05")
	srv := mediaServer(t, "X-API-Key", "factice", map[string]string{
		"/api/status":                `{"users":86,"invites":12,"pending":3,"expired":4}`,
		"/api/version":               `{"version":"2026.1.0"}`,
		"/api/activity/stats?days=7": `{"total_sessions":40,"unique_users":9,"total_watch_time_seconds":90000}`,
		"/api/users":                 `{"users":[{"username":"sans_fin","server":"Neoflix","expires":null,"created_at":"2026-01-02T10:00:00"},{"username":"bientot","server":"Neoflix","expires":"` + in3 + `","created_at":"2026-09-01T10:00:00"}],"count":2}`,
		"/api/invitations":           `{"invitations":[{"code":"CODESECRET","status":"pending","created":"2026-10-20T10:00:00","display_name":"Invité","used_by":null}],"count":1}`,
	})
	defer srv.Close()
	src, err := newWizarr(config.Tile{URL: srv.URL}, mediaSecret)
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*wizarr).poll(t.Context())
	if err != nil || v["users"].Text != "86" || v["users"].Sub != "1 expirent sous 7 j" || v["pending"].Text != "3" || v["version"].Text != "2026.1.0" || v["activity"].Text != "40 sessions" {
		t.Fatalf("compteurs : %v %+v", err, v)
	}
	if u := v["user_list"].Rows; len(u) != 2 || u[0][0].Text != "bientot" || u[0][2].Level != "warn" || u[1][2].Text != "jamais" {
		t.Fatalf("comptes : %+v", u)
	}
	for _, row := range v["invite_list"].Rows {
		for _, c := range row {
			if strings.Contains(c.Text, "CODESECRET") {
				t.Fatal("code d'invitation affiché")
			}
		}
	}
	// Sans clé valable : panne avec erreur, sans fuite.
	bad, _ := newWizarr(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*wizarr).poll(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue : %v", err)
	}
}
