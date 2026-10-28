package integ

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// customapi est générique : la réponse ci-dessous couvre les cas limites du
// JSON (null, tableaux vides, nombres en chaîne, grands entiers, booléens).
func fl(f float64) *float64 { return &f }

func TestCustomAPI(t *testing.T) {
	recent := strconv.FormatInt(time.Now().Add(-2*time.Hour).UnixMilli(), 10)
	body := `{"data":{"items":[{"count":3,"name":"a"},{"count":"1234.5","name":"b"}],"empty":[],"nothing":null,
		"used":1500000000,"rate":2500,"pct":"91.2","free":8,"uptime":93784,"ok":true,"id":12345678901234567890,
		"when":"2026-10-27T08:00:00Z","ms":` + recent + `,"str":"en ligne","nan":"abc"}}`
	var gotAuth, gotKey, gotX string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gotAuth, gotKey, gotX = r.Header.Get("Authorization"), r.Header.Get("X-Clef"), r.Header.Get("X-Custom")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	fields := []config.FieldMap{
		{ID: "n_items", Label: "Éléments", Path: "data.items.#", Format: "number"},
		{ID: "first", Path: "data.items.0.count", Format: "number", Warn: fl(2), Crit: fl(5)},
		{ID: "last_name", Path: "data.items.-1.name"},
		{ID: "strnum", Path: "data.items.1.count", Format: "number"},
		{ID: "empty", Path: "data.empty.#", Format: "number"},
		{ID: "null_len", Path: "data.nothing.#", Format: "number"},
		{ID: "null", Path: "data.nothing"},
		{ID: "used", Path: "data.used", Format: "bytes", Placement: "detail"},
		{ID: "rate", Path: "data.rate", Format: "rate"},
		{ID: "pct", Path: "data.pct", Format: "percent", Warn: fl(80), Crit: fl(90)},
		{ID: "free", Path: "data.free", Format: "number", Warn: fl(20), Crit: fl(10)}, // inversé
		{ID: "uptime", Path: "data.uptime", Format: "duration"},
		{ID: "ok", Path: "data.ok"},
		{ID: "id", Path: "data.id"},
		{ID: "when", Path: "data.when", Format: "date"},
		{ID: "ms", Path: "data.ms", Format: "date", Warn: fl(3600), Crit: fl(86400)},
		{ID: "str", Path: "data.str"},
		{ID: "missing", Path: "data.absent.x"},
		{ID: "oob", Path: "data.items.5.count"},
		{ID: "badidx", Path: "data.items.x"},
		{ID: "notnum", Path: "data.nan", Format: "number"},
		{ID: "scalar", Path: "data.str.x"},
	}
	src, err := newCustomAPI(config.Tile{URL: srv.URL + "/api", Fields: fields,
		Secrets: map[string]string{"token": "t", "key": "k"},
		Options: map[string]string{"key_header": "X-Clef", "header.X-Custom": "oui", "method": "get"}},
		netSecrets(map[string]string{"token": "jeton-factice", "key": "clef-factice"}))
	if err != nil {
		t.Fatal(err)
	}
	c := src.(*customAPI)
	if f := c.Fields(); len(f) != len(fields) || f[7].Default != tile.InDetail || f[9].Kind != tile.Meter || f[0].Label != "Éléments" || f[1].Label != "first" {
		t.Errorf("Fields = %+v", f)
	}
	v, err := c.poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer jeton-factice" || gotKey != "clef-factice" || gotX != "oui" {
		t.Errorf("en-têtes : %q %q %q", gotAuth, gotKey, gotX)
	}
	want := map[string]struct{ text, level string }{
		"n_items":   {"2", ""},
		"first":     {"3", "warn"},
		"last_name": {"b", ""},
		"strnum":    {"1\u202f235", ""},
		"empty":     {"0", ""},
		"null_len":  {"0", ""},
		"null":      {"—", ""},
		"used":      {"1,5 Go", ""},
		"rate":      {"2,5 ko/s", ""},
		"pct":       {"91,2 %", "crit"},
		"free":      {"8", "crit"},
		"uptime":    {"1 j 2 h", ""},
		"ok":        {"oui", ""},
		"id":        {"12345678901234567890", ""},
		"str":       {"en ligne", ""},
		"ms":        {"", "warn"},
		"missing":   {"—", "warn"},
		"oob":       {"—", "warn"},
		"badidx":    {"—", "warn"},
		"notnum":    {"—", "warn"},
		"scalar":    {"—", "warn"},
	}
	for id, w := range want {
		x := netNeed(t, v, id)
		if (w.text != "" && x.Text != w.text) || x.Level != w.level {
			t.Errorf("%s = %+v, attendu %q/%q", id, x, w.text, w.level)
		}
	}
	if v["pct"].Pct != 91.2 {
		t.Errorf("pct.Pct = %v", v["pct"].Pct)
	}
	if !strings.Contains(v["missing"].Sub, `clé "absent"`) || !strings.Contains(v["oob"].Sub, "2 éléments") {
		t.Errorf("messages : %q / %q", v["missing"].Sub, v["oob"].Sub)
	}
	if !strings.HasPrefix(v["ms"].Sub, "il y a 2 h") || !strings.Contains(v["when"].Text, "/10/2026") {
		t.Errorf("dates : %+v %+v", v["ms"], v["when"])
	}
}

func TestCustomAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html":
			w.Write([]byte("<html>connexion</html>"))
		case "/vide":
		case "/basic":
			if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "factice" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`42`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	f := []config.FieldMap{{ID: "v", Format: "number"}}
	mk := func(path string, sec map[string]string) *customAPI {
		t.Helper()
		roles := map[string]string{}
		for k := range sec {
			roles[k] = k
		}
		src, err := newCustomAPI(config.Tile{URL: srv.URL + path + "?api_key=secret", Fields: f, Secrets: roles}, netSecrets(sec))
		if err != nil {
			t.Fatal(err)
		}
		return src.(*customAPI)
	}
	for _, p := range []string{"/html", "/vide", "/500"} {
		_, err := mk(p, nil).poll(context.Background())
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("%s : erreur = %v", p, err)
		}
	}
	v, err := mk("/basic", map[string]string{"user": "admin", "pass": "factice"}).poll(context.Background())
	if err != nil || v["v"].Text != "42" {
		t.Errorf("basic + racine : %+v %v", v, err)
	}

	bad := []config.Tile{
		{URL: srv.URL, Fields: f, Options: map[string]string{"method": "POST"}},
		{URL: srv.URL, Fields: f, Options: map[string]string{"interval": "1s"}},
		{URL: srv.URL, Fields: f, Options: map[string]string{"interval": "bof"}},
		{URL: srv.URL},
		{Fields: f},
		{URL: srv.URL, Fields: []config.FieldMap{{ID: "a"}, {ID: "a"}}},
		{URL: srv.URL, Fields: []config.FieldMap{{ID: "a", Format: "kg"}}},
		{URL: srv.URL, Fields: []config.FieldMap{{ID: "a", Placement: "ailleurs"}}},
		{URL: srv.URL, Fields: f, Secrets: map[string]string{"user": "u"}},
	}
	for i, tl := range bad {
		if _, err := newCustomAPI(tl, netSecrets(map[string]string{"user": "x"})); err == nil {
			t.Errorf("config %d acceptée", i)
		}
	}
	if (&customAPI{}).Fields() != nil {
		t.Error("valeur zéro : nil attendu")
	}
}
