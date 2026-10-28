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

// Grafana : /api/health (sans authentification), /api/admin/stats (droit
// serveur : absent avec un simple compte de service, les champs restent
// masqués) et les alertes en cours de l'Alertmanager intégré. Jeton de compte
// de service (rôle key), en-tête Bearer. Relevé toutes les 30 s.
type grafana struct {
	url, key string
	hc       *http.Client
}

func newGrafana(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g := &grafana{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	var err error
	if g.key, err = secret("key"); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *grafana) Fields() []tile.Field {
	return []tile.Field{
		tile.F("health", "État", tile.Stat, tile.OnTile),
		tile.F("alerts", "Alertes en cours", tile.Stat, tile.OnTile),
		tile.F("dashboards", "Tableaux de bord", tile.Stat, tile.OnTile),
		tile.F("users", "Utilisateurs", tile.Stat, tile.InDetail),
		tile.F("datasources", "Sources de données", tile.Stat, tile.InDetail),
		tile.F("orgs", "Organisations", tile.Stat, tile.InDetail),
		tile.F("sessions", "Sessions actives", tile.Stat, tile.InDetail),
		tile.F("alert_list", "Alertes déclenchées", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (g *grafana) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, g.hc, g.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+g.key) })
}

func (g *grafana) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := g.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (g *grafana) poll(ctx context.Context) (tile.Values, error) {
	var h struct{ Database, Version string }
	if err := g.get(ctx, "/api/health", &h); err != nil {
		return nil, err
	}
	hv := tile.Value{Text: "OK", Sub: "base " + h.Database, Level: "ok"}
	if h.Database != "ok" {
		hv = tile.Value{Text: "Dégradé", Sub: "base " + h.Database, Level: "crit"}
	}
	v := tile.Values{"health": hv}
	if h.Version != "" {
		v["version"] = tile.Value{Text: h.Version}
	}

	var st struct {
		Orgs, Dashboards, Datasources, Users, ActiveSessions int
	}
	if g.get(ctx, "/api/admin/stats", &st) == nil {
		v["dashboards"] = tile.Value{Text: fmt.Sprint(st.Dashboards)}
		v["users"] = tile.Value{Text: fmt.Sprint(st.Users)}
		v["datasources"] = tile.Value{Text: fmt.Sprint(st.Datasources)}
		v["orgs"] = tile.Value{Text: fmt.Sprint(st.Orgs)}
		v["sessions"] = tile.Value{Text: fmt.Sprint(st.ActiveSessions)}
	}

	var al []struct {
		Labels      map[string]string
		Annotations map[string]string
		StartsAt    time.Time `json:"startsAt"`
		Status      struct{ State string }
	}
	if g.get(ctx, "/api/alertmanager/grafana/api/v2/alerts?active=true&silenced=false&inhibited=false", &al) == nil {
		lvl := "ok"
		if len(al) > 0 {
			lvl = "crit"
		}
		v["alerts"] = tile.Value{Text: fmt.Sprint(len(al)), Level: lvl}
		var rows [][]tile.Cell
		for _, a := range al {
			if len(rows) == 15 {
				break
			}
			sev := a.Labels["severity"]
			rows = append(rows, []tile.Cell{{Text: a.Labels["alertname"], Level: "crit"}, {Text: sev}, {Text: a.Annotations["summary"]}, {Text: tile.Ago(a.StartsAt)}})
		}
		v["alert_list"] = tile.Value{Head: []string{"Alerte", "Gravité", "Résumé", "Depuis"}, Rows: rows}
	}
	return v, nil
}
