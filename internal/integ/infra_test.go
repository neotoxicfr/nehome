package integ

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Les réponses ci-dessous suivent la documentation de chaque API (Proxmox VE
// API viewer, Portainer API, Glances OpenAPI, Netdata v1, Pi-hole v6 /api/docs,
// Home Assistant REST, Synology DSM Login Web API Guide, OMV RPC, OPNsense API,
// UniFi API non officielle). Aucune n'a été jouée contre une vraie instance.

// infServer sert un corps par chemin (sans requête) et vérifie l'en-tête voulu.
func infServer(t *testing.T, hdr, want string, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hdr != "" && r.Header.Get(hdr) != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, b)
	}))
}

func infTile(url string, opts map[string]string) config.Tile {
	return config.Tile{URL: url, Options: opts}
}

func TestInfraFieldsZeroValue(t *testing.T) {
	for _, f := range [][]tile.Field{(&proxmox{}).Fields(), (&portainer{}).Fields(), (&glances{}).Fields(), (&netdata{}).Fields(), (&pihole{}).Fields(),
		(&homeassistant{}).Fields(), (&synology{}).Fields(), (&omv{}).Fields(), (&opnsense{}).Fields(), (&unifi{}).Fields()} {
		if len(f) < 5 {
			t.Fatal("trop peu de champs")
		}
	}
}

func TestProxmox(t *testing.T) {
	tok := "PVEAPIToken=nehome@pve!ro=factice"
	srv := infServer(t, "Authorization", tok, map[string]string{
		"/api2/json/cluster/resources": `{"data":[
 {"id":"node/pve","type":"node","node":"pve","status":"online","cpu":0.25,"maxcpu":8,"mem":4000000000,"maxmem":16000000000,"disk":50000000000,"maxdisk":100000000000,"uptime":90000},
 {"id":"qemu/100","type":"qemu","node":"pve","vmid":100,"name":"vm1","status":"running","cpu":0.1,"mem":1000000000,"maxmem":2000000000,"uptime":100},
 {"id":"qemu/101","type":"qemu","node":"pve","vmid":101,"name":"vm2","status":"stopped","maxmem":2000000000},
 {"id":"lxc/200","type":"lxc","node":"pve","vmid":200,"name":"ct1","status":"running","cpu":0,"mem":100000000,"maxmem":512000000},
 {"id":"node/pve2","type":"node","node":"pve2","status":"offline"},
 {"id":"storage/pve/local","type":"storage","node":"pve","storage":"local","status":"available","disk":30000000000,"maxdisk":100000000000},
 {"id":"storage/pve2/local","type":"storage","node":"pve2","storage":"local","status":"available","disk":10000000000,"maxdisk":100000000000},
 {"id":"storage/pve/nfs","type":"storage","node":"pve","storage":"nfs","shared":1,"status":"available","disk":50000000000,"maxdisk":100000000000},
 {"id":"storage/pve2/nfs","type":"storage","node":"pve2","storage":"nfs","shared":1,"status":"available","disk":50000000000,"maxdisk":100000000000}]}`,
		"/api2/json/cluster/tasks": `{"data":[{"node":"pve","type":"vzdump","id":"100","status":"OK","endtime":1793000000},{"node":"pve","type":"qmstart","id":"101","status":"command 'x' failed: exit code 1","endtime":1793000100}]}`,
	})
	defer srv.Close()
	src, err := newProxmox(infTile(srv.URL, nil), netSecrets(map[string]string{"token": "nehome@pve!ro=factice"}))
	if err != nil {
		t.Fatal(err)
	}
	p := src.(*proxmox)
	v, err := p.resources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g := netNeed(t, v, "guests"); g.Text != "2 / 3" || !strings.Contains(g.Sub, "1 VM") {
		t.Errorf("guests = %+v", g)
	}
	if c := netNeed(t, v, "cpu"); c.Text != "25 %" {
		t.Errorf("cpu = %+v", c)
	}
	if b := netNeed(t, v, "storages").Bars; len(b) != 3 || b[0].Pct != 30 || b[1].Label != "local (pve2)" || b[2].Label != "nfs" {
		t.Errorf("storages = %+v", b)
	}
	tv := p.tasks(context.Background())
	if netNeed(t, tv, "failed").Text != "1" || len(tv["failedtasks"].Rows) != 1 {
		t.Errorf("tasks = %+v", tv)
	}
	p.token = "mauvais"
	if _, err := p.resources(context.Background()); err == nil {
		t.Error("401 attendu")
	}
}

