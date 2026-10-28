package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Forme d'après le code de Scrutiny (webapp/backend/pkg/models/device_summary.go :
// data.summary indexé par WWN, device_status masque de bits).
func TestScrutiny(t *testing.T) {
	srv := f4Serve(t, nil, nil, map[string]string{
		"/api/summary": `{"success":true,"data":{"summary":{
"0x5000c500a":{"device":{"wwn":"0x5000c500a","device_name":"sda","model_name":"ST4000","device_status":0,"capacity":4000787030016,"device_protocol":"ATA"},"smart":{"temp":34,"power_on_hours":20000,"collector_date":"2026-10-27T01:00:00Z"}},
"0x5000c500b":{"device":{"wwn":"0x5000c500b","device_name":"sdb","model_name":"WD40","device_status":1,"capacity":4000787030016,"device_protocol":"ATA"},"smart":{"temp":52,"power_on_hours":30000,"collector_date":"2026-10-27T01:00:00Z"}}}}}`,
	})
	defer srv.Close()
	src, _ := newScrutiny(config.Tile{URL: srv.URL}, nil)
	v, err := src.(*scrutiny).poll(t.Context())
	if err != nil || v["disks"].Text != "2" || v["failed"].Text != "1" || v["failed"].Level != "crit" || v["hottest"].Text != "52 °C" || v["hottest"].Sub != "sdb" || v["hottest"].Level != "warn" {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	if r := v["disk_list"].Rows; len(r) != 2 || r[0][0].Text != "sdb" || r[0][3].Text != "Échec SMART" || len(v["temps"].Bars) != 2 {
		t.Fatalf("table : %+v", r)
	}
}
