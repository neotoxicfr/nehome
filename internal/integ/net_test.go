package integ

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

func netSecrets(m map[string]string) func(string) (string, error) {
	return func(r string) (string, error) { return m[r], nil }
}

func netNeed(t *testing.T, v tile.Values, id string) tile.Value {
	t.Helper()
	x, ok := v[id]
	if !ok {
		t.Fatalf("champ %q absent (%v)", id, v)
	}
	return x
}

func TestNetFieldsZeroValue(t *testing.T) {
	for _, f := range [][]tile.Field{(&adguard{}).Fields(), (&crowdsec{}).Fields(), (&traefik{}).Fields(), (&gluetun{}).Fields(), (&uptimeKuma{}).Fields(), (&cloudflare{}).Fields()} {
		if len(f) == 0 {
			t.Fatal("aucun champ")
		}
	}
}

func TestAdGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/control/status":
			w.Write([]byte(`{"version":"v0.107.79","language":"fr","dns_port":53,"http_port":80,"protection_disabled_duration":0,"start_time":1793086422945.8162,"protection_enabled":true,"dhcp_available":true,"running":true}`))
		case "/control/stats":
			w.Write([]byte(`{"time_units":"hours","top_queried_domains":[{"example.net":35954},{"ghcr.io":150}],"top_clients":[{"::1":20029},{"192.168.1.3":87}],"top_blocked_domains":[{"logs.example.com":2}],"top_upstreams_responses":[{"https://dns.cloudflare.com:443/dns-query":293}],"top_upstreams_avg_time":[{"https://dns.cloudflare.com:443/dns-query":0.0309}],"dns_queries":[228,227,257],"blocked_filtering":[0,1,3],"num_dns_queries":39196,"num_blocked_filtering":4,"num_replaced_safebrowsing":0,"num_replaced_safesearch":0,"num_replaced_parental":0,"avg_processing_time":0.001481}`))
		case "/control/filtering/status":
			w.Write([]byte(`{"filters":[{"url":"https://x/f1.txt","name":"AdGuard DNS filter","last_updated":"2026-10-26T14:55:24+01:00","id":1,"rules_count":180602,"enabled":true},{"url":"https://x/f2.txt","name":"Off","last_updated":"2026-10-26T14:55:24+01:00","id":2,"rules_count":9,"enabled":false}],"whitelist_filters":null,"interval":24,"enabled":true}`))
		}
	}))
	defer srv.Close()
	src, err := newAdGuard(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	a := src.(*adguard)
	v, err := a.live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := netNeed(t, v, "queries").Text; got != "39 196" {
		t.Errorf("queries = %q", got)
	}
	if p := netNeed(t, v, "blocked"); p.Text != "0 %" {
		t.Errorf("blocked = %q", p.Text)
	}
	if netNeed(t, v, "protection").Level != "ok" || netNeed(t, v, "avg").Text != "1,5 ms" {
		t.Errorf("protection/avg : %+v %+v", v["protection"], v["avg"])
	}
	if r := netNeed(t, v, "domains").Rows; len(r) != 2 {
		t.Errorf("domains = %v", r)
	}
	if l := a.lists(context.Background()); netNeed(t, l, "rules").Text != "180 602" || len(l["lists"].Rows) != 1 {
		t.Errorf("lists = %+v", l)
	}
	a.pass = "mauvais"
	if _, err := a.live(context.Background()); err == nil {
		t.Error("401 attendu")
	}
}

