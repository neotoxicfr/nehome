package integ

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme d'après le schéma de l'API de Nginx Proxy Manager (backend/schema :
// POST /tokens {identity,secret}, proxy-hosts, certificates, expires_on).
func TestNPM(t *testing.T) {
	soon := time.Now().Add(5 * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	later := time.Now().Add(60 * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	routes := map[string]string{
		"/api/nginx/proxy-hosts":       `[{"domain_names":["a.example.com"],"forward_host":"10.0.0.2","forward_port":80,"enabled":true},{"domain_names":["b.example.com"],"forward_host":"10.0.0.3","forward_port":8080,"enabled":0}]`,
		"/api/nginx/certificates":      `[{"nice_name":"*.example.com","domain_names":["*.example.com"],"provider":"letsencrypt","expires_on":"` + later + `"},{"nice_name":"vieux.example.com","provider":"letsencrypt","expires_on":"` + soon + `"}]`,
		"/api/nginx/redirection-hosts": `[{}]`,
		"/api/nginx/streams":           `[]`,
		"/api/nginx/dead-hosts":        `[]`,
		"/api/":                        `{"status":"OK","version":{"major":2,"minor":12,"revision":3}}`,
	}
	logins := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tokens" {
			var in map[string]string
			json.NewDecoder(r.Body).Decode(&in)
			if r.Method != http.MethodPost || in["identity"] != "admin@example.com" || in["secret"] != "factice" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			logins++
			w.Write([]byte(`{"token":"JETON","expires":"2027-01-01T00:00:00.000Z"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer JETON" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	src, _ := newNPM(config.Tile{URL: srv.URL}, func(role string) (string, error) {
		if role == "user" {
			return "admin@example.com", nil
		}
		return "factice", nil
	})
	v, err := src.(*npm).poll(t.Context())
	if err != nil || logins != 1 {
		t.Fatalf("login : %v (%d)", err, logins)
	}
	if v["proxy"].Text != "1 actifs" || v["proxy"].Sub != "1 désactivés" || v["certs"].Text != "2" || v["expiry"].Level != "crit" || v["expiry"].Sub != "vieux.example.com" || v["other"].Text != "1 / 0 / 0" || v["version"].Text != "2.12.3" {
		t.Fatalf("valeurs : %+v", v)
	}
	if r := v["cert_list"].Rows; len(r) != 2 || r[0][0].Text != "vieux.example.com" {
		t.Fatalf("certificats : %+v", r)
	}
	// Jeton expiré : une seule reconnexion, sans fuite du mot de passe.
	src.(*npm).token = "PERIME"
	if _, err := src.(*npm).poll(t.Context()); err != nil || logins != 2 {
		t.Fatalf("relance : %v (%d)", err, logins)
	}
	bad, _ := newNPM(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*npm).poll(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue : %v", err)
	}
}
