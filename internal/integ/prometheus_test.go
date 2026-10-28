package integ

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses conformes à https://prometheus.io/docs/prometheus/latest/querying/api/
// (format des réponses, résultats instantanés vector/scalar, erreurs 400 bad_data).
func TestPrometheusQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" || r.Header.Get("Authorization") != "Bearer jeton-factice" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Query().Get("query") {
		case `up{job="node"}`:
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up","job":"node","instance":"localhost:9100"},"value":[1435781451.781,"1"]},{"metric":{"__name__":"up","job":"node","instance":"b:9100"},"value":[1435781451.781,"0"]}]}}`))
		case "scalar(42.5)":
			w.Write([]byte(`{"status":"success","data":{"resultType":"scalar","result":[1435781451.781,"42.5"]}}`))
		case "absent_series":
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		case "rate(x[5m])[1h:]":
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case "0/0":
			w.Write([]byte(`{"status":"success","data":{"resultType":"scalar","result":[1435781451.781,"NaN"]}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"invalid parameter \"query\": 1:4: parse error: unexpected end of input"}`))
		}
	}))
	defer srv.Close()
	fields := []config.FieldMap{
		{ID: "up", Path: `up{job="node"}`, Format: "number", Warn: fl(0.5), Crit: fl(0)},
		{ID: "scalar", Path: "scalar(42.5)", Format: "percent"},
		{ID: "none", Path: "absent_series", Format: "number"},
		{ID: "matrix", Path: "rate(x[5m])[1h:]", Format: "number"},
		{ID: "nan", Path: "0/0", Format: "number"},
		{ID: "bad", Path: "sum(", Format: "number"},
	}
	src, err := newPrometheus(config.Tile{URL: srv.URL + "/", Fields: fields, Secrets: map[string]string{"token": "t"}}, netSecrets(map[string]string{"token": "jeton-factice"}))
	if err != nil {
		t.Fatal(err)
	}
	p := src.(*prometheus)
	v, err := p.pollQuery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if x := netNeed(t, v, "up"); x.Text != "1" || x.Level != "ok" {
		t.Errorf("up = %+v", x)
	}
	if x := netNeed(t, v, "scalar"); x.Text != "42,5 %" || x.Pct != 42.5 {
		t.Errorf("scalar = %+v", x)
	}
	for id, sub := range map[string]string{"none": "aucun résultat", "matrix": "matrix", "nan": "non finie", "bad": "parse error"} {
		if x := netNeed(t, v, id); x.Level != "warn" || !strings.Contains(x.Sub, sub) {
			t.Errorf("%s = %+v", id, x)
		}
	}

	// Serveur injoignable ou jeton refusé : toute la tuile en erreur.
	p.set = func(*http.Request) {}
	if _, err := p.pollQuery(context.Background()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404 attendu : %v", err)
	}
	if _, err := newPrometheus(config.Tile{URL: srv.URL, Fields: []config.FieldMap{{ID: "a"}}}, nil); err == nil {
		t.Error("path vide refusé attendu")
	}
}

// Exposition texte selon https://prometheus.io/docs/instrumenting/exposition_formats/
// (exemple officiel repris, avec échappements et horodatages).
const promSampleText = `# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="post",code="200"} 1027 1395066363000
http_requests_total{method="post",code="400"}    3 1395066363000
http_requests_total{code="200",method="get"} 500

# Escaping in label values:
msdos_file_access_time_seconds{path="C:\\DIR\\FILE.TXT",error="Cannot find file:\n\"FILE.TXT\""} 1.458255915e9

# Minimalistic line:
metric_without_timestamp_and_labels 12.47

# A weird metric from before the epoch:
something_weird{problem="division by zero"} +Inf -3982045

# A histogram, which has a pretty complex representation in the text format:
# HELP http_request_duration_seconds A histogram of the request duration.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.05"} 24054
http_request_duration_seconds_bucket{le="+Inf"} 144320
http_request_duration_seconds_sum 53423
http_request_duration_seconds_count 144320
node_filesystem_avail_bytes{mountpoint="/",fstype="ext4"} 5e9
`

func TestPrometheusScrape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(promSampleText))
	}))
	defer srv.Close()
	fields := []config.FieldMap{
		{ID: "all", Path: "http_requests_total", Format: "number"},
		{ID: "post", Path: `http_requests_total{method="post"}`, Format: "number"},
		{ID: "post200", Path: `http_requests_total{ code="200", method="post" }`, Format: "number"},
		{ID: "esc", Path: `msdos_file_access_time_seconds{path="C:\\DIR\\FILE.TXT"}`, Format: "date"},
		{ID: "plain", Path: "metric_without_timestamp_and_labels"},
		{ID: "inf", Path: "something_weird", Format: "number"},
		{ID: "bucket", Path: `http_request_duration_seconds_bucket{le="+Inf"}`, Format: "number"},
		{ID: "avail", Path: `node_filesystem_avail_bytes{mountpoint="/"}`, Format: "bytes"},
		{ID: "absent", Path: `http_requests_total{method="put"}`, Format: "number"},
	}
	src, err := newPrometheus(config.Tile{URL: srv.URL + "/metrics", Fields: fields, Options: map[string]string{"scrape": "true"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := src.(*prometheus)
	v, err := p.pollScrape(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"all": "1\u202f530", "post": "1\u202f030", "post200": "1\u202f027", "plain": "12.47", "bucket": "144\u202f320", "avail": "5 Go", "esc": ""}
	for id, w := range want {
		if x := netNeed(t, v, id); !strings.HasPrefix(x.Text, w) {
			t.Errorf("%s = %+v, attendu %q", id, x, w)
		}
	}
	if x := netNeed(t, v, "esc"); !strings.Contains(x.Text, "/03/2016") {
		t.Errorf("esc = %+v", x)
	}
	if x := netNeed(t, v, "inf"); x.Level != "warn" || !strings.Contains(x.Sub, "non finie") {
		t.Errorf("inf = %+v", x)
	}
	if x := netNeed(t, v, "absent"); x.Level != "warn" || !strings.Contains(x.Sub, "introuvable") {
		t.Errorf("absent = %+v", x)
	}

	p.base = srv.URL + "/ailleurs"
	if _, err := p.pollScrape(context.Background()); err == nil {
		t.Error("404 attendu")
	}
	for _, path := range []string{"9nom", `x{a=~"b"}`, `x{a="b"`, `x{a=b}`, `x{a="b"} y`} {
		if _, err := newPrometheus(config.Tile{URL: srv.URL, Options: map[string]string{"scrape": "1"}, Fields: []config.FieldMap{{ID: "a", Path: path}}}, nil); err == nil {
			t.Errorf("sélecteur %q accepté", path)
		}
	}
	if _, err := newPrometheus(config.Tile{URL: srv.URL, Options: map[string]string{"scrape": "peut-être"}, Fields: fields}, nil); err == nil {
		t.Error("scrape invalide accepté")
	}
}
