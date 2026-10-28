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

// Traefik (API interne :8080, sans auth sur le réseau docker). Les métriques
// Prometheus ne sont pas exposées sur ce port (/metrics → 404) : non lues.
type traefik struct {
	url string
	hc  *http.Client
}

func newTraefik(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	return &traefik{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}, nil
}

func (x *traefik) Fields() []tile.Field {
	return []tile.Field{
		tile.F("routers", "Routeurs", tile.Stat, tile.OnTile),
		tile.F("services", "Services", tile.Stat, tile.OnTile),
		tile.F("health", "État", tile.Stat, tile.OnTile),
		tile.F("version", "Version", tile.Stat, tile.OnTile),
		tile.F("middlewares", "Middlewares", tile.Stat, tile.InDetail),
		tile.F("certs", "Certificats", tile.Stat, tile.InDetail),
		tile.F("providers", "Fournisseurs", tile.Stat, tile.InDetail),
		tile.F("entrypoints", "Points d'entrée", tile.Table, tile.InDetail),
		tile.F("badrouters", "Routeurs en erreur", tile.Table, tile.InDetail),
		tile.F("down", "Serveurs hors service", tile.Table, tile.InDetail),
	}
}

type trCount struct {
	Total    int `json:"total"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

func (c trCount) value() tile.Value {
	v := tile.Value{Text: fmt.Sprint(c.Total), Level: "ok"}
	switch {
	case c.Errors > 0:
		v.Level, v.Sub = "crit", fmt.Sprintf("%d en erreur", c.Errors)
	case c.Warnings > 0:
		v.Level, v.Sub = "warn", fmt.Sprintf("%d avertissement(s)", c.Warnings)
	}
	return v
}

func (x *traefik) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, x.hc, x.url+"/"+path, out, nil)
}

func (x *traefik) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := x.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (x *traefik) poll(ctx context.Context) (tile.Values, error) {
	var ov struct {
		HTTP struct {
			Routers, Services, Middlewares trCount
		} `json:"http"`
		Certs     trCount  `json:"certificates"`
		Providers []string `json:"providers"`
	}
	if err := x.get(ctx, "overview", &ov); err != nil {
		return nil, err
	}
	v := tile.Values{
		"routers":     ov.HTTP.Routers.value(),
		"services":    ov.HTTP.Services.value(),
		"middlewares": ov.HTTP.Middlewares.value(),
		"certs":       ov.Certs.value(),
		"providers":   {Text: strings.Join(ov.Providers, ", ")},
	}

	// Routeurs en erreur et serveurs DOWN : détail secondaire.
	bad := tile.Value{Head: []string{"Routeur", "Règle", "Erreur"}}
	var routers []struct {
		Name   string   `json:"name"`
		Rule   string   `json:"rule"`
		Status string   `json:"status"`
		Err    []string `json:"error"`
	}
	nbad := 0
	if x.get(ctx, "http/routers?per_page=1000", &routers) == nil {
		for _, r := range routers {
			if r.Status == "enabled" {
				continue
			}
			nbad++
			if len(bad.Rows) < 15 {
				bad.Rows = append(bad.Rows, []tile.Cell{{Text: r.Name, Level: "crit"}, {Text: r.Rule}, {Text: strings.Join(r.Err, " ; ")}})
			}
		}
		v["badrouters"] = bad
	}
	down := tile.Value{Head: []string{"Service", "Serveur"}}
	var svcs []struct {
		Name   string            `json:"name"`
		Status map[string]string `json:"serverStatus"`
	}
	ndown := 0
	if x.get(ctx, "http/services?per_page=1000", &svcs) == nil {
		for _, s := range svcs {
			var urls []string
			for u, st := range s.Status {
				if st != "UP" {
					urls = append(urls, u)
				}
			}
			sort.Strings(urls)
			for _, u := range urls {
				ndown++
				if len(down.Rows) < 15 {
					down.Rows = append(down.Rows, []tile.Cell{{Text: s.Name, Level: "crit"}, {Text: u}})
				}
			}
		}
		v["down"] = down
	}
	switch {
	case nbad+ndown > 0:
		v["health"] = tile.Value{Text: "dégradé", Level: "crit", Sub: fmt.Sprintf("%d routeur(s) en erreur, %d serveur(s) DOWN", nbad, ndown)}
	case ov.HTTP.Routers.Errors+ov.HTTP.Services.Errors > 0:
		v["health"] = tile.Value{Text: "erreurs", Level: "crit"}
	case ov.HTTP.Routers.Warnings+ov.HTTP.Services.Warnings > 0:
		v["health"] = tile.Value{Text: "avertissements", Level: "warn"}
	default:
		v["health"] = tile.Value{Text: "sain", Level: "ok"}
	}

	var ver struct {
		Version, Codename string
		StartDate         string `json:"startDate"`
	}
	if x.get(ctx, "version", &ver) == nil {
		s := tile.Value{Text: ver.Version, Sub: ver.Codename}
		if t, err := time.Parse(time.RFC3339, ver.StartDate); err == nil {
			s.Sub += " · démarré " + tile.Ago(t)
		}
		v["version"] = s
	}

	var eps []struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		HTTP    struct {
			Redirections struct {
				EntryPoint struct {
					To string `json:"to"`
				} `json:"entryPoint"`
			} `json:"redirections"`
			Middlewares []string `json:"middlewares"`
		} `json:"http"`
	}
	if x.get(ctx, "entrypoints", &eps) == nil {
		t := tile.Value{Head: []string{"Nom", "Adresse", "Détail"}}
		for _, e := range eps {
			d := ""
			if to := e.HTTP.Redirections.EntryPoint.To; to != "" {
				d = "redirige vers " + to
			} else if len(e.HTTP.Middlewares) > 0 {
				d = strings.Join(e.HTTP.Middlewares, ", ")
			}
			t.Rows = append(t.Rows, []tile.Cell{{Text: e.Name}, {Text: e.Address}, {Text: d}})
		}
		v["entrypoints"] = t
	}
	return v, nil
}
