package integ

import (
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme d'après le schéma OpenAPI d'authentik v3 (listes paginées
// {pagination:{count},results}, /admin/version/, /events/events/).
func TestAuthentik(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	srv := f4Serve(t, f4Hdr("Authorization", "Bearer factice"), nil, map[string]string{
		"/api/v3/core/users/?is_active=true&page_size=1": `{"pagination":{"count":7},"results":[]}`,
		"/api/v3/core/applications/?page_size=1":         `{"pagination":{"count":12},"results":[]}`,
		"/api/v3/admin/version/":                         `{"version_current":"2025.8.1","version_latest":"2025.10.0","outdated":true}`,
		"/api/v3/events/events/?action=login_failed&ordering=-created&page_size=100": `{"pagination":{"count":2},"results":[
{"created":"` + now + `","client_ip":"203.0.113.9","user":{"username":"MOTDEPASSE"},"action":"login_failed","context":{"stage":"default-authentication-identification","message":"x"}}]}`,
	})
	defer srv.Close()
	src, _ := newAuthentik(config.Tile{URL: srv.URL}, mediaSecret)
	v, err := src.(*authentik).poll(t.Context())
	if err != nil || v["users"].Text != "7" || v["apps"].Text != "12" || v["failed"].Text != "1" || v["failed"].Level != "warn" || v["version"].Level != "warn" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	for _, c := range v["failed_list"].Rows[0] {
		if strings.Contains(c.Text, "MOTDEPASSE") {
			t.Fatal("identifiant saisi affiché")
		}
	}
	if v["failed_list"].Rows[0][1].Text != "203.0.113.9" {
		t.Fatalf("ligne : %+v", v["failed_list"].Rows)
	}
}
