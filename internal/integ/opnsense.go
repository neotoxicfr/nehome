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

// OPNsense : API REST, clé + secret en auth basique (System > Access > Users >
// clé d'API). Lectures seules (diagnostics, routes/gateway/status, firmware/status).
type opnsense struct {
	url, key, sec string
	hc            *http.Client
}

func newOPNsense(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	o := &opnsense{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if o.key, err = secret("key"); err != nil {
		return nil, err
	}
	if o.sec, err = secret("secret"); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *opnsense) Fields() []tile.Field {
	return []tile.Field{
		tile.F("gateways", "Passerelles", tile.Stat, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("load", "Charge", tile.Stat, tile.OnTile),
		tile.F("firmware", "Mises à jour", tile.Stat, tile.OnTile),
		tile.F("uptime", "Uptime", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("gwtable", "Passerelles (détail)", tile.Table, tile.InDetail),
		tile.F("ifaces", "Interfaces", tile.Table, tile.InDetail),
		tile.F("disks", "Disques", tile.Bars, tile.InDetail),
	}
}

func (o *opnsense) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, o.hc, o.url+path, out, func(r *http.Request) { r.SetBasicAuth(o.key, o.sec) })
}

func (o *opnsense) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 10*time.Minute, func() { // le firmware est un cache côté pare-feu
		if v := o.firmware(ctx); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, 30*time.Second, func() {
		v, err := o.live(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (o *opnsense) live(ctx context.Context) (tile.Values, error) {
	// requête principale : passerelles (porte l'état de la connexion)
	var gw struct {
		Items []struct {
			Name   string `json:"name"`
			Addr   string `json:"address"`
			Status string `json:"status"`
			Loss   string `json:"loss"`
			Delay  string `json:"delay"`
		} `json:"items"`
	}
	if err := o.get(ctx, "/routes/gateway/status", &gw); err != nil {
		return nil, err
	}
	tab := tile.Value{Head: []string{"Passerelle", "Adresse", "État", "Latence", "Perte"}}
	up := 0
	for _, g := range gw.Items {
		lv := "crit"
		switch g.Status {
		case "none", "online":
			lv = "ok"
			up++
		case "delay", "loss", "force_down":
			lv = "warn"
		}
		tab.Rows = append(tab.Rows, []tile.Cell{{Text: g.Name}, {Text: g.Addr}, {Text: g.Status, Level: lv}, {Text: g.Delay}, {Text: g.Loss}})
	}
	tab.Rows = infRows(tab.Rows)
	gv := tile.Value{Text: fmt.Sprintf("%d / %d", up, len(gw.Items)), Sub: "en ligne", Level: "ok"}
	if up < len(gw.Items) {
		gv.Level = "crit"
	}
	v := tile.Values{"gwtable": tab}
	if len(gw.Items) > 0 {
		v["gateways"] = gv
	}

	var t struct {
		Uptime  string `json:"uptime"`
		LoadAvg string `json:"loadavg"`
	}
	if o.get(ctx, "/diagnostics/system/systemTime", &t) == nil {
		if t.Uptime != "" {
			v["uptime"] = tile.Value{Text: t.Uptime}
		}
		if f := strings.Split(t.LoadAvg, ","); len(f) == 3 {
			v["load"] = tile.Value{Text: strings.TrimSpace(f[0]), Sub: strings.TrimSpace(f[1]) + " · " + strings.TrimSpace(f[2]) + " (5 et 15 min)"}
		}
	}
	var m struct {
		Memory struct {
			Total infNum `json:"total"`
			Used  infNum `json:"used"`
		} `json:"memory"`
	}
	if o.get(ctx, "/diagnostics/system/systemResources", &m) == nil && m.Memory.Total > 0 {
		p := infPct(float64(m.Memory.Used), float64(m.Memory.Total))
		v["ram"] = tile.Value{Pct: p, Text: tile.Pct(p), Level: infLvl(p), Sub: infBytes(float64(m.Memory.Used)) + " / " + infBytes(float64(m.Memory.Total))}
	}
	var d struct {
		Devices []struct {
			Mount string `json:"mountpoint"`
			Used  infNum `json:"used_pct"`
			Size  string `json:"blocks"`
		} `json:"devices"`
	}
	if o.get(ctx, "/diagnostics/system/systemDisk", &d) == nil && len(d.Devices) > 0 {
		var bars []tile.Bar
		for _, x := range d.Devices {
			bars = append(bars, tile.Bar{Label: x.Mount, Pct: float64(x.Used), Level: infLvl(float64(x.Used)), Text: tile.Pct(float64(x.Used)) + " · " + x.Size})
		}
		v["disks"] = tile.Value{Bars: bars[:min(len(bars), 15)]}
	}
	var ifs struct {
		Rows []struct {
			Device string `json:"device"`
			Descr  string `json:"description"`
			Status string `json:"status"`
			Addr4  string `json:"addr4"`
		} `json:"rows"`
	}
	if o.get(ctx, "/interfaces/overview/interfacesInfo", &ifs) == nil && len(ifs.Rows) > 0 {
		it := tile.Value{Head: []string{"Interface", "Appareil", "État", "IPv4"}}
		for _, r := range ifs.Rows {
			lv := ""
			if r.Status == "up" {
				lv = "ok"
			}
			it.Rows = append(it.Rows, []tile.Cell{{Text: r.Descr}, {Text: r.Device}, {Text: r.Status, Level: lv}, {Text: r.Addr4}})
		}
		it.Rows = infRows(it.Rows)
		v["ifaces"] = it
	}
	return v, nil
}

func (o *opnsense) firmware(ctx context.Context) tile.Values {
	var f struct {
		Status      string `json:"status"`
		Msg         string `json:"status_msg"`
		NeedsReboot string `json:"needs_reboot"`
		Product     struct {
			Version string `json:"product_version"`
		} `json:"product"`
		Version string `json:"product_version"`
		New     []any  `json:"new_packages"`
		Upgrade []any  `json:"upgrade_packages"`
	}
	if o.get(ctx, "/core/firmware/status", &f) != nil {
		return nil
	}
	n := len(f.New) + len(f.Upgrade)
	fw := tile.Value{Text: "à jour", Level: "ok", Sub: f.Msg}
	if n > 0 || f.Status == "update" || f.Status == "upgrade" {
		fw = tile.Value{Text: fmt.Sprintf("%d paquets", n), Level: "warn", Sub: f.Msg}
	}
	if f.NeedsReboot == "1" {
		fw.Sub, fw.Level = "redémarrage requis", "warn"
	}
	v := tile.Values{"firmware": fw}
	ver := f.Product.Version
	if ver == "" {
		ver = f.Version
	}
	if ver != "" {
		v["version"] = tile.Value{Text: ver}
	}
	return v
}
