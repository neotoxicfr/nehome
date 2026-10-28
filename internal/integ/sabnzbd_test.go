package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Réponses construites d'après la documentation de l'API SABnzbd
// (sabnzbd.org/wiki/advanced/api : modes queue, history, server_stats ;
// valeurs numériques en chaînes, erreur de clé en HTTP 200).
func TestSABnzbd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apikey") != "factice" {
			w.Write([]byte(`{"status":false,"error":"API Key Incorrect"}`))
			return
		}
		switch q.Get("mode") {
		case "queue":
			w.Write([]byte(`{"queue":{"version":"4.3.2","status":"Downloading","paused":false,"kbpersec":"2048.0","speedlimit_abs":"","mb":"1000.0","mbleft":"400.0","timeleft":"0:03:20","diskspace1":"200.00","diskspace2":"150.00","have_warnings":"2","noofslots_total":2,
			"slots":[{"filename":"Film.2024","status":"Downloading","percentage":"60","mbleft":"400.0","timeleft":"0:03:20","cat":"movies"},{"filename":"Serie","status":"Queued","percentage":"0","mbleft":"0","timeleft":"0:00:00","cat":"tv"}]}}`))
		case "history":
			w.Write([]byte(`{"history":{"slots":[{"name":"Bon","status":"Completed","bytes":1073741824,"completed":1700000000},{"name":"Mauvais","status":"Failed","bytes":1048576,"completed":1700000100,"fail_message":"too many missing"}]}}`))
		case "server_stats":
			w.Write([]byte(`{"total":5000000000000,"month":100000000000,"week":30000000000,"day":5000000000,"servers":{"news.example.com":{"total":5000000000000,"day":5000000000}}}`))
		}
	}))
	defer srv.Close()
	src, err := newSABnzbd(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"key": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["servers"].Rows != nil && v["history"].Rows != nil && v["down"].Text != ""
	})
	if v["down"].Text != "2,1 Mo/s" || v["queue"].Text != "2" || v["left"].Text != "419 Mo" || v["status"].Text != "en téléchargement" {
		t.Fatalf("file : %+v %+v %+v %+v", v["down"], v["queue"], v["left"], v["status"])
	}
	if v["free"].Text != "215 Go" || v["free"].Sub != "terminés : 161 Go" || v["warnings"].Text != "2" || v["day"].Text != "5 Go" {
		t.Fatalf("détail : %+v %+v %+v %+v", v["free"], v["warnings"], v["day"], v["total"])
	}
	if v["failed"].Text != "1" || v["history"].Rows[1][1].Level != "crit" {
		t.Fatalf("historique : %+v", v["history"])
	}
}

func TestSABnzbdBadKeyHidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":false,"error":"API Key Incorrect"}`))
	}))
	defer srv.Close()
	src, _ := newSABnzbd(config.Tile{URL: srv.URL}, netSecrets(map[string]string{"key": "SECRET-CLE"}))
	var q struct{}
	err := src.(*sabnzbd).get(t.Context(), "queue", nil, &q)
	if err == nil || !strings.Contains(err.Error(), "API Key Incorrect") {
		t.Fatalf("clé fausse non signalée : %v", err)
	}
	// Serveur arrêté : l'erreur réseau ne doit pas porter la clé.
	srv.Close()
	err = src.(*sabnzbd).get(t.Context(), "queue", nil, &q)
	if err == nil || strings.Contains(err.Error(), "SECRET-CLE") {
		t.Fatalf("clé dans l'erreur : %v", err)
	}
}
