package integ

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Glances : API REST v4 (/api/4), repli sur v3 (/api/3) si v4 n'existe pas.
// Authentification basique facultative (rôles user et pass, ensemble).
type glances struct {
	url, user, pass string
	hc              *http.Client
	api             string // "/api/4" ou "/api/3", fixé au premier succès
	cpu             *tile.Ring
}

func newGlances(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g := &glances{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false), cpu: tile.NewRing(60)}
	if _, ok := t.Secrets["user"]; ok {
		var err error
		if g.user, err = secret("user"); err != nil {
			return nil, err
		}
		if g.pass, err = secret("pass"); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *glances) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cpu", "Processeur", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("load", "Charge", tile.Stat, tile.OnTile),
		tile.F("temp", "Température max", tile.Stat, tile.OnTile),
		tile.F("curve", "Processeur (historique)", tile.Spark, tile.InDetail),
		tile.F("swap", "Swap", tile.Meter, tile.InDetail),
		tile.F("system", "Système", tile.Stat, tile.InDetail),
		tile.F("uptime", "Uptime", tile.Stat, tile.InDetail),
		tile.F("disks", "Disques", tile.Bars, tile.InDetail),
		tile.F("net", "Réseau", tile.Table, tile.InDetail),
		tile.F("sensors", "Capteurs de température", tile.Table, tile.InDetail),
		tile.F("procs", "Processus les plus gourmands", tile.Table, tile.InDetail),
	}
}

func (g *glances) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, g.hc, g.url+g.api+path, out, func(r *http.Request) {
		if g.user != "" {
			r.SetBasicAuth(g.user, g.pass)
		}
	})
}

