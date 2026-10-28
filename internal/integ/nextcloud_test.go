package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponse réelle de serverinfo (NC 35, skipApps=false), allégée ; seule la
// liste app_updates est inventée (elle valait [] ce jour-là).
const ncFixture = `{"ocs":{"meta":{"status":"ok","statuscode":200,"message":"OK"},"data":{"nextcloud":{"system":{"version":"35.0.2.2","theme":"x","enable_avatars":"yes","enable_previews":"yes","memcache.local":"\\OC\\Memcache\\APCu","memcache.distributed":"\\OC\\Memcache\\Redis","filelocking.enabled":"yes","memcache.locking":"\\OC\\Memcache\\Redis","debug":"no","freespace":2264326471680,"cpuload":[2.384765625,2.8623046875,3.20654296875],"cpunum":8,"mem_total":4194304,"mem_free":3949568,"swap_total":4194304,"swap_free":4194304,"apps":{"num_installed":34,"num_updates_available":1,"app_updates":{"calendar":"6.1.0"}},"update":{"lastupdatedat":0,"available":false}},"storage":{"num_users":3,"num_disabled_users":0,"num_files":46804,"num_storages":7,"num_storages_local":4,"num_storages_home":3,"num_storages_other":0,"size_appdata_storage":51629384,"num_files_appdata":1288},"shares":{"num_shares":15,"num_shares_user":0,"num_shares_groups":0,"num_shares_link":15,"num_shares_mail":0,"num_shares_room":0,"num_shares_link_no_password":14,"num_fed_shares_sent":0,"num_fed_shares_received":0,"permissions_3_17":12,"permissions_3_19":1,"permissions_3_21":2}},"server":{"webserver":"nginx/1.31.8","php":{"version":"8.5.12","memory_limit":2147483648,"max_execution_time":3600,"upload_max_filesize":107374182400,"opcache_revalidate_freq":60,"opcache":{"opcache_enabled":true,"cache_full":false,"restart_pending":false,"restart_in_progress":false,"memory_usage":{"used_memory":74746368,"free_memory":193682456,"wasted_memory":6632,"current_wasted_percentage":0.0024706125259399414},"opcache_statistics":{"num_cached_scripts":2158,"num_cached_keys":4163,"max_cached_keys":32531,"hits":5582199,"start_time":1793007714,"last_restart_time":0,"oom_restarts":0,"hash_restarts":0,"manual_restarts":0,"misses":2183,"blacklist_misses":0,"blacklist_miss_ratio":0,"opcache_hit_rate":99.960908834675}},"apcu":{"cache":{"num_slots":16411,"ttl":0,"num_hits":324232,"num_misses":8767,"num_inserts":26013,"num_entries":2985,"cleanups":0,"defragmentations":0,"expunges":0,"start_time":1793007714,"mem_size":1185592,"memory_type":"mmap"},"sma":{"num_seg":1,"seg_size":33554312,"avail_mem":32132760}}},"fpm":{"pool":"www","process-manager":"dynamic","start-time":"26 octobre 2026, 13:41:54","start-since":103282,"accepted-conn":11906,"listen-queue":0,"max-listen-queue":0,"listen-queue-len":0,"idle-processes":11,"active-processes":1,"total-processes":12,"max-active-processes":9,"max-children-reached":0,"slow-requests":0,"memory-peak":14913536},"database":{"type":"mysql","version":"12.3.3","size":176701440}},"activeUsers":{"last5minutes":1,"last1hour":1,"last24hours":1,"last7days":3,"last1month":3,"last3months":3,"last6months":3,"lastyear":3}}}}`

func TestNextcloud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "admin" || p != "factice" || r.Header.Get("OCS-APIRequest") != "true" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/ocs/v2.php/apps/serverinfo/api/v1/info" || r.URL.Query().Get("format") != "json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := ncFixture
		if r.URL.Query().Get("skipApps") != "false" { // comme le serveur : ni apps ni update
			body = strings.Replace(body, `"apps":{"num_installed":34,"num_updates_available":1,"app_updates":{"calendar":"6.1.0"}},`, "", 1)
			body = strings.Replace(body, `"update":{"lastupdatedat":0,"available":false},`, "", 1)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	src, err := newNextcloud(config.Tile{URL: srv.URL + "/", Secrets: map[string]string{"user": "u", "pass": "p"}}, func(role string) (string, error) {
		return map[string]string{"user": "admin", "pass": "factice"}[role], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	n := src.(*nextcloud)
	i, err := n.get(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if i.Nextcloud.System.Apps != nil {
		t.Fatal("la fixture par défaut ne devait pas contenir apps")
	}
	v := ncValues(i)
	want := map[string]string{"active": "1", "files": "46804", "free": "2,3 To", "users": "3", "shares": "15", "version": "35.0.2.2", "php": "8.5.12", "db": "177 Mo", "opcache": "succès 100 %", "storages": "7"}
	for k, w := range want {
		if v[k].Text != w {
			t.Errorf("%s = %q, attendu %q", k, v[k].Text, w)
		}
	}
	if v["caches"].Text != "local APCu · distribué Redis" || v["fpm"].Level != "ok" || v["ram"].Pct <= 0 || v["apcu"].Pct <= 0 {
		t.Errorf("caches/fpm/ram/apcu : %+v %+v %+v %+v", v["caches"], v["fpm"], v["ram"], v["apcu"])
	}
	if _, ok := v["updates"]; ok {
		t.Error("updates ne vient que de l'appel complet")
	}

	i, err = n.get(t.Context(), "&skipApps=false&skipUpdate=false")
	if err != nil {
		t.Fatal(err)
	}
	u := ncUpdates(i)
	if u["updates"].Text != "1 appli" || u["updates"].Level != "warn" || len(u["app_updates"].Rows) != 1 || u["app_updates"].Rows[0][0].Text != "calendar" {
		t.Errorf("mises à jour : %+v %+v", u["updates"], u["app_updates"])
	}

	// Liste vide renvoyée sous forme de tableau JSON.
	i.Nextcloud.System.Apps.Updates = []byte("[]")
	if u := ncUpdates(i); u["updates"].Text != "à jour" || len(u["app_updates"].Rows) != 0 {
		t.Errorf("à jour : %+v", u["updates"])
	}

	n.pass = "faux"
	if _, err := n.get(t.Context(), ""); err == nil || strings.Contains(err.Error(), "faux") {
		t.Errorf("erreur attendue sans secret : %v", err)
	}
}
