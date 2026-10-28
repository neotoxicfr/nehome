package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme d'après la documentation de l'API v1 de Speedtest Tracker
// (/results/latest et /results ; débits en octets/s, ping en ms). Non éprouvé
// sur une vraie instance.
func TestSpeedtest(t *testing.T) {
	one := func(id, down, up, ping, at string) string {
		return `{"id":` + id + `,"ping":` + ping + `,"download":` + down + `,"upload":` + up + `,"status":"completed","server_name":"Free","created_at":"` + at + `","data":{"isp":"Free SAS","server":{"name":"Free","location":"Paris"}}}`
	}
	srv := f4Serve(t, f4Hdr("Authorization", "Bearer factice"), nil, map[string]string{
		"/api/v1/results/latest":                             `{"message":"ok","data":` + one("2", "125000000", "12500000", "11.5", "2026-10-27 06:00:00") + `}`,
		"/api/v1/results?page%5Bsize%5D=30&sort=-created_at": `{"data":[` + one("2", "125000000", "12500000", "11.5", "2026-10-27 06:00:00") + `,` + one("1", "100000000", "10000000", "12", "2026-10-26 06:00:00") + `]}`,
	})
	defer srv.Close()
	src, _ := newSpeedtest(config.Tile{URL: srv.URL}, mediaSecret)
	v, err := src.(*speedtest).poll(t.Context())
	if err != nil || v["download"].Text != "1000 Mbit/s" || v["upload"].Text != "100 Mbit/s" || v["ping"].Text != "11,5 ms" || v["server"].Sub != "Paris Free SAS" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	if p := v["download_hist"].Points; len(p) != 2 || p[0] != 800 || p[1] != 1000 {
		t.Fatalf("courbe (ordre chronologique) : %v", p)
	}
}
