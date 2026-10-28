package integ

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Synology DSM : Web API (webapi/auth.cgi puis entry.cgi). Tout passe en POST
// formulaire, identifiants et sid dans le corps jamais dans l'URL ; ce sont des
// appels de lecture (get / load_info). Un compte avec 2FA n'est pas pris en
// charge : créer un compte local dédié, non administrateur si possible.
type synology struct {
	url, user, pass string
	hc              *http.Client
	sid             string
}

func newSynology(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	s := &synology{url: strings.TrimRight(t.URL, "/") + "/webapi", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if s.user, err = secret("user"); err != nil {
		return nil, err
	}
	if s.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *synology) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cpu", "Processeur", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("volumes", "Volumes", tile.Bars, tile.OnTile),
		tile.F("temp", "Température", tile.Stat, tile.OnTile),
		tile.F("model", "Modèle", tile.Stat, tile.InDetail),
		tile.F("firmware", "Version de DSM", tile.Stat, tile.InDetail),
		tile.F("uptime", "Uptime", tile.Stat, tile.InDetail),
		tile.F("load", "Charge", tile.Stat, tile.InDetail),
		tile.F("net", "Réseau", tile.Stat, tile.InDetail),
		tile.F("disktable", "Disques", tile.Table, tile.InDetail),
	}
}

type synoResp struct {
	Success bool `json:"success"`
	Error   struct {
		Code int `json:"code"`
	} `json:"error"`
	Data map[string]any `json:"data"`
}

func (s *synology) login(ctx context.Context) error {
	f := url.Values{"api": {"SYNO.API.Auth"}, "version": {"6"}, "method": {"login"},
		"account": {s.user}, "passwd": {s.pass}, "session": {"nehome"}, "format": {"sid"}}
	var r synoResp
	if err := infPost(ctx, s.hc, s.url+"/auth.cgi", "application/x-www-form-urlencoded", []byte(f.Encode()), &r, nil); err != nil {
		return err
	}
	sid, _ := r.Data["sid"].(string)
	if !r.Success || sid == "" {
		return fmt.Errorf("connexion refusée (code %d)", r.Error.Code)
	}
	s.sid = sid
	return nil
}

// call relance un login si la session est perdue (codes 105, 106, 107, 119).
func (s *synology) call(ctx context.Context, api string, ver int, method string, out any) error {
	for try := 0; ; try++ {
		if s.sid == "" {
			if err := s.login(ctx); err != nil {
				return err
			}
		}
		f := url.Values{"api": {api}, "version": {strconv.Itoa(ver)}, "method": {method}, "_sid": {s.sid}}
		var raw struct {
			Success bool `json:"success"`
			Error   struct {
				Code int `json:"code"`
			} `json:"error"`
			Data any `json:"data"`
		}
		raw.Data = out
		if err := infPost(ctx, s.hc, s.url+"/entry.cgi", "application/x-www-form-urlencoded", []byte(f.Encode()), &raw, nil); err != nil {
			return err
		}
		if raw.Success {
			return nil
		}
		switch raw.Error.Code {
		case 105, 106, 107, 119:
			if try == 0 {
				s.sid = ""
				continue
			}
		}
		return fmt.Errorf("%s : erreur DSM %d", api, raw.Error.Code)
	}
}