func TestCrowdSec(t *testing.T) {
	logins := 0
	now := time.Now().UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/watchers/login" {
			logins++
			w.Write([]byte(`{"code":200,"expire":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `","token":"jeton"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer jeton" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("has_active_decision") == "true" {
			w.Write([]byte(`[{"kind":"crowdsec","scenario":"crowdsecurity/http-probing","created_at":"` + now + `","decisions":[{"duration":"3h27m13s","type":"ban","origin":"crowdsec"}],"source":{"ip":"203.0.113.7","value":"203.0.113.7","cn":"BE"}},{"kind":"capi","scenario":"update : +15000/-0 IPs","created_at":"` + now + `","decisions":[{"duration":"100h","type":"ban"}],"source":{"scope":"crowdsec","value":"update"}}]`))
			return
		}
		w.Write([]byte(`[{"kind":"waf","scenario":"crowdsecurity/vpatch-env-access","created_at":"` + now + `","decisions":null,"source":{"ip":"203.0.113.7","value":"203.0.113.7","cn":"BE","as_name":"GOOGLE-CLOUD-PLATFORM"}},{"kind":"crowdsec","scenario":"crowdsecurity/http-probing","created_at":"` + now + `","decisions":[{"duration":"3h27m13s","type":"ban"}],"source":{"ip":"203.0.113.7","value":"203.0.113.7","cn":"BE"}},{"kind":"capi","scenario":"update : +15000/-0 IPs","created_at":"` + now + `","source":{"value":"update"}}]`))
	}))
	defer srv.Close()
	src, err := newCrowdSec(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	c := src.(*crowdsec)
	v, err := c.poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a := netNeed(t, v, "alerts"); a.Text != "2" {
		t.Errorf("alertes locales = %q (capi exclu)", a.Text)
	}
	if netNeed(t, v, "appsec").Text != "1" || netNeed(t, v, "decisions").Text != "1" {
		t.Errorf("appsec/decisions : %+v %+v", v["appsec"], v["decisions"])
	}
	if r := netNeed(t, v, "countries").Rows; len(r) != 1 || r[0][0].Text != "BE" || r[0][1].Text != "2" {
		t.Errorf("pays = %v", r)
	}
	if _, err := c.poll(context.Background()); err != nil || logins != 1 {
		t.Errorf("le jeton doit être réutilisé : logins=%d err=%v", logins, err)
	}
}