func TestPortainer(t *testing.T) {
	srv := infServer(t, "X-API-Key", "factice", map[string]string{
		"/api/endpoints":     `[{"Id":1,"Name":"local","Type":1,"Status":1,"Snapshots":[{"RunningContainerCount":5,"StoppedContainerCount":2,"HealthyContainerCount":4,"UnhealthyContainerCount":1,"StackCount":3}]}]`,
		"/api/stacks":        `[{"Id":1,"Name":"web","Type":2,"EndpointId":1,"Status":1},{"Id":2,"Name":"old","Type":2,"EndpointId":1,"Status":2}]`,
		"/api/system/status": `{"Version":"2.21.0","InstanceID":"x"}`,
	})
	defer srv.Close()
	src, _ := newPortainer(infTile(srv.URL, nil), netSecrets(map[string]string{"key": "factice"}))
	v, err := src.(*portainer).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "running").Text != "5" || netNeed(t, v, "unhealthy").Level != "crit" || netNeed(t, v, "stacks").Text != "1" || netNeed(t, v, "version").Text != "2.21.0" {
		t.Errorf("valeurs : %+v", v)
	}
}

func TestGlancesRepliV3(t *testing.T) {
	srv := infServer(t, "", "", map[string]string{ // pas de /api/4 : repli v3
		"/api/3/cpu":     `{"total":12.5,"user":8}`,
		"/api/3/mem":     `{"percent":40,"used":4000000000,"total":10000000000}`,
		"/api/3/load":    `{"min1":0.5,"min5":0.4,"min15":0.3,"cpucore":4}`,
		"/api/3/network": `[{"interface_name":"eth0","bytes_recv_rate_per_sec":1000,"bytes_sent_rate_per_sec":500},{"interface_name":"lo"}]`,
	})
	defer srv.Close()
	src, _ := newGlances(infTile(srv.URL, nil), netSecrets(nil))
	g := src.(*glances)
	v, err := g.fast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g.api != "/api/3" || netNeed(t, v, "cpu").Text != "12,5 %" || netNeed(t, v, "ram").Pct != 40 || len(v["net"].Rows) != 1 {
		t.Errorf("api=%s v=%+v", g.api, v)
	}
}

func TestGlancesLent(t *testing.T) {
	srv := infServer(t, "", "", map[string]string{
		"/api/4/memswap":     `{"percent":10,"used":100,"total":1000}`,
		"/api/4/fs":          `[{"mnt_point":"/","percent":55,"used":55000000000,"size":100000000000}]`,
		"/api/4/sensors":     `[{"label":"Core 0","value":54,"unit":"C","type":"temperature_core"},{"label":"fan","value":900,"unit":"R","type":"fan_speed"}]`,
		"/api/4/processlist": `[{"name":"a","cpu_percent":1,"memory_percent":2},{"name":"b","cpu_percent":9,"memory_percent":1}]`,
	})
	defer srv.Close()
	g := &glances{url: srv.URL, hc: srv.Client(), api: "/api/4"}
	v := g.slow(context.Background())
	if netNeed(t, v, "temp").Text != "54 °C" || netNeed(t, v, "disks").Bars[0].Pct != 55 || v["procs"].Rows[0][0].Text != "b" {
		t.Errorf("v = %+v", v)
	}
}

func TestNetdata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/info":
			io.WriteString(w, `{"version":"v2.1.0","mirrored_hosts":["nas"],"os_name":"Debian","alarms":{"normal":10,"warning":1,"critical":0}}`)
		case r.URL.Path == "/api/v1/data" && r.URL.Query().Get("chart") == "system.cpu":
			io.WriteString(w, `{"labels":["time","user","system","iowait"],"data":[[1793000000,10,5,1]]}`)
		case r.URL.Path == "/api/v1/data":
			io.WriteString(w, `{"labels":["time","free","used","cached","buffers"],"data":[[1793000000,2000,4000,3000,1000]]}`)
		case r.URL.Path == "/api/v1/alarms":
			io.WriteString(w, `{"alarms":{"disk_space":{"name":"disk_space_usage","chart":"disk_space._","status":"WARNING","value":91.2,"units":"%"}}}`)
		}
	}))
	defer srv.Close()
	src, _ := newNetdata(infTile(srv.URL, nil), netSecrets(nil))
	v, err := src.(*netdata).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "cpu").Text != "16 %" || netNeed(t, v, "ram").Text != "40 %" || netNeed(t, v, "alarms").Level != "warn" || len(v["alarmlist"].Rows) != 1 {
		t.Errorf("v = %+v", v)
	}
}

