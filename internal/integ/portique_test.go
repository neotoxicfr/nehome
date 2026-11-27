package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme de crates/server/src/api/machine.rs (Portique) : online null = par rebond.
func TestPortique(t *testing.T) {
	body := `{"machines":[{"id":10003,"name":"Routeur","rdp":false,"ssh":true,"can_wake":false,"online":true,"sessions":{"rdp":0,"ssh":2}},
{"id":10007,"name":"Distant","rdp":false,"ssh":true,"can_wake":false,"online":null,"sessions":{"rdp":0,"ssh":0}},
{"id":1,"name":"PC","rdp":true,"ssh":true,"can_wake":true,"online":false,"sessions":{"rdp":1,"ssh":0}},
{"id":5,"name":"Serveur","rdp":false,"ssh":true,"can_wake":false,"online":false,"sessions":{"rdp":0,"ssh":0}}],"online":1,"sessions":{"rdp":1,"ssh":2}}`
	woken := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/machine/status" && r.Header.Get("Authorization") == "Bearer lecture":
			w.Write([]byte(body))
		case r.Method == http.MethodPost && r.Header.Get("Authorization") == "Bearer reveil":
			woken = r.URL.Path
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	keys := map[string]string{"token": "lecture", "wake": "reveil"}
	src, err := newPortique(config.Tile{URL: srv.URL, Href: "https://portique.example/", Secrets: map[string]string{"token": "a", "wake": "b"}}, func(r string) (string, error) { return keys[r], nil })
	if err != nil {
		t.Fatal(err)
	}
	p := src.(*portique)
	v, err := p.poll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["online"].Text != "1 / 4" || v["online"].Sub != "1 hors ligne" || v["online"].Level != "warn" || v["sessions"].Text != "1 bureau · 2 terminaux" {
		t.Fatalf("résumé : %+v", v)
	}
	rows := v["machines"].Rows
	if rows[1][1].Text != "Par rebond" || rows[2][1].Text != "Éteinte" || rows[3][1].Level != "crit" || rows[1][4].Text != "—" || rows[2][2].Href != "https://portique.example/session/1" || rows[2][3].Href != "https://portique.example/ssh/1" || rows[0][2].Text != "—" || rows[0][4].Text != "2 terminaux" {
		t.Fatalf("lignes : %+v", rows)
	}
	a := p.Actions()
	if len(a) != 1 || a[0].ID != "wake-1" || a[0].Label != "Réveiller PC" || a[0].Default != "tile" {
		t.Fatalf("actions : %+v", a)
	}
	if _, err := p.Do(t.Context(), "wake-10003", ""); err == nil {
		t.Fatal("machine non réveillable acceptée")
	}
	if _, err := p.Do(t.Context(), "wake-1", ""); err != nil || woken != "/api/machine/wake/1" {
		t.Fatalf("réveil : %v %q", err, woken)
	}

	// sans clé de réveil : aucun bouton
	src, _ = newPortique(config.Tile{URL: srv.URL}, func(string) (string, error) { return "lecture", nil })
	p = src.(*portique)
	if v, err := p.poll(t.Context()); err != nil || len(p.Actions()) != 0 || v["machines"].Rows[2][2].Text != "oui" {
		t.Fatalf("sans wake : %v %+v", err, p.Actions())
	}
}
