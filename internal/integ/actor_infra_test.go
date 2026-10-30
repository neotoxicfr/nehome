package integ

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// dkWrite : faux Docker qui accepte les redémarrages (POST) et renvoie le
// reste au faux proxy en lecture.
func dkWrite(t *testing.T, refuse bool) (*httptest.Server, *[]string) {
	ro := dkFake(t)
	t.Cleanup(ro.Close)
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			got = append(got, r.URL.RequestURI())
			mu.Unlock()
			switch {
			case refuse:
				w.WriteHeader(http.StatusForbidden)
			case r.URL.Path == "/containers/absent/restart":
				http.NotFound(w, r)
			default:
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		resp, err := http.Get(ro.URL + r.URL.RequestURI())
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestDockerRestart(t *testing.T) {
	srv, got := dkWrite(t, false)
	src, _ := newDocker(config.Tile{URL: srv.URL, Options: map[string]string{"protect": "nehome, jackett"}}, nil)
	d := src.(*docker)
	if len(d.Actions()) != 0 {
		t.Fatal("aucun choix avant le premier relevé")
	}
	if _, err := d.snapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	a := d.Actions()
	if len(a) != 1 || a[0].ID != "restart" || a[0].Confirm == "" || !slices.Equal(a[0].Choices, []string{"claude-tools", "cleanuparr", "neuf"}) {
		t.Fatalf("actions : %+v", a)
	}
	msg, err := d.Do(t.Context(), "restart", "cleanuparr")
	if err != nil || msg != "cleanuparr redémarré" {
		t.Fatalf("restart : %q %v", msg, err)
	}
	for _, p := range []string{"nehome", "jackett", ""} {
		if _, err := d.Do(t.Context(), "restart", p); err == nil {
			t.Errorf("%q protégé : redémarrage accepté", p)
		}
	}
	if _, err := d.Do(t.Context(), "restart", "absent"); err == nil || !strings.Contains(err.Error(), "introuvable") {
		t.Errorf("absent : %v", err)
	}
	if want := []string{"/containers/cleanuparr/restart?t=60", "/containers/absent/restart?t=60"}; !slices.Equal(*got, want) {
		t.Errorf("requêtes %v, attendu %v", *got, want)
	}
	// Protection par défaut : nehome.
	src, _ = newDocker(config.Tile{URL: srv.URL}, nil)
	d = src.(*docker)
	d.snapshot(t.Context())
	if c := d.Actions()[0].Choices; slices.Contains(c, "nehome") || !slices.Contains(c, "jackett") {
		t.Errorf("protection par défaut : %v", c)
	}
	// Proxy en lecture seule : message clair.
	ro, _ := dkWrite(t, true)
	if err := newDockerAPI(ro.URL).restart(t.Context(), "x"); err == nil || !strings.Contains(err.Error(), "refuse") {
		t.Errorf("proxy en lecture : %v", err)
	}
}

func TestLinkRestart(t *testing.T) {
	srv, got := dkWrite(t, false)
	mk := func(opts map[string]string) *link {
		opts["docker_url"] = srv.URL
		src, err := newLink(config.Tile{Options: opts}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return src.(*link)
	}
	if a := mk(map[string]string{"container": "nehome"}).Actions(); len(a) != 0 {
		t.Errorf("nehome protégé par défaut : %+v", a)
	}
	if a := mk(map[string]string{}).Actions(); len(a) != 0 {
		t.Errorf("sans conteneur : %+v", a)
	}
	if _, err := mk(map[string]string{"container": "plex", "protect": "plex"}).Do(t.Context(), "restart", ""); err == nil {
		t.Error("conteneur protégé redémarré")
	}
	l := mk(map[string]string{"container": "plex"})
	if a := l.Actions(); len(a) != 1 || a[0].ID != "restart" || a[0].Confirm == "" || len(a[0].Choices) != 0 {
		t.Fatalf("actions : %+v", a)
	}
	if msg, err := l.Do(t.Context(), "restart", ""); err != nil || msg != "plex redémarré" {
		t.Fatalf("restart : %q %v", msg, err)
	}
	if !slices.Equal(*got, []string{"/containers/plex/restart?t=60"}) {
		t.Errorf("requêtes %v", *got)
	}
}

func TestGluetunRestart(t *testing.T) {
	var mu sync.Mutex
	var got []string
	failStop, deny := false, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.URL.Path != "/v1/vpn/status" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("requête %s %s", r.Method, r.URL.Path)
		}
		var body struct{ Status string }
		json.Unmarshal(b, &body)
		got = append(got, body.Status)
		switch {
		case deny:
			w.WriteHeader(http.StatusUnauthorized)
		case failStop && body.Status == "stopped":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Write([]byte(`{"outcome":"` + body.Status + `"}`))
		}
	}))
	defer srv.Close()
	src, _ := newGluetun(config.Tile{URL: srv.URL + "/", Secrets: map[string]string{"key": "s"}}, func(string) (string, error) { return "k", nil })
	g := src.(*gluetun)
	if a := g.Actions(); len(a) != 1 || a[0].ID != "vpnrestart" || a[0].Confirm == "" {
		t.Fatalf("actions : %+v", a)
	}
	if msg, err := g.Do(t.Context(), "vpnrestart", ""); err != nil || msg != "VPN relancé" {
		t.Fatalf("relance : %q %v", msg, err)
	}
	if !slices.Equal(got, []string{"stopped", "running"}) {
		t.Errorf("ordre %v", got)
	}
	failStop = true
	if msg, err := g.Do(t.Context(), "vpnrestart", ""); err != nil || !strings.Contains(msg, "l'arrêt a échoué") {
		t.Errorf("arrêt en échec : %q %v", msg, err)
	}
	deny = true
	if _, err := g.Do(t.Context(), "vpnrestart", ""); err == nil || !strings.Contains(err.Error(), "refuse") {
		t.Errorf("401 : %v", err)
	}
}