func (s *synology) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		if v := s.slow(ctx); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, 10*time.Second, func() {
		v, err := s.live(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (s *synology) live(ctx context.Context) (tile.Values, error) {
	var u struct {
		CPU struct {
			User   infNum `json:"user_load"`
			System infNum `json:"system_load"`
			Other  infNum `json:"other_load"`
			M1     infNum `json:"1min_load"`
			M5     infNum `json:"5min_load"`
			M15    infNum `json:"15min_load"`
		} `json:"cpu"`
		Mem struct {
			Real  infNum `json:"real_usage"`
			Total infNum `json:"total_real"` // Kio
			Avail infNum `json:"avail_real"`
		} `json:"memory"`
		Net []struct {
			Dev string `json:"device"`
			Rx  infNum `json:"rx"`
			Tx  infNum `json:"tx"`
		} `json:"network"`
	}
	if err := s.call(ctx, "SYNO.Core.System.Utilization", 1, "get", &u); err != nil {
		return nil, err
	}
	cpu := float64(u.CPU.User + u.CPU.System + u.CPU.Other)
	v := tile.Values{
		"cpu":  {Pct: cpu, Text: tile.Pct(cpu), Level: infLvl(cpu)},
		"ram":  {Pct: float64(u.Mem.Real), Text: tile.Pct(float64(u.Mem.Real)), Level: infLvl(float64(u.Mem.Real)), Sub: infBytes(float64(u.Mem.Total-u.Mem.Avail)*1024) + " / " + infBytes(float64(u.Mem.Total)*1024)},
		"load": {Text: tile.Num(float64(u.CPU.M1) / 100), Sub: tile.Num(float64(u.CPU.M5)/100) + " · " + tile.Num(float64(u.CPU.M15)/100) + " (5 et 15 min)"},
	}
	for _, n := range u.Net {
		if n.Dev == "total" {
			v["net"] = tile.Value{Text: "↓ " + tile.Rate(float64(n.Rx)), Sub: "↑ " + tile.Rate(float64(n.Tx))}
		}
	}
	return v, nil
}

func (s *synology) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var sys struct {
		Model  string `json:"model"`
		FW     string `json:"firmware_ver"`
		Temp   infNum `json:"sys_temp"`
		Warn   bool   `json:"temperature_warning"`
		Uptime string `json:"up_time"`
	}
	if s.call(ctx, "SYNO.Core.System", 1, "info", &sys) == nil {
		v["model"] = tile.Value{Text: sys.Model}
		v["firmware"] = tile.Value{Text: sys.FW}
		if sys.Temp > 0 {
			lv := tile.Level(float64(sys.Temp), 70, 85)
			if sys.Warn {
				lv = "crit"
			}
			v["temp"] = tile.Value{Text: tile.Num(float64(sys.Temp)) + " °C", Level: lv}
		}
		// DSM renvoie des secondes ou « h:m:s » selon la version
		if n, err := strconv.ParseFloat(sys.Uptime, 64); err == nil {
			v["uptime"] = tile.Value{Text: tile.Dur(time.Duration(n) * time.Second)}
		} else if sys.Uptime != "" {
			v["uptime"] = tile.Value{Text: sys.Uptime}
		}
	}
	var st struct {
		Volumes []struct {
			ID     string `json:"id"`
			Path   string `json:"vol_path"`
			Status string `json:"status"`
			Size   struct {
				Total infNum `json:"total"`
				Used  infNum `json:"used"`
			} `json:"size"`
		} `json:"volumes"`
		Disks []struct {
			Name   string `json:"name"`
			Model  string `json:"model"`
			Temp   infNum `json:"temp"`
			Status string `json:"status"`
			Smart  string `json:"smart_status"`
		} `json:"disks"`
	}
	if s.call(ctx, "SYNO.Storage.CGI.Storage", 1, "load_info", &st) == nil {
		var bars []tile.Bar
		for _, x := range st.Volumes {
			p := infPct(float64(x.Size.Used), float64(x.Size.Total))
			lv := infLvl(p)
			if x.Status != "normal" {
				lv = "crit"
			}
			name := strings.TrimPrefix(x.Path, "/")
			if name == "" {
				name = x.ID
			}
			bars = append(bars, tile.Bar{Label: name, Pct: p, Level: lv, Text: infBytes(float64(x.Size.Used)) + " / " + infBytes(float64(x.Size.Total))})
		}
		if len(bars) > 0 {
			v["volumes"] = tile.Value{Bars: bars}
		}
		tab := tile.Value{Head: []string{"Disque", "Modèle", "Température", "État"}}
		for _, d := range st.Disks {
			lv := "ok"
			if d.Status != "normal" || (d.Smart != "" && d.Smart != "normal") {
				lv = "crit"
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: d.Name}, {Text: strings.TrimSpace(d.Model)}, {Text: tile.Num(float64(d.Temp)) + " °C", Level: tile.Level(float64(d.Temp), 50, 60)}, {Text: d.Status, Level: lv}})
		}
		if len(tab.Rows) > 0 {
			tab.Rows = infRows(tab.Rows)
			v["disktable"] = tab
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}
