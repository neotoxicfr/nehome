package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// OpenMediaVault 6/7 : RPC JSON sur /rpc.php. Session (cookie) obtenue par
// Session.login ; seules les méthodes de lecture sont appelées (getInformation,
// enumerateFilesystems, getList). Un compte d'administration web dédié est
// nécessaire : OMV n'a pas de rôle en lecture seule pour ces méthodes.
type omv struct {
	url, user, pass string
	hc              *http.Client
	logged          bool
}

func newOMV(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	o := &omv{url: strings.TrimRight(t.URL, "/") + "/rpc.php", hc: httpClient(t.InsecureTLS, true)}
	var err error
	if o.user, err = secret("user"); err != nil {
		return nil, err
	}
	if o.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *omv) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cpu", "Processeur", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("fs", "Systèmes de fichiers", tile.Bars, tile.OnTile),
		tile.F("smart", "Santé des disques (SMART)", tile.Stat, tile.OnTile),
		tile.F("host", "Machine", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("uptime", "Uptime", tile.Stat, tile.InDetail),
		tile.F("load", "Charge", tile.Stat, tile.InDetail),
		tile.F("updates", "Mises à jour disponibles", tile.Stat, tile.InDetail),
		tile.F("disks", "Disques", tile.Table, tile.InDetail),
	}
}

type omvResp struct {
	Response json.RawMessage `json:"response"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// rpc relance un login si la session a expiré (erreur 5001/5002 ou HTTP 401).
func (o *omv) rpc(ctx context.Context, service, method string, params, out any) error {
	for try := 0; ; try++ {
		if !o.logged && !(service == "Session" && method == "login") {
			if err := o.rpc(ctx, "Session", "login", map[string]string{"username": o.user, "password": o.pass}, nil); err != nil {
				return err
			}
			o.logged = true
		}
		body, _ := json.Marshal(map[string]any{"service": service, "method": method, "params": params, "options": nil})
		var r omvResp
		err := infPost(ctx, o.hc, o.url, "application/json", body, &r, nil)
		if se, ok := err.(statusError); ok && se.Code == http.StatusUnauthorized && try == 0 {
			o.logged = false
			continue
		}
		if err != nil {
			return err
		}
		if r.Error != nil {
			if (r.Error.Code == 5001 || r.Error.Code == 5002) && try == 0 {
				o.logged = false
				continue
			}
			return fmt.Errorf("%s.%s : %s", service, method, r.Error.Message)
		}
		if service == "Session" && method == "login" {
			var a struct {
				Authenticated bool `json:"authenticated"`
			}
			json.Unmarshal(r.Response, &a)
			if !a.Authenticated {
				return fmt.Errorf("connexion refusée")
			}
		}
		if out != nil {
			return json.Unmarshal(r.Response, out)
		}
		return nil
	}
}

func (o *omv) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		if v := o.slow(ctx); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, 15*time.Second, func() {
		v, err := o.live(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (o *omv) live(ctx context.Context) (tile.Values, error) {
	var i struct {
		Host    string `json:"hostname"`
		Version string `json:"version"`
		CPU     infNum `json:"cpuUtilization"`
		Mem     infNum `json:"memUtilization"`
		MemUsed infNum `json:"memUsed"`
		MemTot  infNum `json:"memTotal"`
		Uptime  infNum `json:"uptime"`
		Load    struct {
			M1  infNum `json:"1min"`
			M5  infNum `json:"5min"`
			M15 infNum `json:"15min"`
		} `json:"loadAverage"`
		Pkgs   infNum `json:"availablePkgUpdates"`
		Reboot bool   `json:"rebootRequired"`
	}
	if err := o.rpc(ctx, "System", "getInformation", nil, &i); err != nil {
		return nil, err
	}
	upd := tile.Value{Text: "aucune", Level: "ok"}
	if i.Pkgs > 0 {
		upd = tile.Value{Text: tile.Num(float64(i.Pkgs)), Level: "warn"}
	}
	if i.Reboot {
		upd.Sub, upd.Level = "redémarrage requis", "warn"
	}
	mem := float64(i.Mem) * 100 // fraction 0-1 (cpuUtilization est déjà en %)
	return tile.Values{
		"cpu":     {Pct: float64(i.CPU), Text: tile.Pct(float64(i.CPU)), Level: infLvl(float64(i.CPU))},
		"ram":     {Pct: mem, Text: tile.Pct(mem), Level: infLvl(mem), Sub: infBytes(float64(i.MemUsed)) + " / " + infBytes(float64(i.MemTot))},
		"host":    {Text: i.Host},
		"version": {Text: i.Version},
		"uptime":  {Text: tile.Dur(time.Duration(float64(i.Uptime)) * time.Second)},
		"load":    {Text: tile.Num(float64(i.Load.M1)), Sub: tile.Num(float64(i.Load.M5)) + " · " + tile.Num(float64(i.Load.M15)) + " (5 et 15 min)"},
		"updates": upd,
	}, nil
}

func (o *omv) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var fs []struct {
		Dev   string `json:"devicefile"`
		Label string `json:"label"`
		Mount string `json:"mountpoint"`
		Type  string `json:"type"`
		Size  infNum `json:"size"`
		Used  infNum `json:"used"`
		Perc  infNum `json:"percentage"`
		Ok    bool   `json:"mounted"`
	}
	if o.rpc(ctx, "FileSystemMgmt", "enumerateFilesystems", nil, &fs) == nil {
		var bars []tile.Bar
		for _, f := range fs {
			// seuls les systèmes montés ont une taille ; on ignore le système (/)
			if !f.Ok || f.Size <= 0 || f.Mount == "" {
				continue
			}
			p := float64(f.Perc)
			if p == 0 {
				p = infPct(float64(f.Used), float64(f.Size))
			}
			name := f.Label
			if name == "" {
				name = f.Dev
			}
			bars = append(bars, tile.Bar{Label: name, Pct: p, Level: infLvl(p), Text: infBytes(float64(f.Used)) + " / " + infBytes(float64(f.Size))})
		}
		if len(bars) > 0 {
			v["fs"] = tile.Value{Bars: bars[:min(len(bars), 15)]}
		}
	}
	var sm struct {
		Data []struct {
			Dev    string `json:"devicefile"`
			Model  string `json:"model"`
			Temp   infNum `json:"temperature"`
			Status string `json:"overallstatus"`
		} `json:"data"`
	}
	if o.rpc(ctx, "Smart", "getList", map[string]int{"start": 0, "limit": -1}, &sm) == nil && len(sm.Data) > 0 {
		tab := tile.Value{Head: []string{"Disque", "Modèle", "Température", "SMART"}}
		bad := 0
		for _, d := range sm.Data {
			lv := "ok"
			if !strings.EqualFold(d.Status, "GOOD") {
				lv = "crit"
				bad++
			}
			temp := ""
			if d.Temp > 0 {
				temp = tile.Num(float64(d.Temp)) + " °C"
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: d.Dev}, {Text: d.Model}, {Text: temp}, {Text: d.Status, Level: lv}})
		}
		tab.Rows = infRows(tab.Rows)
		v["disks"] = tab
		s := tile.Value{Text: "tous bons", Level: "ok", Sub: fmt.Sprintf("%d disques", len(sm.Data))}
		if bad > 0 {
			s = tile.Value{Text: fmt.Sprintf("%d en défaut", bad), Level: "crit", Sub: fmt.Sprintf("sur %d disques", len(sm.Data))}
		}
		v["smart"] = s
	}
	if len(v) == 0 {
		return nil
	}
	return v
}