func TestPiHoleV6(t *testing.T) {
	logins := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth" {
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), "factice") {
				io.WriteString(w, `{"session":{"valid":false,"sid":null,"message":"password incorrect"}}`)
				return
			}
			logins++
			io.WriteString(w, `{"session":{"valid":true,"sid":"SID`+string(rune('0'+logins))+`","validity":300}}`)
			return
		}
		if r.Header.Get("X-FTL-SID") != "SID2" { // la première session est refusée : relance du login
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/stats/summary":
			io.WriteString(w, `{"queries":{"total":1000,"blocked":250,"percent_blocked":25,"unique_domains":300,"forwarded":600,"cached":150},"clients":{"active":4,"total":9},"gravity":{"domains_being_blocked":90000,"last_update":1793000000}}`)
		case "/api/dns/blocking":
			io.WriteString(w, `{"blocking":"disabled","timer":120}`)
		case "/api/stats/top_domains":
			io.WriteString(w, `{"domains":[{"domain":"example.net","count":42}]}`)
		case "/api/stats/top_clients":
			io.WriteString(w, `{"clients":[{"ip":"192.168.1.5","name":"","count":7}]}`)
		}
	}))
	defer srv.Close()
	src, _ := newPiHole(infTile(srv.URL, nil), netSecrets(map[string]string{"pass": "factice"}))
	p := src.(*pihole)
	v, err := p.poll6(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "queries").Text != netInt(1000) || netNeed(t, v, "blocked").Text != "25 %" || netNeed(t, v, "blocking").Level != "crit" || v["topclients"].Rows[0][0].Text != "192.168.1.5" {
		t.Errorf("v = %+v", v)
	}
	p.secret, p.sid, p.open = "mauvais", "", false
	if _, err := p.poll6(context.Background()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Errorf("login refusé attendu, sans secret : %v", err)
	}
}

func TestPiHoleV5(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/admin/api.php" || q.Get("auth") != "factice" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case q.Has("summaryRaw"):
			io.WriteString(w, `{"domains_being_blocked":80000,"dns_queries_today":500,"ads_blocked_today":50,"ads_percentage_today":10,"unique_domains":100,"queries_forwarded":300,"queries_cached":100,"unique_clients":3,"status":"enabled","gravity_last_updated":{"absolute":1793000000}}`)
		case q.Has("topItems"):
			io.WriteString(w, `{"top_queries":{"a.net":5,"b.net":9},"top_ads":{"ads.net":3}}`)
		case q.Has("getQuerySources"):
			io.WriteString(w, `{"top_sources":{"laptop|192.168.1.2":10}}`)
		case q.Has("version"):
			io.WriteString(w, `{"version":"5.18.4"}`)
		}
	}))
	defer srv.Close()
	src, _ := newPiHole(infTile(srv.URL, map[string]string{"api": "5"}), netSecrets(map[string]string{"token": "factice"}))
	p := src.(*pihole)
	v, err := p.poll5(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "queries").Text != "500" || v["topdomains"].Rows[0][0].Text != "b.net" || v["topclients"].Rows[0][0].Text != "laptop" {
		t.Errorf("v = %+v", v)
	}
	p.secret = "mauvais"
	_, err = p.poll5(context.Background())
	if err == nil || strings.Contains(err.Error(), "mauvais") || strings.Contains(err.Error(), "auth=") {
		t.Errorf("erreur attendue sans jeton : %v", err)
	}
}

