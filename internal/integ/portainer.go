package integ

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Portainer : API REST, en-tête X-API-Key (jeton d'accès créé dans le profil
// de l'utilisateur). Les comptes de conteneurs viennent des instantanés
// d'endpoints, donc sans appel Docker par environnement.
type portainer struct {
	url, key string
	hc       *http.Client
}

func newPortainer(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	k, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &portainer{url: strings.TrimRight(t.URL, "/") + "/api", key: k, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (p *portainer) Fields() []tile.Field {
	return []tile.Field{
		tile.F("running", "Conteneurs en marche", tile.Stat, tile.OnTile),
		tile.F("unhealthy", "Conteneurs en défaut", tile.Stat, tile.OnTile),
		tile.F("endpoints", "Environnements", tile.Stat, tile.OnTile),
		tile.F("stacks", "Stacks actives", tile.Stat, tile.OnTile),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("envtable", "Détail par environnement", tile.Table, tile.InDetail),
		tile.F("stacktable", "Stacks", tile.Table, tile.InDetail),
	}
}

func (p *portainer) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, p.hc, p.url+path, out, func(r *http.Request) { r.Header.Set("X-API-Key", p.key) })
}

func (p *portainer) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := p.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (p *portainer) poll(ctx context.Context) (tile.Values, error) {
	var eps []struct {
		ID        int    `json:"Id"`
		Name      string `json:"Name"`
		Status    int    `json:"Status"` // 1 = up, 2 = down
		Snapshots []struct {
			Running   int `json:"RunningContainerCount"`
			Stopped   int `json:"StoppedContainerCount"`
			Healthy   int `json:"HealthyContainerCount"`
			Unhealthy int `json:"UnhealthyContainerCount"`
			Stacks    int `json:"StackCount"`
		} `json:"Snapshots"`
	}
	if err := p.get(ctx, "/endpoints", &eps); err != nil {
		return nil, err
	}
	run, stop, bad, up := 0, 0, 0, 0
	tab := tile.Value{Head: []string{"Environnement", "État", "En marche", "Arrêtés", "En défaut"}}
	for _, e := range eps {
		r, s, u := 0, 0, 0
		if len(e.Snapshots) > 0 {
			r, s, u = e.Snapshots[0].Running, e.Snapshots[0].Stopped, e.Snapshots[0].Unhealthy
		}
		st, lv := "hors ligne", "crit"
		if e.Status == 1 {
			st, lv = "en ligne", "ok"
			up++
		}
		run, stop, bad = run+r, stop+s, bad+u
		tab.Rows = append(tab.Rows, []tile.Cell{{Text: e.Name}, {Text: st, Level: lv}, {Text: strconv.Itoa(r)}, {Text: strconv.Itoa(s)}, {Text: strconv.Itoa(u)}})
	}
	tab.Rows = infRows(tab.Rows)
	v := tile.Values{
		"running":   {Text: strconv.Itoa(run), Sub: fmt.Sprintf("%d arrêtés", stop)},
		"unhealthy": {Text: strconv.Itoa(bad), Level: map[bool]string{true: "crit", false: "ok"}[bad > 0]},
		"endpoints": {Text: fmt.Sprintf("%d / %d", up, len(eps)), Sub: "en ligne", Level: map[bool]string{true: "ok", false: "warn"}[up == len(eps)]},
		"envtable":  tab,
	}
	// secondaires : leur échec n'efface rien
	var st []struct {
		Name     string `json:"Name"`
		Status   int    `json:"Status"` // 1 = active, 2 = inactive
		Endpoint int    `json:"EndpointId"`
	}
	if p.get(ctx, "/stacks", &st) == nil {
		act := 0
		stt := tile.Value{Head: []string{"Stack", "État"}}
		for _, s := range st {
			text, lv := "inactive", "warn"
			if s.Status == 1 {
				act++
				text, lv = "active", "ok"
			}
			stt.Rows = append(stt.Rows, []tile.Cell{{Text: s.Name}, {Text: text, Level: lv}})
		}
		stt.Rows = infRows(stt.Rows)
		v["stacks"] = tile.Value{Text: strconv.Itoa(act), Sub: fmt.Sprintf("sur %d", len(st))}
		v["stacktable"] = stt
	}
	var sys struct {
		Version string `json:"Version"`
	}
	if p.get(ctx, "/system/status", &sys) == nil && sys.Version != "" {
		v["version"] = tile.Value{Text: sys.Version}
	}
	return v, nil
}
