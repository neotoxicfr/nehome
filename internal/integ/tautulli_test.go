package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestTautulli(t *testing.T) {
	data := map[string]string{
		"get_activity": `{"response":{"result":"success","data":{"stream_count":"3","stream_count_direct_play":2,"stream_count_direct_stream":0,"stream_count_transcode":1,"total_bandwidth":25067,"lan_bandwidth":0,"wan_bandwidth":25067,"sessions":[]}}}`,
		"get_home_stats": `{"response":{"result":"success","data":[
{"stat_id":"popular_movies","rows":[{"title":"Film A","total_plays":4,"users_watched":4}]},
{"stat_id":"top_users","rows":[{"title":"x","friendly_name":"Alice","total_plays":88,"total_duration":153616}]},
{"stat_id":"most_concurrent","rows":[{"title":"Concurrent Streams","count":14},{"title":"Concurrent Transcodes","count":4}]}]}}`,
		"get_plays_by_date": `{"response":{"result":"success","data":{"categories":["a","b"],"series":[{"name":"TV","data":[1,2]},{"name":"Total","data":[60,18]}]}}}`,
		"get_history":       `{"response":{"result":"success","data":{"data":[{"started":1793110925,"friendly_name":"Alice","full_title":"Série - Épisode","percent_complete":64,"transcode_decision":"transcode"}]}}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apikey") != "factice" {
			w.Write([]byte(`{"response":{"result":"error","message":"Invalid apikey","data":{}}}`))
			return
		}
		w.Write([]byte(data[q.Get("cmd")]))
	}))
	defer srv.Close()
	src, err := newTautulli(config.Tile{URL: srv.URL}, mediaSecret)
	if err != nil {
		t.Fatal(err)
	}
	a := src.(*tautulli)
	v, err := a.activity(t.Context())
	if err != nil || v["streams"].Text != "3" || v["bandwidth"].Text != "3,1 Mo/s" || !strings.Contains(v["bandwidth"].Sub, "WAN 3,1 Mo/s · LAN 0 o/s") {
		t.Fatalf("activité : %v %+v", err, v)
	}
	v, err = a.stats(t.Context())
	if err != nil || v["popular_movies"].Rows[0][1].Text != "4" || v["top_users"].Rows[0][2].Text != "1 j 18 h" || v["peak"].Text != "14" || v["peak"].Sub != "dont 4 transcodage(s)" || v["plays"].Text != "18 aujourd'hui" || len(v["plays"].Points) != 2 {
		t.Fatalf("stats : %v %+v", err, v)
	}
	v, err = a.history(t.Context())
	if err != nil || v["history"].Rows[0][3].Text != "Transcodage" || v["history"].Rows[0][2].Text != "64 %" {
		t.Fatalf("historique : %v %+v", err, v)
	}

	// Clé refusée : l'erreur ne doit pas contenir la clé.
	bad, _ := newTautulli(config.Tile{URL: srv.URL}, func(string) (string, error) { return "secretmauvais", nil })
	if _, err := bad.(*tautulli).activity(t.Context()); err == nil || strings.Contains(err.Error(), "secretmauvais") {
		t.Fatalf("erreur attendue sans clé : %v", err)
	}
	// Serveur arrêté : l'erreur réseau ne doit pas non plus contenir la clé.
	srv.Close()
	if _, err := bad.(*tautulli).activity(t.Context()); err == nil || strings.Contains(err.Error(), "secretmauvais") {
		t.Fatalf("erreur réseau avec clé : %v", err)
	}
}