func (g *glances) Run(ctx context.Context, emit func(tile.Values, error)) {
	n := 0
	every(ctx, 5*time.Second, func() { // une seule goroutine : g.api n'est pas partagé
		v, err := g.fast(ctx)
		if err == nil && n%12 == 0 { // sondes lentes : une fois par minute
			for k, x := range g.slow(ctx) {
				v[k] = x
			}
		}
		n++
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (g *glances) fast(ctx context.Context) (tile.Values, error) {
	var cpu struct {
		Total float64 `json:"total"`
	}
	if g.api == "" {
		// détection : v4 d'abord, v3 si 404
		var err error
		for _, a := range []string{"/api/4", "/api/3"} {
			g.api = a
			if err = g.get(ctx, "/cpu", &cpu); err == nil {
				break
			}
			if se, ok := err.(statusError); !ok || se.Code != http.StatusNotFound {
				break
			}
		}
		if err != nil {
			g.api = ""
			return nil, err
		}
	} else if err := g.get(ctx, "/cpu", &cpu); err != nil {
		return nil, err
	}
	v := tile.Values{"cpu": {Pct: cpu.Total, Text: tile.Pct(cpu.Total), Level: infLvl(cpu.Total)}}
	v["curve"] = tile.Value{Points: g.cpu.Push(cpu.Total), Text: tile.Pct(cpu.Total)}

	var mem struct {
		Percent float64 `json:"percent"`
		Used    float64 `json:"used"`
		Total   float64 `json:"total"`
	}
	if g.get(ctx, "/mem", &mem) == nil {
		v["ram"] = tile.Value{Pct: mem.Percent, Text: tile.Pct(mem.Percent), Level: infLvl(mem.Percent), Sub: infBytes(mem.Used) + " / " + infBytes(mem.Total)}
	}
	var ld struct {
		M1  float64 `json:"min1"`
		M5  float64 `json:"min5"`
		M15 float64 `json:"min15"`
		Cpu float64 `json:"cpucore"`
	}
	if g.get(ctx, "/load", &ld) == nil {
		lv := "ok"
		if ld.Cpu > 0 {
			lv = tile.Level(ld.M5/ld.Cpu, 1, 2)
		}
		v["load"] = tile.Value{Text: tile.Num(ld.M1), Level: lv, Sub: tile.Num(ld.M5) + " · " + tile.Num(ld.M15) + " (5 et 15 min)"}
	}
	var net []struct {
		Name  string   `json:"interface_name"`
		Rx    *float64 `json:"bytes_recv_rate_per_sec"`
		Tx    *float64 `json:"bytes_sent_rate_per_sec"`
		Recv  float64  `json:"bytes_recv"` // API v3 : octets sur l'intervalle
		Sent  float64  `json:"bytes_sent"`
		Since float64  `json:"time_since_update"`
		Up    bool     `json:"is_up"`
		Speed float64  `json:"speed"`
	}
	if g.get(ctx, "/network", &net) == nil {
		tab := tile.Value{Head: []string{"Interface", "Réception", "Émission"}}
		for _, n := range net {
			if strings.HasPrefix(n.Name, "lo") || strings.HasPrefix(n.Name, "veth") {
				continue
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: n.Name}, {Text: tile.Rate(netRate(n.Rx, n.Recv, n.Since))}, {Text: tile.Rate(netRate(n.Tx, n.Sent, n.Since))}})
		}
		tab.Rows = infRows(tab.Rows)
		v["net"] = tab
	}
	return v, nil
}

func (g *glances) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var sw struct {
		Percent float64 `json:"percent"`
		Used    float64 `json:"used"`
		Total   float64 `json:"total"`
	}
	if g.get(ctx, "/memswap", &sw) == nil && sw.Total > 0 {
		v["swap"] = tile.Value{Pct: sw.Percent, Text: tile.Pct(sw.Percent), Level: infLvl(sw.Percent), Sub: infBytes(sw.Used) + " / " + infBytes(sw.Total)}
	}
	var fs []struct {
		Mnt     string  `json:"mnt_point"`
		Percent float64 `json:"percent"`
		Used    float64 `json:"used"`
		Size    float64 `json:"size"`
	}
	if g.get(ctx, "/fs", &fs) == nil && len(fs) > 0 {
		var bars []tile.Bar
		for _, d := range fs {
			if len(bars) < 15 {
				bars = append(bars, tile.Bar{Label: d.Mnt, Pct: d.Percent, Level: infLvl(d.Percent), Text: infBytes(d.Used) + " / " + infBytes(d.Size)})
			}
		}
		v["disks"] = tile.Value{Bars: bars}
	}
	var sys struct {
		Host string `json:"hostname"`
		OS   string `json:"hr_name"`
	}
	if g.get(ctx, "/system", &sys) == nil && sys.Host != "" {
		v["system"] = tile.Value{Text: sys.Host, Sub: sys.OS}
	}
	var up string
	if g.get(ctx, "/uptime", &up) == nil && up != "" {
		v["uptime"] = tile.Value{Text: up}
	}
	var sens []struct {
		Label string  `json:"label"`
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
		Type  string  `json:"type"`
	}
	if g.get(ctx, "/sensors", &sens) == nil {
		tab := tile.Value{Head: []string{"Capteur", "Température"}}
		max, ok := 0.0, false
		for _, s := range sens {
			if !strings.HasPrefix(s.Type, "temperature") || s.Unit != "C" {
				continue
			}
			ok = true
			if s.Value > max {
				max = s.Value
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: s.Label}, {Text: tile.Num(s.Value) + " °C", Level: tile.Level(s.Value, 70, 85)}})
		}
		if ok {
			tab.Rows = infRows(tab.Rows)
			v["sensors"] = tab
			v["temp"] = tile.Value{Text: tile.Num(max) + " °C", Level: tile.Level(max, 70, 85)}
		}
	}
	var ps []struct {
		Name string  `json:"name"`
		CPU  float64 `json:"cpu_percent"`
		Mem  float64 `json:"memory_percent"`
	}
	if g.get(ctx, "/processlist", &ps) == nil && len(ps) > 0 {
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].CPU > ps[j].CPU })
		tab := tile.Value{Head: []string{"Processus", "CPU", "Mémoire"}}
		for _, p := range ps {
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: p.Name}, {Text: tile.Pct(p.CPU)}, {Text: tile.Pct(p.Mem)}})
		}
		tab.Rows = tab.Rows[:min(len(tab.Rows), 10)]
		v["procs"] = tab
	}
	return v
}

// netRate : débit en octets/s ; en API v3 il n'y a que le volume de l'intervalle.
func netRate(rate *float64, bytes, since float64) float64 {
	if rate != nil {
		return *rate
	}
	if since > 0 {
		return bytes / since
	}
	return 0
}
