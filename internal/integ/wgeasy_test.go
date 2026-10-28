package integ

import (
	"net/http"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// v14 : GET /api/wireguard/client (src/lib/WireGuard.js getClients : id, name,
// enabled, address, latestHandshakeAt, transferRx, transferTx). v15 : GET
// /api/client en Basic (champs ipv4Address, latestHandshakeAt, transferRx/Tx).
func TestWGEasy(t *testing.T) {
	recent := time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	body := `[{"id":"1","name":"telephone","enabled":true,"address":"10.8.0.2","ipv4Address":"10.8.0.2","latestHandshakeAt":"` + recent + `","transferRx":1000000,"transferTx":2000000},
{"id":"2","name":"portable","enabled":true,"address":"10.8.0.3","ipv4Address":"10.8.0.3","latestHandshakeAt":null,"transferRx":0,"transferTx":0}]`

	v14 := f4Serve(t, f4Hdr("Authorization", "factice"), nil, map[string]string{"/api/wireguard/client": body})
	defer v14.Close()
	src, err := newWGEasy(config.Tile{URL: v14.URL}, func(role string) (string, error) {
		if role == "user" {
			return "", http.ErrNoCookie // pas de rôle user : v14
		}
		return "factice", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*wgeasy).poll(t.Context())
	if err != nil || v["online"].Text != "1 / 2" || v["rx"].Text != "1 Mo" || v["tx"].Text != "2 Mo" || v["client_list"].Rows[0][3].Text != "jamais" {
		t.Fatalf("v14 : %v %+v", err, v)
	}

	v15 := f4Serve(t, func(r *http.Request) bool { u, p, ok := r.BasicAuth(); return ok && u == "admin" && p == "factice" }, nil, map[string]string{"/api/client": body})
	defer v15.Close()
	src, _ = newWGEasy(config.Tile{URL: v15.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(string) (string, error) { return "factice", nil }) // user = « factice » : refusé
	if _, err := src.(*wgeasy).poll(t.Context()); err == nil {
		t.Fatal("v15 : identifiants faux acceptés")
	}
	src, _ = newWGEasy(config.Tile{URL: v15.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		if role == "user" {
			return "admin", nil
		}
		return "factice", nil
	})
	v, err = src.(*wgeasy).poll(t.Context())
	if err != nil || v["online"].Text != "1 / 2" || v["client_list"].Rows[1][1].Text != "10.8.0.2" {
		t.Fatalf("v15 : %v %+v", err, v)
	}

	// user déclaré mais illisible : erreur, pas de repli silencieux en v14
	if _, err := newWGEasy(config.Tile{URL: v15.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		if role == "user" {
			return "", http.ErrNoCookie
		}
		return "factice", nil
	}); err == nil {
		t.Fatal("secret user illisible accepté")
	}
}
