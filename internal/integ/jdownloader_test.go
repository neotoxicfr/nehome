package integ

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Faux relais MyJD : vérifie signatures et chiffrement comme le vrai. Les
// données appareil sont des réponses réelles (noms anonymisés) ; le paquet en
// cours est ajouté à la main (rien ne tournait lors du relevé).
func TestJDownloader(t *testing.T) {
	login := jdHash([]byte("admin@example.com" + "factice" + "server"))
	device := jdHash([]byte("admin@example.com" + "factice" + "device"))
	var mu sync.Mutex
	sessions := 0
	session := ""
	expireOnce := false
	connects := 0

	reply := func(w http.ResponseWriter, key []byte, rid int64, extra map[string]any) {
		extra["rid"] = rid
		b, _ := json.Marshal(extra)
		w.Write([]byte(jdEncrypt(b, key)))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/my/") {
			q := r.URL.Query()
			key := login
			if r.URL.Path != "/my/connect" {
				if q.Get("sessiontoken") != session {
					w.WriteHeader(http.StatusForbidden)
					w.Write([]byte(`{"src":"MYJD","type":"TOKEN_INVALID"}`))
					return
				}
				key = jdToken(login, session)
			}
			raw, sig, _ := strings.Cut(r.URL.RequestURI(), "&signature=")
			mac := hmac.New(sha256.New, key)
			mac.Write([]byte(raw))
			if sig != hex.EncodeToString(mac.Sum(nil)) {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"src":"MYJD","type":"AUTH_FAILED"}`))
				return
			}
			var rid int64
			fmt.Sscan(q.Get("rid"), &rid)
			newSession := func() map[string]any {
				sessions++
				session = fmt.Sprintf("%064x", sessions)
				return map[string]any{"sessiontoken": session, "regaintoken": "ab"} // regaintoken inutilisé
			}
			switch r.URL.Path {
			case "/my/connect":
				connects++
				if q.Get("email") != "admin@example.com" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				reply(w, key, rid, newSession())
			case "/my/listdevices":
				reply(w, key, rid, map[string]any{"list": []map[string]any{{"id": "autre", "name": "pc"}, {"id": "d1", "name": "truenas", "type": "jd"}}})
			case "/my/disconnect":
				reply(w, key, rid, map[string]any{})
			}
			return
		}
		// Appel appareil : /t_<session>_<appareil>/<chemin>
		if !strings.HasPrefix(r.URL.Path, "/t_"+session+"_d1/") || expireOnce {
			expireOnce = false
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"src":"MYJD","type":"TOKEN_INVALID"}`))
			return
		}
		tok := jdToken(device, session)
		body, _ := io.ReadAll(r.Body)
		plain, err := jdDecrypt(body, tok)
		var req struct {
			URL string `json:"url"`
			Rid int64  `json:"rid"`
		}
		if err != nil || json.Unmarshal(plain, &req) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var data any
		switch req.URL {
		case "/downloadsV2/queryPackages":
			json.Unmarshal([]byte(`[{"bytesLoaded":1500000000,"bytesTotal":1500000000,"childCount":3,"enabled":true,"finished":true,"name":"A","uuid":1},
				{"bytesLoaded":0,"childCount":1,"enabled":true,"name":"B","uuid":2},
				{"bytesLoaded":250000000,"bytesTotal":1000000000,"childCount":2,"enabled":true,"running":true,"speed":5000000,"eta":150,"status":"Téléchargement","name":"C","uuid":3}]`), &data)
		case "/downloadcontroller/getCurrentState":
			data = "RUNNING"
		case "/downloadcontroller/getSpeedInBps":
			data = 5000000
		case "/linkgrabberv2/queryPackages":
			json.Unmarshal([]byte(`[{"childCount":1,"name":"X","uuid":1790629405634},{"childCount":4,"name":"Y","uuid":1790629405635}]`), &data)
		}
		reply(w, tok, req.Rid, map[string]any{"data": data})
	}))
	defer srv.Close()

	src, err := newJDownloader(config.Tile{URL: srv.URL, Options: map[string]string{"device": "truenas"}, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		return map[string]string{"user": "Admin@Example.com", "pass": "factice"}[role], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	j := src.(*jdown)
	v, err := j.poll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"state": "en cours", "speed": "5 Mo/s", "active": "1", "remaining": "750 Mo", "grabber": "5 liens", "total": "3 paquets"}
	for k, w := range want {
		if v[k].Text != w {
			t.Errorf("%s = %q, attendu %q", k, v[k].Text, w)
		}
	}
	rows := v["packages"].Rows
	if len(rows) != 2 || rows[0][2].Text != "?" || rows[1][1].Text != "25 %" || rows[1][4].Text != "2 min" {
		t.Errorf("paquets : %+v", rows)
	}

	// Jeton expiré côté MyJD : une nouvelle session, sondage relancé.
	mu.Lock()
	expireOnce = true
	mu.Unlock()
	if _, err := j.poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Sondage normal : aucune requête de session.
	if _, err := j.poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if connects != 2 {
		t.Errorf("connect %d, attendu 2", connects)
	}
	j.disconnect()
	if j.session != "" {
		t.Error("session non fermée")
	}

	// Mauvais mot de passe : l'erreur ne contient ni secret ni jeton.
	bad, _ := newJDownloader(config.Tile{URL: srv.URL, Options: map[string]string{"device": "truenas"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin@example.com", "pass": "faux"}[role], nil
	})
	if _, err := bad.(*jdown).poll(t.Context()); err == nil || strings.Contains(err.Error(), "faux") || strings.Contains(err.Error(), "signature") {
		t.Errorf("mauvais mot de passe : %v", err)
	}
}
