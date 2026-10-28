package integ

import (
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses d'après l'API Swagger de Gitea/Forgejo (/api/v1 : repos/search,
// notifications/new, version, admin/users avec X-Total-Count).
func TestGitea(t *testing.T) {
	srv := f4Serve(t, f4Hdr("Authorization", "token factice"), map[string]string{"X-Total-Count": "42"}, map[string]string{
		"/api/v1/repos/search":      `{"ok":true,"data":[{"full_name":"neo/nehome","stars_count":3,"open_issues_count":2,"updated_at":"2026-10-26T10:00:00Z","private":true}]}`,
		"/api/v1/version":           `{"version":"1.22.3"}`,
		"/api/v1/notifications/new": `{"new":4}`,
		"/api/v1/admin/users":       `[]`,
		"/api/v1/admin/orgs":        `[]`,
	})
	defer srv.Close()
	for _, mk := range []func(config.Tile, func(string) (string, error)) (any, error){
		func(c config.Tile, s func(string) (string, error)) (any, error) { return newGitea(c, s) },
		func(c config.Tile, s func(string) (string, error)) (any, error) { return newForgejo(c, s) },
	} {
		src, err := mk(config.Tile{URL: srv.URL}, mediaSecret)
		if err != nil {
			t.Fatal(err)
		}
		v, err := src.(*gitea).poll(t.Context())
		if err != nil || v["repos"].Text != "42" || v["users"].Text != "42" || v["orgs"].Text != "42" || v["notifications"].Text != "4" || v["version"].Text != "1.22.3" {
			t.Fatalf("valeurs : %v %+v", err, v)
		}
		if r := v["recent"].Rows; len(r) != 1 || !strings.Contains(r[0][0].Text, "(privé)") {
			t.Fatalf("table : %+v", r)
		}
	}
	// Jeton non admin : users/orgs absents mais le reste vit.
	srv2 := f4Serve(t, nil, map[string]string{"X-Total-Count": "1"}, map[string]string{"/api/v1/repos/search": `{"data":[]}`})
	defer srv2.Close()
	src, _ := newGitea(config.Tile{URL: srv2.URL}, mediaSecret)
	v, err := src.(*gitea).poll(t.Context())
	if err != nil || v["repos"].Text != "1" || v["users"].Text != "" {
		t.Fatalf("sans droits admin : %v %+v", err, v)
	}
	bad, _ := newGitea(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*gitea).poll(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue : %v", err)
	}
}
