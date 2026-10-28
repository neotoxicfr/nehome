package integ

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses construites d'après la spécification RPC de Transmission
// (github.com/transmission/transmission, docs/rpc-spec.md) : session-stats,
// session-get, free-space, torrent-get, et le 409 portant X-Transmission-Session-Id.
func TestTransmission(t *testing.T) {
	const sid = "jeton-factice"
	calls409 := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") != sid {
			calls409++
			w.Header().Set("X-Transmission-Session-Id", sid)
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req struct{ Method string }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		body := map[string]string{
			"session-stats": `{"result":"success","arguments":{"activeTorrentCount":2,"pausedTorrentCount":1,"torrentCount":5,"downloadSpeed":1500000,"uploadSpeed":250000,"cumulative-stats":{"downloadedBytes":1000000000,"uploadedBytes":2500000000}}}`,
			"session-get":   `{"result":"success","arguments":{"version":"4.0.6 (38c164933e)","download-dir":"/downloads","alt-speed-enabled":false,"speed-limit-down-enabled":true,"speed-limit-down":5000,"speed-limit-up-enabled":false}}`,
			"free-space":    `{"result":"success","arguments":{"path":"/downloads","size-bytes":500000000000}}`,
			"torrent-get": `{"result":"success","arguments":{"torrents":[
				{"name":"debian.iso","status":4,"percentDone":0.5,"rateDownload":1500000,"rateUpload":0,"eta":600,"totalSize":4000000000,"error":0,"uploadRatio":0.1},
				{"name":"attente","status":3,"percentDone":0,"rateDownload":0,"rateUpload":0,"eta":-1,"totalSize":1,"error":0,"uploadRatio":0},
				{"name":"partage","status":6,"percentDone":1,"rateDownload":0,"rateUpload":250000,"eta":-1,"totalSize":1,"error":0,"uploadRatio":2.5},
				{"name":"cassé","status":0,"percentDone":1,"rateDownload":0,"rateUpload":0,"eta":-1,"totalSize":1,"error":3,"uploadRatio":0}]}}`,
		}[req.Method]
		w.Write([]byte(body))
	}))
	defer srv.Close()
	src, err := newTransmission(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, netSecrets(map[string]string{"user": "admin", "pass": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*transmission).poll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls409 != 1 {
		t.Fatalf("le jeton doit être repris une seule fois : %d", calls409)
	}
	if v["down"].Text != "1,5 Mo/s" || v["leech"].Text != "2" || v["seed"].Text != "1" || v["errors"].Text != "1" {
		t.Fatalf("compteurs : %+v", v)
	}
	if v["ratio"].Text != "2,5" || v["free"].Text != "500 Go" || v["version"].Text != "4.0.6 (38c164933e)" || v["alt"].Text != "non" {
		t.Fatalf("détail : %+v %+v %+v", v["ratio"], v["free"], v["version"])
	}
	if r := v["dl"].Rows; len(r) != 2 || r[0][0].Text != "debian.iso" || r[0][1].Text != "50 %" || r[0][3].Text == "∞" {
		t.Fatalf("table : %+v", v["dl"].Rows)
	}
}

func TestTransmissionAuthRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	src, _ := newTransmission(config.Tile{URL: srv.URL}, netSecrets(nil))
	if _, err := src.(*transmission).poll(t.Context()); err == nil {
		t.Fatal("401 non signalé")
	}
}
