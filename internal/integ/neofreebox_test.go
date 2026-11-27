package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme de internal/server/status.go (apiSummary) de neofreebox.
func TestNeofreebox(t *testing.T) {
	full := `{"ok":true,"connection":{"up":true,"state":"up","media":"ftth","rate_down_bps":12500000,"rate_up_bps":800000,"bandwidth_down_bps":8000000000,"bandwidth_up_bps":700000000},
"box":{"model":"Freebox Ultra","firmware":"4.9.1","uptime_s":90000},"temperatures":[{"name":"CPU","celsius":84},{"name":"Disque","celsius":40}],"temperature_max_c":84,"fans_rpm":[1200],
"devices":{"connected":12,"seen":40},"wifi":{"enabled":false},"calls":{"unread":2}}`
	srv := f4Serve(t, f4Hdr("Authorization", "Bearer jeton"), nil, map[string]string{"/api/v1/summary": full})
	defer srv.Close()
	src, err := newNeofreebox(config.Tile{URL: srv.URL}, func(string) (string, error) { return "jeton", nil })
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*neofreebox).poll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["state"].Text != "Connectée" || v["state"].Sub != "FTTH" || v["down"].Text != "12,5 Mb/s" || v["down"].Sub != "sur 8 Gb/s" || v["up"].Text != "800 kb/s" {
		t.Fatalf("connexion : %+v", v)
	}
	if v["temp"].Text != "84 °C" || v["temp"].Level != "warn" || v["devices"].Text != "12" || v["wifi"].Level != "warn" || v["calls"].Text != "2" || v["uptime"].Text != "1 j 1 h" {
		t.Fatalf("état : %+v", v)
	}
	if r := v["sensors"].Rows; len(r) != 3 || r[2][1].Text != "1200 tr/min" {
		t.Fatalf("capteurs : %+v", r)
	}

	// jeton sans droits sur les appareils, le Wi-Fi et les appels : ces lignes manquent
	down := `{"ok":true,"connection":{"up":false,"state":"going_down","media":"ftth"},"box":{},"temperatures":[],"temperature_max_c":0,"fans_rpm":[]}`
	srv2 := f4Serve(t, nil, nil, map[string]string{"/api/v1/summary": down})
	defer srv2.Close()
	src, _ = newNeofreebox(config.Tile{URL: srv2.URL}, func(string) (string, error) { return "x", nil })
	v, err = src.(*neofreebox).poll(t.Context())
	if err != nil || v["state"].Level != "crit" {
		t.Fatalf("coupée : %v %+v", err, v)
	}
	for _, k := range []string{"devices", "wifi", "calls", "temp"} {
		if _, ok := v[k]; ok {
			t.Fatalf("%s présent sans donnée", k)
		}
	}
}
