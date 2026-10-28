package integ

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Réponses construites d'après la documentation de l'API NZBGet
// (nzbget.com/documentation/api : status, listgroups, history ; MB binaires,
// statuts d'historique « SUCCESS/ALL », « FAILURE/UNPACK »).
func TestNZBGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "nzbget" || p != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct{ Method string }
		json.NewDecoder(r.Body).Decode(&req)
		body := map[string]string{
			"status":     `{"RemainingSizeMB":1024,"DownloadedSizeMB":2048,"FreeDiskSpaceMB":102400,"DownloadRate":2097152,"DownloadLimit":0,"UpTimeSec":90000,"DownloadPaused":false,"ThreadCount":12,"ParJobCount":1,"PostJobCount":2,"ArticleCacheMB":16}`,
			"listgroups": `[{"NZBName":"Film.2024","Status":"DOWNLOADING","FileSizeMB":2048,"RemainingSizeMB":1024,"Category":"movies","Health":1000},{"NZBName":"Serie","Status":"PAUSED","FileSizeMB":100,"RemainingSizeMB":100,"Category":"tv","Health":900}]`,
			"history":    `[{"Name":"Vieux","Status":"SUCCESS/ALL","FileSizeMB":500,"HistoryTime":1700000000},{"Name":"Raté","Status":"FAILURE/UNPACK","FileSizeMB":700,"HistoryTime":1700000500}]`,
		}[req.Method]
		w.Write([]byte(`{"version":"1.1","id":1,"result":` + body + `}`))
	}))
	defer srv.Close()
	src, err := newNZBGet(config.Tile{URL: srv.URL, Secrets: map[string]string{"user": "u", "pass": "p"}}, netSecrets(map[string]string{"user": "nzbget", "pass": "factice"}))
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool { return v["history"].Rows != nil && v["dl"].Rows != nil })
	if v["down"].Text != "2,1 Mo/s" || v["queue"].Text != "2" || v["queue"].Sub != "1 en pause" || v["left"].Text != "1,1 Go" || v["left"].Sub == "" {
		t.Fatalf("file : %+v %+v %+v", v["down"], v["queue"], v["left"])
	}
	if v["status"].Text != "en téléchargement" || v["free"].Text != "107 Go" || v["failed"].Text != "1" {
		t.Fatalf("détail : %+v %+v %+v", v["status"], v["free"], v["failed"])
	}
	h := v["history"].Rows
	if len(h) != 2 || h[0][0].Text != "Raté" || h[0][1].Level != "crit" || h[1][1].Level != "ok" {
		t.Fatalf("historique : %+v", h)
	}
	if d := v["dl"].Rows; d[0][0].Text != "Film.2024" || d[0][1].Text != "50 %" || d[1][3].Level != "warn" {
		t.Fatalf("file : %+v", d)
	}
}

func TestNZBGetBadAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	src, _ := newNZBGet(config.Tile{URL: srv.URL}, netSecrets(nil))
	if err := src.(*nzbget).rpc(t.Context(), "status", nil, &struct{}{}); err == nil {
		t.Fatal("401 non signalé")
	}
}
