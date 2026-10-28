package integ

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Réponses construites d'après le code de Deluge (deluge/ui/web/json_api.py :
// auth.login, web.update_ui, erreur « Not authenticated » de code 1).
func TestDeluge(t *testing.T) {
	logins, expire := 0, true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string
			Params []any
			ID     int
		}
		json.NewDecoder(r.Body).Decode(&req)
		reply := func(res string) {
			fmt.Fprintf(w, `{"id":%d,"result":%s,"error":null}`, req.ID, res)
		}
		switch req.Method {
		case "auth.login":
			logins++
			if req.Params[0] != "factice" {
				reply("false")
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "_session_id", Value: "s", Path: "/"})
			reply("true")
		case "web.update_ui":
			if c, err := r.Cookie("_session_id"); err != nil || c.Value != "s" {
				w.Write([]byte(`{"id":1,"result":null,"error":{"message":"Not authenticated","code":1}}`))
				return
			}
			if expire { // la session saute une fois : doit déclencher un nouveau login
				expire = false
				http.SetCookie(w, &http.Cookie{Name: "_session_id", Value: "", Path: "/", MaxAge: -1})
				w.Write([]byte(`{"id":1,"result":null,"error":{"message":"Not authenticated","code":1}}`))
				return
			}
			reply(`{"connected":true,"filters":{},"stats":{"max_download":-1.0,"max_upload":100.0,"max_num_connections":200,"num_connections":42,"download_rate":2000000.0,"upload_rate":300000.0,"dht_nodes":312,"has_incoming_connections":true,"free_space":123000000000},
			"torrents":{"aa":{"name":"ubuntu.iso","state":"Downloading","progress":25.5,"download_payload_rate":2000000,"upload_payload_rate":0,"eta":3600,"total_size":5000000000,"ratio":0.0},
			"bb":{"name":"seed","state":"Seeding","progress":100.0,"download_payload_rate":0,"upload_payload_rate":300000,"eta":0,"total_size":1,"ratio":1.5},
			"cc":{"name":"pause","state":"Paused","progress":10.0,"download_payload_rate":0,"upload_payload_rate":0,"eta":0,"total_size":1,"ratio":0.0}}}`)
		}
	}))
	defer srv.Close()
	src, err := newDeluge(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"pass": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool { return v["down"].Text != "" })
	if logins != 2 {
		t.Fatalf("connexions : %d (attendu 2 : initiale + reprise après expiration)", logins)
	}
	if v["down"].Text != "2 Mo/s" || v["leech"].Text != "1" || v["seed"].Text != "1" || v["paused"].Text != "1" {
		t.Fatalf("compteurs : %+v", v)
	}
	if v["limits"].Text != "↓ ∞ · ↑ 102 ko/s" || v["conn"].Text != "42 / 200" || v["incoming"].Text != "oui" || v["free"].Text != "123 Go" {
		t.Fatalf("détail : %+v %+v %+v %+v", v["limits"], v["conn"], v["incoming"], v["free"])
	}
	if r := v["dl"].Rows; len(r) != 1 || r[0][1].Text != "25,5 %" {
		t.Fatalf("table : %+v", v["dl"].Rows)
	}
}

func TestDelugeBadPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"result":false,"error":null}`))
	}))
	defer srv.Close()
	src, _ := newDeluge(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"pass": "mauvais"}))
	if _, err := src.(*deluge).update(t.Context()); err == nil {
		t.Fatal("mot de passe refusé non signalé")
	}
}
