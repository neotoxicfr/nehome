package integ

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

func TestQbitIncremental(t *testing.T) {
	calls, logins := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if r.FormValue("username") != "admin" || r.FormValue("password") != "factice" {
				logins++
				w.Write([]byte("Fails.")) // mauvais mot de passe : HTTP 200
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s", Path: "/"}) // comme qBittorrent
			w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			if c, err := r.Cookie("SID"); err != nil || c.Value != "s" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			calls++
			if r.URL.Query().Get("rid") == "0" {
				w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{"h1":{"name":"A","state":"stalledUP","category":"radarr","tags":"Books, TrackerA","size":100},"h2":{"name":"B","state":"downloading","progress":0.5,"dlspeed":1000,"size":50}},"server_state":{"dl_info_speed":1000,"up_info_speed":2000,"global_ratio":"0.87","connection_status":"connected","free_space_on_disk":5e12}}`))
				return
			}
			// Delta : B passe en partage, A disparaît, seul le débit montant change.
			w.Write([]byte(`{"rid":2,"torrents":{"h2":{"state":"uploading","progress":1}},"torrents_removed":["h1"],"server_state":{"up_info_speed":3000}}`))
		}
	}))
	defer srv.Close()
	src, err := newQbit(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin", "pass": "factice"}[role], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q := src.(*qbit)
	if err := q.sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	v := q.values()
	if v["leech"].Text != "1" || v["seed"].Text != "1" || v["ratio"].Text != "0,87" || v["conn"].Text != "connecté" {
		t.Fatalf("premier état : %+v %+v %+v", v["leech"], v["seed"], v["ratio"])
	}
	if rows := v["tags"].Rows; len(rows) != 2 {
		t.Fatalf("étiquettes : %+v", rows)
	}
	if err := q.sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	v = q.values()
	if v["leech"].Text != "0" || v["seed"].Text != "1" || len(q.torrents) != 1 || v["up"].Text != "3 ko/s" || v["ratio"].Text != "0,87" {
		t.Fatalf("après delta : leech=%s seed=%s n=%d up=%s ratio=%s", v["leech"].Text, v["seed"].Text, len(q.torrents), v["up"].Text, v["ratio"].Text)
	}

	// mauvais mot de passe : refus clair, puis aucun nouvel essai avant 5 min
	bad, _ := newQbit(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin", "pass": "mauvais"}[role], nil
	})
	q = bad.(*qbit)
	for range 2 {
		if err := q.sync(t.Context()); err == nil || !strings.Contains(err.Error(), "identifiants refusés") {
			t.Fatalf("refus attendu : %v", err)
		}
	}
	if logins != 1 {
		t.Fatalf("logins = %d, 1 attendu (délai de 5 min)", logins)
	}
}

func TestMikroTik(t *testing.T) {
	rx := 1000.0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body := map[string]string{
			"/rest/system/resource":                   `{"cpu-load":"12","free-memory":"212848640","total-memory":"1073741824","free-hdd-space":"89812992","total-hdd-space":"134217728","uptime":"2d7h43m41s","version":"7.24.1 (stable)"}`,
			"/rest/system/health":                     `[{"name":"cpu-temperature","value":"56"},{"name":"psu1-voltage","value":"23.6"}]`,
			"/rest/system/package/update":             `{"installed-version":"7.24.1","latest-version":"7.25"}`,
			"/rest/ip/cloud":                          `{"dns-name":"x.sn.mynetname.net","public-address":"192.0.2.1","status":"updated"}`,
			"/rest/ipv6/address":                      `[{"address":"2001:db8::1/64","global":"true","disabled":"false","interface":"bridge","valid":"23h56m29s"}]`,
			"/rest/ip/socks":                          `{"enabled":"true","port":"1080","version":"5","auth-method":"none"}`,
			"/rest/ip/dhcp-server/lease":              `[{"mac-address":"aa:bb:cc:dd:ee:ff","host-name":"pc","active-address":"192.168.88.10","status":"bound","expires-after":"40m"},{"mac-address":"11:22:33:44:55:66","status":"waiting"}]`,
			"/rest/interface/wifi/registration-table": `[{"mac-address":"AA:BB:CC:DD:EE:FF","interface":"wifi1","signal":"-58","tx-rate":"866Mbps","uptime":"1h2m"}]`,
			"/rest/interface/wireguard/peers":         `[{"name":"iphone","last-handshake":"45s","rx":"1000","tx":"2000"},{"comment":"pc","rx":"0","tx":"0"}]`,
			"/rest/log":                               `[{"time":"10:00:00","topics":"system,info,account","message":"user admin logged in"},{"time":"10:01:00","topics":"interface,warning","message":"ether5 link down"}]`,
		}[r.URL.Path]
		if r.URL.Path == "/rest/interface" {
			body = `[{"rx-byte":"` + tile.Num(rx) + `","tx-byte":"0"}]`
			rx += 5000
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	src, err := newMikroTik(config.Tile{URL: srv.URL, Options: map[string]string{"wan": "ether5"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin", "pass": "factice"}[role], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m := src.(*mikrotik)
	v, err := m.slow(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{"cpu": "12 %", "temp": "56 °C", "uptime": "2 j 7 h", "leases": "1", "socks": "actif, port 1080", "wanip": "192.0.2.1"}
	for k, want := range checks {
		if v[k].Text != want {
			t.Errorf("%s = %q, attendu %q", k, v[k].Text, want)
		}
	}
	if v["version"].Level != "warn" || !strings.Contains(v["version"].Sub, "7.25") {
		t.Errorf("mise à jour non signalée : %+v", v["version"])
	}
	if w := v["wifi"].Rows; len(w) != 1 || w[0][0].Text != "pc" || w[0][2].Level != "ok" {
		t.Errorf("wifi : %+v", w)
	}
	if g := v["wg"].Rows; len(g) != 2 || g[0][2].Level != "ok" || g[1][2].Text != "jamais" {
		t.Errorf("wireguard : %+v", g)
	}
	if l := v["logs"].Rows; len(l) != 1 || l[0][1].Level != "warn" {
		t.Errorf("journal filtré : %+v", l)
	}
	if v, _ := m.traffic(t.Context()); v != nil {
		t.Fatal("premier point de débit : rien à afficher")
	}
	m.lastAt = m.lastAt.Add(-5 * time.Second) // 5 s écoulées
	v, err = m.traffic(t.Context())
	if p := v["down"].Points; err != nil || len(p) != 1 || p[0] < 990 || p[0] > 1000 {
		t.Fatalf("débit : %+v %v", v["down"], err)
	}
}

func TestTrueNASRealtime(t *testing.T) {
	n := &trueNAS{cpu: tile.NewRing(60), rx: tile.NewRing(60), tx: tile.NewRing(60)}
	raw := json.RawMessage(`{"msg":"added","collection":"reporting.realtime","fields":{"memory":{"arc_size":34416881544,"physical_memory_total":67147821056,"physical_memory_available":10918010880},"cpu":{"cpu":{"usage":22.0,"temp":44.0},"cpu0":{"usage":19.0,"temp":40.0}},"disks":{"read_bytes":29196162,"write_bytes":19060},"interfaces":{"eno1":{"received_bytes_rate":809022,"sent_bytes_rate":9340995}}}}`)
	v := n.realtime(raw)
	if v["cpu"].Text != "22 %" || v["cputemp"].Text != "44 °C" || v["net_tx"].Text != "9,3 Mo/s" {
		t.Fatalf("temps réel : %+v %+v %+v", v["cpu"], v["cputemp"], v["net_tx"])
	}
	// Mémoire des applications = total − disponible − ARC ≈ 21,8 Go.
	if !strings.HasPrefix(v["ram"].Text, "21,8 Go") || v["ram"].Sub != "ARC 34,4 Go" {
		t.Fatalf("mémoire : %+v", v["ram"])
	}
	if n.realtime(json.RawMessage(`{"collection":"autre"}`)) != nil {
		t.Fatal("collection étrangère prise en compte")
	}
}

func TestMissingSecret(t *testing.T) {
	cfg := &config.Config{SecretsDir: t.TempDir()}
	src := New(cfg, config.Tile{Type: "mikrotik", Secrets: map[string]string{"user": "absent", "pass": "absent"}})
	if len(src.Fields()) == 0 {
		t.Fatal("tuile en panne : ses champs doivent rester réglables")
	}
	if _, err := cfg.Secret("../etc/passwd"); err == nil {
		t.Fatal("chemin de secret hors du dossier accepté")
	}
}