func TestHomeAssistant(t *testing.T) {
	srv := infServer(t, "Authorization", "Bearer factice", map[string]string{
		"/api/config": `{"version":"2026.10.1","state":"RUNNING","location_name":"Maison"}`,
		"/api/states": `[{"entity_id":"sensor.temp","state":"21.5","attributes":{"unit_of_measurement":"°C","friendly_name":"Salon"}},
 {"entity_id":"light.lampe","state":"unavailable","attributes":{"friendly_name":"Lampe"}},
 {"entity_id":"sensor.hum","state":"40","attributes":{"unit_of_measurement":"%"}}]`,
	})
	defer srv.Close()
	src, _ := newHomeAssistant(infTile(srv.URL, map[string]string{"entities": "sensor.temp, light.lampe,sensor.absent"}), netSecrets(map[string]string{"token": "factice"}))
	v, err := src.(*homeassistant).poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w := netNeed(t, v, "watched").Rows
	if netNeed(t, v, "entities").Text != "3" || netNeed(t, v, "unavailable").Text != "1" || w[0][1].Text != "21,5 °C" && w[0][1].Text != "21.5 °C" || w[1][1].Text != "indisponible" || w[2][1].Text != "introuvable" {
		t.Errorf("v = %+v", v)
	}
	if netNeed(t, v, "version").Text != "2026.10.1" {
		t.Errorf("version = %+v", v["version"])
	}
}

func TestSynology(t *testing.T) {
	sessions := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if strings.Contains(r.URL.RawQuery, "factice") {
			t.Error("identifiant dans l'URL")
		}
		if r.URL.Path == "/webapi/auth.cgi" {
			if r.Form.Get("passwd") != "factice" {
				io.WriteString(w, `{"success":false,"error":{"code":400}}`)
				return
			}
			sessions++
			io.WriteString(w, `{"success":true,"data":{"sid":"S`+string(rune('0'+sessions))+`"}}`)
			return
		}
		if r.Form.Get("_sid") != "S2" { // première session « expirée » : code 119
			io.WriteString(w, `{"success":false,"error":{"code":119}}`)
			return
		}
		switch r.Form.Get("api") {
		case "SYNO.Core.System.Utilization":
			io.WriteString(w, `{"success":true,"data":{"cpu":{"user_load":10,"system_load":5,"other_load":1,"1min_load":50,"5min_load":40,"15min_load":30},"memory":{"real_usage":35,"total_real":8000000,"avail_real":4000000},"network":[{"device":"total","rx":2000,"tx":1000}]}}`)
		case "SYNO.Core.System":
			io.WriteString(w, `{"success":true,"data":{"model":"DS920+","firmware_ver":"DSM 7.2.1-69057","sys_temp":47,"temperature_warning":false,"up_time":"3600"}}`)
		case "SYNO.Storage.CGI.Storage":
			io.WriteString(w, `{"success":true,"data":{"volumes":[{"id":"volume_1","vol_path":"/volume1","status":"normal","size":{"total":"1000000000000","used":"400000000000"}}],"disks":[{"name":"Disque 1","model":"WD40 ","temp":35,"status":"normal","smart_status":"normal"}]}}`)
		}
	}))
	defer srv.Close()
	src, _ := newSynology(infTile(srv.URL, nil), netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
	s := src.(*synology)
	v, err := s.live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "cpu").Text != "16 %" || netNeed(t, v, "ram").Text != "35 %" || sessions != 2 {
		t.Errorf("v = %+v sessions=%d", v, sessions)
	}
	sv := s.slow(context.Background())
	if netNeed(t, sv, "volumes").Bars[0].Pct != 40 || netNeed(t, sv, "temp").Text != "47 °C" || netNeed(t, sv, "uptime").Text != "1 h 0 min" {
		t.Errorf("slow = %+v", sv)
	}
	s.pass, s.sid = "mauvais", ""
	if _, err := s.live(context.Background()); err == nil {
		t.Error("login refusé attendu")
	}
}