func TestTraefik(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/overview":
			w.Write([]byte(`{"http":{"routers":{"total":45,"warnings":0,"errors":1},"services":{"total":45,"warnings":0,"errors":0},"middlewares":{"total":24,"warnings":0,"errors":0}},"certificates":{"total":1,"warnings":0,"errors":0},"features":{"tracing":"","metrics":"","accessLog":true},"providers":["Docker","File"]}`))
		case "/api/version":
			w.Write([]byte(`{"Version":"3.7.16","Codename":"langres","startDate":"2026-10-27T08:34:18.416906244+01:00","dashboardName":"truenas"}`))
		case "/api/http/routers":
			w.Write([]byte(`[{"service":"a","rule":"Host(` + "`a.example.net`" + `)","status":"enabled","name":"a@docker"},{"service":"b","rule":"Host(` + "`b.example.net`" + `)","status":"disabled","error":["service \"b\" does not exist"],"name":"b@docker"}]`))
		case "/api/http/services":
			w.Write([]byte(`[{"name":"a@docker","serverStatus":{"http://10.0.0.6:80":"UP"}},{"name":"c@file","serverStatus":{"http://192.168.1.1:5380":"DOWN"}}]`))
		case "/api/entrypoints":
			w.Write([]byte(`[{"address":":80","http":{"redirections":{"entryPoint":{"to":"https","scheme":"https"}}},"name":"http"},{"address":":443","http":{"middlewares":["crowdsec@file"]},"name":"https"}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	src, _ := newTraefik(config.Tile{URL: srv.URL}, nil)
	v, err := src.(*traefik).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h := netNeed(t, v, "health"); h.Level != "crit" || !strings.Contains(h.Sub, "1 routeur") || !strings.Contains(h.Sub, "1 serveur") {
		t.Errorf("health = %+v", h)
	}
	if r := netNeed(t, v, "badrouters").Rows; len(r) != 1 || !strings.Contains(r[0][2].Text, "does not exist") {
		t.Errorf("badrouters = %v", r)
	}
	if netNeed(t, v, "version").Text != "3.7.16" || len(netNeed(t, v, "entrypoints").Rows) != 2 {
		t.Errorf("version/entrypoints : %+v", v)
	}
}

func TestGluetun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/vpn/status":
			w.Write([]byte(`{"status":"running"}`))
		case "/v1/publicip/ip":
			w.Write([]byte(`{"public_ip":"203.0.113.9","region":"Ontario","country":"Canada","city":"Toronto","hostname":"static.example.com","location":"43.7,-79.4","organization":"AS11878 tzulo, inc.","postal_code":"M5A","timezone":"America/Toronto"}`))
		case "/v1/portforward":
			w.Write([]byte(`{"port":0,"ports":[]}`))
		case "/v1/dns/status":
			w.Write([]byte(`{"status":"running"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	src, _ := newGluetun(config.Tile{URL: srv.URL}, nil)
	v, err := src.(*gluetun).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "vpn").Level != "ok" || netNeed(t, v, "ip").Text != "203.0.113.9" || netNeed(t, v, "location").Text != "Toronto, Canada" || netNeed(t, v, "port").Text != "aucun" {
		t.Errorf("v = %+v", v)
	}
}

func TestUptimeKuma(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/plex":
			w.Write([]byte(`{"config":{"slug":"plex"},"incidents":[],"publicGroupList":[{"id":2,"name":"Services","monitorList":[{"id":15,"name":"Plex","type":"docker"},{"id":14,"name":"Seerr","type":"docker"}]}],"maintenanceList":[]}`))
		case "/api/status-page/heartbeat/plex":
			w.Write([]byte(`{"heartbeatList":{"15":[{"status":1,"time":"2026-10-27 14:23:48.753","msg":"","ping":null}],"14":[{"status":0,"time":"x","msg":"","ping":12}]},"uptimeList":{"15_24":1,"14_24":0.5}}`))
		case "/metrics":
			if _, p, _ := r.BasicAuth(); p != "factice" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte("# HELP monitor_status x\nmonitor_status{monitor_name=\"Plex\",monitor_type=\"docker\"} 1\nmonitor_response_time{monitor_name=\"Plex\",monitor_type=\"docker\"} 7\nmonitor_status{monitor_name=\"Autre\",monitor_type=\"http\"} 0\n"))
		}
	}))
	defer srv.Close()
	src, err := newUptimeKuma(config.Tile{URL: srv.URL, Secrets: map[string]string{"key": "k"}}, netSecrets(map[string]string{"key": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*uptimeKuma).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s := netNeed(t, v, "summary"); s.Text != "1 en panne" || s.Level != "crit" {
		t.Errorf("summary = %+v", s)
	}
	if r := netNeed(t, v, "monitors").Rows; r[0][0].Text != "Seerr" {
		t.Errorf("la panne doit passer en tête : %v", r)
	}
	if netNeed(t, v, "uptime").Text != "75 %" || len(netNeed(t, v, "metrics").Rows) != 2 {
		t.Errorf("uptime/metrics : %+v", v)
	}
	// Sans clé : pas de /metrics, la tuile marche quand même.
	s2, _ := newUptimeKuma(config.Tile{URL: srv.URL}, netSecrets(nil))
	if v2, err := s2.(*uptimeKuma).poll(context.Background()); err != nil || v2["metrics"].Rows != nil {
		t.Errorf("sans clé : %v %v", err, v2["metrics"])
	}
}

// Réponse conforme à la documentation officielle (non testée contre le vrai service).
func TestCloudflare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer factice" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/zones":
			w.Write([]byte(`{"result":[{"id":"zone123"}],"success":true}`))
		case "/graphql":
			w.Write([]byte(`{"data":{"viewer":{"zones":[{"httpRequests1hGroups":[{"dimensions":{"datetime":"2026-10-27T10:00:00Z"},"sum":{"requests":900,"cachedRequests":300,"threats":2,"bytes":5000000,"cachedBytes":1000000}},{"dimensions":{"datetime":"2026-10-27T11:00:00Z"},"sum":{"requests":100,"cachedRequests":0,"threats":0,"bytes":1000000,"cachedBytes":0}}],"firewallEventsAdaptive":[{"action":"block","clientASNDescription":"x","clientCountryName":"CN","clientIP":"203.0.113.5","clientRequestPath":"/wp-login.php","datetime":"` + time.Now().UTC().Format(time.RFC3339) + `","source":"firewallrules","ruleId":"abcdef0123456789"}]}]}},"errors":null}`))
		}
	}))
	defer srv.Close()
	src, err := newCloudflare(config.Tile{URL: srv.URL, Options: map[string]string{"zone_name": "exemple.org"}}, netSecrets(map[string]string{"token": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*cloudflare).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "requests").Text != "1 000" || netNeed(t, v, "cache").Text != "30 %" || netNeed(t, v, "threats").Level != "warn" {
		t.Errorf("v = %+v", v)
	}
	if r := netNeed(t, v, "firewall").Rows; len(r) != 1 || r[0][1].Text != "block" || r[0][3].Text != "firewallrules abcdef01" {
		t.Errorf("firewall = %v", r)
	}
}
