package integ

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// neofreebox (passerelle vers l'API Freebox OS). GET /api/v1/summary avec un
// jeton d'API (rôle token, lecture seule suffit) : connexion, débits, box,
// températures, ventilateurs, appareils, Wi-Fi, appels manqués ; ni adresse,
// ni MAC, ni nom d'appareil. Les trois derniers blocs manquent si le jeton de
// la box n'y a pas droit. Relevé toutes les 10 s.
type neofreebox struct {
	url, token string
	hc         *http.Client
	down, up   *tile.Ring
}

func newNeofreebox(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &neofreebox{url: strings.TrimRight(t.URL, "/") + "/api/v1/summary", hc: httpClient(t.InsecureTLS, false), down: tile.NewRing(60), up: tile.NewRing(60)}
	var err error
	if n.token, err = secret("token"); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *neofreebox) Fields() []tile.Field {
	return []tile.Field{
		tile.F("state", "Connexion", tile.Stat, tile.OnTile),
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("up", "Envoi", tile.Spark, tile.OnTile),
		tile.F("devices", "Appareils connectés", tile.Stat, tile.OnTile),
		tile.F("temp", "Température", tile.Stat, tile.OnTile),
		tile.F("wifi", "Wi-Fi", tile.Stat, tile.InDetail),
		tile.F("calls", "Appels manqués", tile.Stat, tile.InDetail),
		tile.F("line", "Débit maximal", tile.Stat, tile.InDetail),
		tile.F("uptime", "Allumée depuis", tile.Stat, tile.InDetail),
		tile.F("box", "Box", tile.Stat, tile.InDetail),
		tile.F("sensors", "Capteurs", tile.Table, tile.InDetail),
	}
}

func (n *neofreebox) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 10*time.Second, func() {
		v, err := n.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (n *neofreebox) poll(ctx context.Context) (tile.Values, error) {
	var s struct {
		Connection struct {
			Up            bool
			State, Media  string
			RateDown      float64 `json:"rate_down_bps"`
			RateUp        float64 `json:"rate_up_bps"`
			BandwidthDown float64 `json:"bandwidth_down_bps"`
			BandwidthUp   float64 `json:"bandwidth_up_bps"`
		}
		Box struct {
			Model, Firmware string
			Uptime          float64 `json:"uptime_s"`
		}
		Temperatures []struct {
			Name    string
			Celsius int
		}
		TempMax int   `json:"temperature_max_c"`
		Fans    []int `json:"fans_rpm"`
		Devices *struct{ Connected, Seen int }
		Wifi    *struct{ Enabled bool }
		Calls   *struct{ Unread int }
	}
	if err := getJSON(ctx, n.hc, n.url, &s, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+n.token) }); err != nil {
		return nil, err
	}
	c := s.Connection
	state := tile.Value{Text: "Connectée", Sub: strings.ToUpper(c.Media), Level: "ok"}
	if !c.Up {
		state = tile.Value{Text: "Coupée", Sub: c.State, Level: "crit"}
	}
	v := tile.Values{
		"state":  state,
		"down":   {Text: bitRate(c.RateDown), Points: n.down.Push(c.RateDown)},
		"up":     {Text: bitRate(c.RateUp), Points: n.up.Push(c.RateUp)},
		"line":   {Text: bitRate(c.BandwidthDown) + " ↓ · " + bitRate(c.BandwidthUp) + " ↑"},
		"uptime": {Text: tile.Dur(time.Duration(s.Box.Uptime) * time.Second)},
		"box":    {Text: s.Box.Model, Sub: "micrologiciel " + s.Box.Firmware},
	}
	rows := make([][]tile.Cell, 0, len(s.Temperatures)+len(s.Fans))
	for _, t := range s.Temperatures {
		rows = append(rows, []tile.Cell{{Text: t.Name}, {Text: fmt.Sprintf("%d °C", t.Celsius), Level: tile.Level(float64(t.Celsius), 80, 90)}})
	}
	for i, f := range s.Fans {
		rows = append(rows, []tile.Cell{{Text: fmt.Sprintf("Ventilateur %d", i+1)}, {Text: fmt.Sprintf("%d tr/min", f)}})
	}
	v["sensors"] = tile.Value{Head: []string{"Capteur", "Valeur"}, Rows: rows}
	if len(s.Temperatures) > 0 {
		v["temp"] = tile.Value{Text: fmt.Sprintf("%d °C", s.TempMax), Sub: "la plus haute", Level: tile.Level(float64(s.TempMax), 80, 90)}
	}
	if d := s.Devices; d != nil {
		v["devices"] = tile.Value{Text: fmt.Sprint(d.Connected), Sub: fmt.Sprintf("sur %d connus", d.Seen)}
	}
	if w := s.Wifi; w != nil {
		v["wifi"] = tile.Value{Text: "Activé", Level: "ok"}
		if !w.Enabled {
			v["wifi"] = tile.Value{Text: "Coupé", Level: "warn"}
		}
	}
	if cl := s.Calls; cl != nil {
		v["calls"] = tile.Value{Text: fmt.Sprint(cl.Unread)}
		if cl.Unread > 0 {
			v["calls"] = tile.Value{Text: fmt.Sprint(cl.Unread), Level: "warn"}
		}
	}
	return v, nil
}

// bitRate : débit réseau en bit/s, unités françaises (kb/s, Mb/s, Gb/s).
func bitRate(bps float64) string {
	switch {
	case bps >= 1e9:
		return tile.Num(bps/1e9) + " Gb/s"
	case bps >= 1e6:
		return tile.Num(bps/1e6) + " Mb/s"
	case bps >= 1e3:
		return tile.Num(bps/1e3) + " kb/s"
	}
	return tile.Num(bps) + " b/s"
}