func TestOpenMediaVault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s := string(b)
		switch {
		case strings.Contains(s, `"login"`):
			if !strings.Contains(s, "factice") {
				io.WriteString(w, `{"response":{"authenticated":false},"error":null}`)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "X-OPENMEDIAVAULT-SESSIONID", Value: "abc"})
			io.WriteString(w, `{"response":{"authenticated":true,"username":"admin"},"error":null}`)
		case r.Header.Get("Cookie") == "":
			io.WriteString(w, `{"response":null,"error":{"code":5001,"message":"Session not authenticated."}}`)
		case strings.Contains(s, "getInformation"):
			io.WriteString(w, `{"response":{"hostname":"omv","version":"7.4.2-1","cpuUtilization":12.5,"memUtilization":0.3,"memUsed":3000000000,"memTotal":10000000000,"uptime":7200,"loadAverage":{"1min":0.1,"5min":0.2,"15min":0.3},"availablePkgUpdates":4,"rebootRequired":false},"error":null}`)
		case strings.Contains(s, "enumerateFilesystems"):
			io.WriteString(w, `{"response":[{"devicefile":"/dev/sda1","label":"data","mountpoint":"/srv/dev-disk-by-uuid-x","type":"ext4","mounted":true,"size":"1000000000","used":"250000000","percentage":25},{"devicefile":"/dev/sdb1","mounted":false}],"error":null}`)
		case strings.Contains(s, "getList"):
			io.WriteString(w, `{"response":{"total":2,"data":[{"devicefile":"/dev/sda","model":"WD","temperature":34,"overallstatus":"GOOD"},{"devicefile":"/dev/sdb","model":"ST","temperature":0,"overallstatus":"BAD_STATUS"}]},"error":null}`)
		}
	}))
	defer srv.Close()
	src, _ := newOMV(infTile(srv.URL, nil), netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
	o := src.(*omv)
	v, err := o.live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if netNeed(t, v, "cpu").Text != "12,5 %" || netNeed(t, v, "updates").Text != "4" || netNeed(t, v, "uptime").Text != "2 h 0 min" || netNeed(t, v, "ram").Pct != 30 {
		t.Errorf("v = %+v", v)
	}
	sv := o.slow(context.Background())
	if len(netNeed(t, sv, "fs").Bars) != 1 || netNeed(t, sv, "smart").Level != "crit" || !strings.Contains(fmt.Sprintf("%+v", netNeed(t, sv, "disks")), "WD") {
		t.Errorf("slow = %+v", sv)
	}
	o.pass, o.logged = "mauvais", false
	if _, err := o.live(context.Background()); err == nil {
		t.Error("login refusé attendu")
	}
}

func TestOPNsense(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "cle" || p != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/routes/gateway/status":
			io.WriteString(w, `{"items":[{"name":"WAN_GW","address":"203.0.113.1","status":"none","status_translated":"Online","loss":"0.0 %","delay":"3.1 ms","stddev":"0.2 ms"},{"name":"WAN2","address":"198.51.100.1","status":"down","loss":"100.0 %","delay":"~"}],"status":"ok"}`)
		case "/api/diagnostics/system/systemTime":
			io.WriteString(w, `{"uptime":"3 days, 02:03:04","loadavg":"0.10, 0.20, 0.30","datetime":"x"}`)
		case "/api/diagnostics/system/systemResources":
			io.WriteString(w, `{"memory":{"total":"8000000000","total_frmt":"7629","used":"2000000000","used_frmt":"1907"}}`)
		case "/api/diagnostics/system/systemDisk":
			io.WriteString(w, `{"devices":[{"device":"/dev/ada0p3","type":"ufs","blocks":"50G","used":"4.2G","available":"43G","used_pct":9,"mountpoint":"/"}]}`)
		case "/api/interfaces/overview/interfacesInfo":
			io.WriteString(w, `{"rows":[{"device":"igb0","description":"WAN","status":"up","addr4":"203.0.113.2/24"}]}`)
		case "/api/core/firmware/status":
			io.WriteString(w, `{"status":"update","status_msg":"There are 3 updates available","needs_reboot":"0","product":{"product_version":"24.7.6"},"new_packages":[],"upgrade_packages":[{"name":"a"},{"name":"b"},{"name":"c"}]}`)
		}
	}))
	defer srv.Close()
	src, _ := newOPNsense(infTile(srv.URL, nil), netSecrets(map[string]string{"key": "cle", "secret": "factice"}))
	o := src.(*opnsense)
	v, err := o.live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g := netNeed(t, v, "gateways"); g.Text != "1 / 2" || g.Level != "crit" {
		t.Errorf("gateways = %+v", g)
	}
	if netNeed(t, v, "ram").Pct != 25 || netNeed(t, v, "load").Text != "0.10" || len(v["ifaces"].Rows) != 1 {
		t.Errorf("v = %+v", v)
	}
	fw := o.firmware(context.Background())
	if netNeed(t, fw, "firmware").Text != "3 paquets" || netNeed(t, fw, "version").Text != "24.7.6" {
		t.Errorf("fw = %+v", fw)
	}
	o.sec = "mauvais"
	if _, err := o.live(context.Background()); err == nil {
		t.Error("401 attendu")
	}
}

