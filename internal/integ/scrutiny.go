package integ

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Scrutiny (surveillance SMART) : GET /api/summary, sans authentification
// (Scrutiny n'en a pas ; à protéger côté reverse proxy). device_status est un
// masque de bits : 0 sain, 1 échec SMART, 2 échec du seuil Scrutiny.
// Relevé toutes les 5 min (le collecteur passe une fois par jour par défaut).
type scrutiny struct {
	url string
	hc  *http.Client
}

func newScrutiny(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	return &scrutiny{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}, nil
}

func (s *scrutiny) Fields() []tile.Field {
	return []tile.Field{
		tile.F("disks", "Disques", tile.Stat, tile.OnTile),
		tile.F("failed", "Disques en échec", tile.Stat, tile.OnTile),
		tile.F("hottest", "Disque le plus chaud", tile.Stat, tile.OnTile),
		tile.F("temps", "Températures", tile.Bars, tile.InDetail),
		tile.F("disk_list", "Disques (SMART)", tile.Table, tile.InDetail),
	}
}

func (s *scrutiny) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 5*time.Minute, func() {
		v, err := s.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (s *scrutiny) poll(ctx context.Context) (tile.Values, error) {
	var r struct {
		Data struct {
			Summary map[string]struct {
				Device struct {
					DeviceName string `json:"device_name"`
					ModelName  string `json:"model_name"`
					Status     int    `json:"device_status"`
					Capacity   float64
					Protocol   string `json:"device_protocol"`
					Label      string
				}
				Smart *struct {
					Temp          float64
					PowerOnHours  float64   `json:"power_on_hours"`
					CollectorDate time.Time `json:"collector_date"`
				}
			}
		}
	}
	if err := getJSON(ctx, s.hc, s.url+"/api/summary", &r, nil); err != nil {
		return nil, err
	}
	type disk struct {
		name, model, proto string
		status             int
		temp, hours, cap   float64
		seen               time.Time
	}
	var ds []disk
	for _, e := range r.Data.Summary {
		d := disk{name: e.Device.DeviceName, model: e.Device.ModelName, proto: e.Device.Protocol, status: e.Device.Status, cap: e.Device.Capacity}
		if e.Device.Label != "" {
			d.name = e.Device.Label
		}
		if e.Smart != nil {
			d.temp, d.hours, d.seen = e.Smart.Temp, e.Smart.PowerOnHours, e.Smart.CollectorDate
		}
		ds = append(ds, d)
	}
	// Échecs d'abord, puis par nom : ordre stable d'un relevé à l'autre.
	sort.Slice(ds, func(i, j int) bool {
		if (ds[i].status != 0) != (ds[j].status != 0) {
			return ds[i].status != 0
		}
		return ds[i].name < ds[j].name
	})
	v := tile.Values{"disks": {Text: fmt.Sprint(len(ds))}}
	failed := 0
	var hot disk
	var bars []tile.Bar
	var rows [][]tile.Cell
	for _, d := range ds {
		st, lvl := "Sain", "ok"
		if d.status != 0 {
			failed++
			st, lvl = "Échec", "crit"
			switch {
			case d.status&1 != 0 && d.status&2 != 0:
				st = "Échec SMART et Scrutiny"
			case d.status&1 != 0:
				st = "Échec SMART"
			case d.status&2 != 0:
				st = "Échec Scrutiny"
			}
		}
		if d.temp > hot.temp {
			hot = d
		}
		temp := "-"
		if d.temp > 0 {
			temp = fmt.Sprintf("%.0f °C", d.temp)
			bars = append(bars, tile.Bar{Label: d.name, Text: temp, Pct: min(d.temp/70*100, 100), Level: tile.Level(d.temp, 50, 60)})
		}
		hours := "-"
		if d.hours > 0 {
			hours = tile.Dur(time.Duration(d.hours) * time.Hour)
		}
		if len(rows) < 15 {
			rows = append(rows, []tile.Cell{{Text: d.name}, {Text: d.model}, {Text: tile.Bytes(d.cap)}, {Text: st, Level: lvl}, {Text: temp, Level: tile.Level(d.temp, 50, 60)}, {Text: hours}})
		}
	}
	fv := tile.Value{Text: fmt.Sprint(failed), Level: "ok"}
	if failed > 0 {
		fv.Level = "crit"
	}
	v["failed"] = fv
	if hot.temp > 0 {
		v["hottest"] = tile.Value{Text: fmt.Sprintf("%.0f °C", hot.temp), Sub: hot.name, Level: tile.Level(hot.temp, 50, 60)}
	}
	if len(bars) > 0 {
		v["temps"] = tile.Value{Bars: bars}
	}
	v["disk_list"] = tile.Value{Head: []string{"Disque", "Modèle", "Capacité", "État SMART", "Température", "Allumé"}, Rows: rows}
	return v, nil
}
