package integ

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Netdata : API v1 d'un agent (toujours servie, y compris par les agents v2).
// Sans authentification par défaut ; un jeton Bearer est pris s'il est déclaré
// (rôle token, utile derrière un proxy).
type netdata struct {
	url, token string
	hc         *http.Client
	cpu        *tile.Ring
}

func newNetdata(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &netdata{url: strings.TrimRight(t.URL, "/") + "/api/v1", hc: httpClient(t.InsecureTLS, false), cpu: tile.NewRing(60)}
	if _, ok := t.Secrets["token"]; ok {
		var err error
		if n.token, err = secret("token"); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func (n *netdata) Fields() []tile.Field {
	return []tile.Field{
		tile.F("alarms", "Alarmes actives", tile.Stat, tile.OnTile),
		tile.F("cpu", "Processeur", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("curve", "Processeur (historique)", tile.Spark, tile.InDetail),
		tile.F("host", "Machine", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("children", "Machines suivies", tile.Stat, tile.InDetail),
		tile.F("alarmlist", "Alarmes", tile.Table, tile.InDetail),
	}
}

func (n *netdata) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, n.hc, n.url+path, out, func(r *http.Request) {
		if n.token != "" {
			r.Header.Set("Authorization", "Bearer "+n.token)
		}
	})
}

func (n *netdata) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 10*time.Second, func() {
		v, err := n.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// last renvoie la dernière ligne (étiquette → valeur) d'un graphique.
func (n *netdata) last(ctx context.Context, chart string) map[string]float64 {
	var r struct {
		Labels []string    `json:"labels"`
		Data   [][]float64 `json:"data"`
	}
	q := url.Values{"chart": {chart}, "after": {"-5"}, "points": {"1"}, "group": {"average"}, "format": {"json"}}
	if n.get(ctx, "/data?"+q.Encode(), &r) != nil || len(r.Data) == 0 {
		return nil
	}
	row := r.Data[len(r.Data)-1]
	m := map[string]float64{}
	for i, l := range r.Labels {
		if i > 0 && i < len(row) { // colonne 0 : horodatage
			m[l] = row[i]
		}
	}
	return m
}

func (n *netdata) poll(ctx context.Context) (tile.Values, error) {
	var info struct {
		Version string   `json:"version"`
		Mirror  []string `json:"mirrored_hosts"`
		OS      string   `json:"os_name"`
		Alarms  struct {
			Normal, Warning, Critical int
		} `json:"alarms"`
	}
	if err := n.get(ctx, "/info", &info); err != nil {
		return nil, err
	}
	a := info.Alarms
	al := tile.Value{Text: "aucune", Level: "ok", Sub: strconv.Itoa(a.Normal) + " normales"}
	if a.Critical+a.Warning > 0 {
		al = tile.Value{Text: strconv.Itoa(a.Critical + a.Warning), Level: "warn", Sub: strconv.Itoa(a.Critical) + " critiques · " + strconv.Itoa(a.Warning) + " avertissements"}
		if a.Critical > 0 {
			al.Level = "crit"
		}
	}
	v := tile.Values{"alarms": al, "version": {Text: info.Version}}
	if len(info.Mirror) > 0 {
		v["host"] = tile.Value{Text: info.Mirror[0], Sub: info.OS}
		v["children"] = tile.Value{Text: strconv.Itoa(len(info.Mirror))}
	}
	if c := n.last(ctx, "system.cpu"); c != nil {
		t := 0.0
		for _, x := range c {
			t += x
		}
		v["cpu"] = tile.Value{Pct: t, Text: tile.Pct(t), Level: infLvl(t)}
		v["curve"] = tile.Value{Points: n.cpu.Push(t), Text: tile.Pct(t)}
	}
	if m := n.last(ctx, "system.ram"); m != nil {
		tot := m["free"] + m["used"] + m["cached"] + m["buffers"]
		if tot > 0 {
			p := m["used"] / tot * 100
			v["ram"] = tile.Value{Pct: p, Text: tile.Pct(p), Level: infLvl(p), Sub: tile.Num(m["used"]) + " Mio / " + tile.Num(tot) + " Mio"}
		}
	}
	var al2 struct {
		Alarms map[string]struct {
			Name   string  `json:"name"`
			Chart  string  `json:"chart"`
			Status string  `json:"status"`
			Value  float64 `json:"value"`
			Units  string  `json:"units"`
		} `json:"alarms"`
	}
	if n.get(ctx, "/alarms", &al2) == nil {
		tab := tile.Value{Head: []string{"Alarme", "Graphique", "État", "Valeur"}}
		names := make([]string, 0, len(al2.Alarms))
		for k := range al2.Alarms {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			x := al2.Alarms[k]
			lv := "warn"
			if x.Status == "CRITICAL" {
				lv = "crit"
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: x.Name}, {Text: x.Chart}, {Text: strings.ToLower(x.Status), Level: lv}, {Text: tile.Num(x.Value) + " " + x.Units}})
		}
		tab.Rows = infRows(tab.Rows)
		v["alarmlist"] = tab
	}
	return v, nil
}