func unifiHandler(prefix, loginPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "login") && r.URL.Path != loginPath {
			w.WriteHeader(http.StatusNotFound) // l'autre mode de connexion n'existe pas
			return
		}
		if r.URL.Path == loginPath {
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), "factice") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "TOKEN", Value: "jwt", Path: "/"})
			io.WriteString(w, `{}`)
			return
		}
		if _, err := r.Cookie("TOKEN"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, prefix+"/api/s/default/") {
		case "stat/health":
			io.WriteString(w, `{"meta":{"rc":"ok"},"data":[{"subsystem":"wlan","status":"ok","num_user":8,"num_guest":2,"num_ap":2},{"subsystem":"wan","status":"ok","wan_ip":"203.0.113.2","rx_bytes-r":5000,"tx_bytes-r":1000},{"subsystem":"lan","status":"ok","num_user":3,"num_sw":1}]}`)
		case "stat/device":
			io.WriteString(w, `{"data":[{"name":"AP salon","model":"U6-LR","type":"uap","state":1,"version":"6.6.55","uptime":86400,"num_sta":6},{"name":"Switch","model":"USW","type":"usw","state":0,"version":"6.6.1","uptime":0}]}`)
		case "stat/sta":
			io.WriteString(w, `{"data":[{"hostname":"tv","ip":"192.168.1.20","is_wired":false,"essid":"maison","rx_bytes-r":100,"tx_bytes-r":10},{"name":"nas","ip":"192.168.1.3","is_wired":true,"rx_bytes-r":9000,"tx_bytes-r":9000}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestUniFiOSetClassique(t *testing.T) {
	for _, c := range []struct{ name, prefix, login string }{{"UniFi OS", "/proxy/network", "/api/auth/login"}, {"classique", "", "/api/login"}} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(unifiHandler(c.prefix, c.login))
			defer srv.Close()
			src, _ := newUniFi(infTile(srv.URL, nil), netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
			u := src.(*unifi)
			v, err := u.poll(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if u.prefix != c.prefix || netNeed(t, v, "clients").Text != "10" || netNeed(t, v, "devices").Level != "crit" || netNeed(t, v, "wan").Sub != "203.0.113.2" {
				t.Errorf("prefix=%q v=%+v", u.prefix, v)
			}
			if v["clienttable"].Rows[0][0].Text != "nas" {
				t.Errorf("tri des clients : %+v", v["clienttable"].Rows)
			}
			u.pass, u.logged = "mauvais", false
			if _, err := u.poll(context.Background()); err == nil {
				t.Error("login refusé attendu")
			}
		})
	}
}

// Sans mot de passe, Pi-hole v6 répond valid:true et sid:null ; la session
// ouverte est fermée (DELETE) à la fin de Run.
func TestPiHoleV6SansMotDePasse(t *testing.T) {
	logins, deletes := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/auth" && r.Method == http.MethodPost:
			logins++
			io.WriteString(w, `{"session":{"valid":true,"sid":null,"validity":-1}}`)
		case r.URL.Path == "/api/auth" && r.Method == http.MethodDelete:
			deletes++
		case r.URL.Path == "/api/stats/summary":
			io.WriteString(w, `{"queries":{"total":10},"clients":{},"gravity":{}}`)
		}
	}))
	defer srv.Close()
	src, _ := newPiHole(infTile(srv.URL, nil), netSecrets(map[string]string{"pass": ""}))
	p := src.(*pihole)
	for range 2 {
		if _, err := p.poll6(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if logins != 1 {
		t.Errorf("logins = %d, 1 attendu", logins)
	}
	p.sid = "SIDX"
	p.logout()
	if deletes != 1 {
		t.Errorf("deletes = %d", deletes)
	}
}
