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

// authentik (API v3) : jeton d'API (rôle token, Répertoire > Jetons), en-tête
// Bearer. Droits minimaux : voir les utilisateurs, les applications et les
// événements. Les adresses IP des connexions échouées sont affichées, jamais
// les identifiants saisis. Relevé toutes les minutes.
type authentik struct {
	url, token string
	hc         *http.Client
}

func newAuthentik(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	a := &authentik{url: strings.TrimRight(t.URL, "/") + "/api/v3", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if a.token, err = secret("token"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *authentik) Fields() []tile.Field {
	return []tile.Field{
		tile.F("users", "Utilisateurs actifs", tile.Stat, tile.OnTile),
		tile.F("failed", "Connexions échouées (24 h)", tile.Stat, tile.OnTile),
		tile.F("apps", "Applications", tile.Stat, tile.OnTile),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("failed_list", "Dernières connexions échouées", tile.Table, tile.InDetail),
	}
}

func (a *authentik) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, a.hc, a.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+a.token) })
}

func (a *authentik) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := a.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type authentikPage struct {
	Pagination struct{ Count int }
}

func (a *authentik) poll(ctx context.Context) (tile.Values, error) {
	var users authentikPage
	if err := a.get(ctx, "/core/users/?is_active=true&page_size=1", &users); err != nil {
		return nil, err
	}
	v := tile.Values{"users": {Text: fmt.Sprint(users.Pagination.Count)}}

	var apps authentikPage
	if a.get(ctx, "/core/applications/?page_size=1", &apps) == nil {
		v["apps"] = tile.Value{Text: fmt.Sprint(apps.Pagination.Count)}
	}
	var ver struct {
		Current  string `json:"version_current"`
		Latest   string `json:"version_latest"`
		Outdated bool
	}
	if a.get(ctx, "/admin/version/", &ver) == nil && ver.Current != "" {
		vv := tile.Value{Text: ver.Current}
		if ver.Outdated {
			vv.Sub, vv.Level = "mise à jour : "+ver.Latest, "warn"
		}
		v["version"] = vv
	}
	var ev struct {
		Results []struct {
			Created  time.Time
			ClientIP string `json:"client_ip"`
			User     struct{ Username string }
			Context  struct{ Message, Stage string }
		}
	}
	if a.get(ctx, "/events/events/?action=login_failed&ordering=-created&page_size=100", &ev) == nil {
		day := 0
		var rows [][]tile.Cell
		for _, e := range ev.Results {
			if time.Since(e.Created) < 24*time.Hour {
				day++
			}
			if len(rows) < 15 {
				rows = append(rows, []tile.Cell{{Text: tile.Ago(e.Created)}, {Text: e.ClientIP}, {Text: e.Context.Stage}})
			}
		}
		fv := tile.Value{Text: fmt.Sprint(day), Level: "ok"}
		if len(ev.Results) == 100 && day == 100 {
			fv.Text = "100+"
		}
		if day >= 10 {
			fv.Level = "crit"
		} else if day > 0 {
			fv.Level = "warn"
		}
		v["failed"] = fv
		v["failed_list"] = tile.Value{Head: []string{"Quand", "Adresse IP", "Étape"}, Rows: rows}
	}
	return v, nil
}