// Faux relais MyJD minimal : session, appareil, puis start/pause (une
// expiration de jeton au premier appel appareil).
func TestJDownloaderActions(t *testing.T) {
	login := jdHash([]byte("admin@example.com" + "factice" + "server"))
	device := jdHash([]byte("admin@example.com" + "factice" + "device"))
	session := strings.Repeat("ab", 32)
	var mu sync.Mutex
	var calls []string
	expire := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		reply := func(key []byte, rid int64, m map[string]any) {
			m["rid"] = rid
			b, _ := json.Marshal(m)
			w.Write([]byte(jdEncrypt(b, key)))
		}
		if strings.HasPrefix(r.URL.Path, "/my/") {
			var rid int64
			json.Unmarshal([]byte(r.URL.Query().Get("rid")), &rid)
			switch r.URL.Path {
			case "/my/connect":
				reply(login, rid, map[string]any{"sessiontoken": session})
			case "/my/listdevices":
				reply(jdToken(login, session), rid, map[string]any{"list": []map[string]any{{"id": "d1", "name": "truenas"}}})
			}
			return
		}
		if expire {
			expire = false
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"src":"MYJD","type":"TOKEN_INVALID"}`))
			return
		}
		tok := jdToken(device, session)
		body, _ := io.ReadAll(r.Body)
		plain, err := jdDecrypt(body, tok)
		var req struct {
			URL    string   `json:"url"`
			Rid    int64    `json:"rid"`
			Params []string `json:"params"`
		}
		if err != nil || json.Unmarshal(plain, &req) != nil || r.URL.Path != "/t_"+session+"_d1"+req.URL {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls = append(calls, req.URL+"("+strings.Join(req.Params, ",")+")")
		reply(tok, req.Rid, map[string]any{"data": true})
	}))
	defer srv.Close()
	src, err := newJDownloader(config.Tile{URL: srv.URL, Options: map[string]string{"device": "truenas"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin@example.com", "pass": "factice"}[role], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	j := src.(*jdown)
	if a := j.Actions(); len(a) != 2 || a[0].ID != "start" || a[1].ID != "pause" {
		t.Fatalf("actions : %+v", a)
	}
	if msg, err := j.Do(t.Context(), "start", ""); err != nil || msg == "" {
		t.Fatalf("start : %q %v", msg, err)
	}
	if _, err := j.Do(t.Context(), "pause", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Do(t.Context(), "stop", ""); err == nil {
		t.Error("action inconnue acceptée")
	}
	if want := []string{"/downloadcontroller/start()", "/downloadcontroller/pause(true)"}; !slices.Equal(calls, want) {
		t.Errorf("appels %v, attendu %v", calls, want)
	}
}

// Faux TrueNAS JSON-RPC sur websocket : la session complète tourne, puis les
// actions passent par la connexion en cours.
func TestTrueNASActions(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		for {
			_, b, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var m struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			json.Unmarshal(b, &m)
			var res any
			switch m.Method {
			case "auth.login_with_api_key":
				res = true
			case "system.info":
				res = map[string]any{"version": "25.10"}
			case "pool.query":
				res = []map[string]any{{"name": "tank", "status": "ONLINE", "healthy": true}, {"name": "ssd", "status": "ONLINE", "healthy": true}}
			case "alert.list":
				res = []map[string]any{{"uuid": "a1", "level": "WARNING"}, {"uuid": "a2", "level": "CRITICAL", "dismissed": true}, {"uuid": "a3", "level": "INFO"}}
			case "pool.scrub.run", "alert.dismiss":
				mu.Lock()
				calls = append(calls, m.Method+string(m.Params))
				mu.Unlock()
			}
			if m.Method == "alert.dismiss" && string(m.Params) == `["a3"]` {
				out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"message": "refusé"}})
				c.Write(r.Context(), websocket.MessageText, out)
				continue
			}
			out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": res})
			c.Write(r.Context(), websocket.MessageText, out)
		}
	}))
	defer srv.Close()

	src, _ := newTrueNAS(config.Tile{URL: srv.URL}, func(string) (string, error) { return "cle-factice", nil })
	n := src.(*trueNAS)
	if _, err := n.Do(t.Context(), "dismiss", ""); err == nil || !strings.Contains(err.Error(), "non connecté") {
		t.Fatalf("hors session : %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { n.Run(ctx, func(tile.Values, error) {}); close(done) }()
	defer func() { cancel(); <-done }()

	var acts []tile.Action
	for deadline := time.Now().Add(5 * time.Second); len(acts) < 2 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		acts = n.Actions()
	}
	if len(acts) != 2 || acts[0].ID != "scrub" || !slices.Equal(acts[0].Choices, []string{"tank", "ssd"}) || acts[0].Confirm == "" || acts[1].ID != "dismiss" {
		t.Fatalf("actions : %+v", acts)
	}
	if msg, err := n.Do(t.Context(), "scrub", "tank"); err != nil || msg != "Scrub lancé sur tank" {
		t.Fatalf("scrub : %q %v", msg, err)
	}
	if _, err := n.Do(t.Context(), "scrub", "inconnu"); err == nil {
		t.Error("pool inconnu accepté")
	}
	if _, err := n.Do(t.Context(), "dismiss", ""); err == nil || !strings.Contains(err.Error(), "1 alerte(s) acquittée(s), puis") || strings.Contains(err.Error(), "cle-factice") {
		t.Errorf("dismiss : %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{`pool.scrub.run["tank",0]`, `alert.dismiss["a1"]`, `alert.dismiss["a3"]`}; !slices.Equal(calls, want) {
		t.Errorf("appels %v, attendu %v", calls, want)
	}
}
