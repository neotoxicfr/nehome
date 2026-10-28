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

// Gotify : jeton de CLIENT (rôle token, pas celui d'une application : seul un
// client peut lire les messages), en-tête X-Gotify-Key. Seul /version est
// public. Le corps des messages peut être sensible : seuls application, titre
// et priorité sont affichés. Relevé toutes les 30 s.
type gotify struct {
	url, token string
	hc         *http.Client
}

func newGotify(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g := &gotify{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	var err error
	if g.token, err = secret("token"); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *gotify) Fields() []tile.Field {
	return []tile.Field{
		tile.F("day", "Messages (24 h)", tile.Stat, tile.OnTile),
		tile.F("last", "Dernier message", tile.Stat, tile.OnTile),
		tile.F("apps", "Applications", tile.Stat, tile.OnTile),
		tile.F("high", "Priorité haute (24 h)", tile.Stat, tile.InDetail),
		tile.F("messages", "Messages récents", tile.Table, tile.InDetail),
		tile.F("app_list", "Applications déclarées", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (g *gotify) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, g.hc, g.url+path, out, func(r *http.Request) { r.Header.Set("X-Gotify-Key", g.token) })
}

func (g *gotify) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := g.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (g *gotify) poll(ctx context.Context) (tile.Values, error) {
	var apps []struct {
		ID          int
		Name        string
		Description string
	}
	if err := g.get(ctx, "/application", &apps); err != nil {
		return nil, err
	}
	names := map[int]string{}
	var arows [][]tile.Cell
	for _, a := range apps {
		names[a.ID] = a.Name
		if len(arows) < 15 {
			arows = append(arows, []tile.Cell{{Text: a.Name}, {Text: a.Description}})
		}
	}
	v := tile.Values{
		"apps":     {Text: fmt.Sprint(len(apps))},
		"app_list": {Head: []string{"Application", "Description"}, Rows: arows},
	}

	var ver struct{ Version string }
	if g.get(ctx, "/version", &ver) == nil && ver.Version != "" {
		v["version"] = tile.Value{Text: ver.Version}
	}

	var ms struct {
		Messages []struct {
			AppID    int `json:"appid"`
			Title    string
			Message  string
			Priority int
			Date     time.Time
		}
	}
	if g.get(ctx, "/message?limit=100", &ms) == nil {
		var day, high int
		var rows [][]tile.Cell
		for _, m := range ms.Messages {
			if time.Since(m.Date) < 24*time.Hour {
				day++
				if m.Priority >= 8 {
					high++
				}
			}
			if len(rows) < 15 {
				lvl := ""
				if m.Priority >= 8 {
					lvl = "warn"
				}
				rows = append(rows, []tile.Cell{{Text: names[m.AppID]}, {Text: m.Title}, {Text: fmt.Sprint(m.Priority), Level: lvl}, {Text: tile.Ago(m.Date)}})
			}
		}
		dv := tile.Value{Text: fmt.Sprint(day)}
		if len(ms.Messages) == 100 && day == 100 {
			dv.Text = "100+"
		}
		v["day"] = dv
		v["high"] = tile.Value{Text: fmt.Sprint(high)}
		v["messages"] = tile.Value{Head: []string{"Application", "Titre", "Priorité", "Reçu"}, Rows: rows}
		if len(ms.Messages) > 0 { // l'API trie du plus récent au plus ancien
			m := ms.Messages[0]
			v["last"] = tile.Value{Text: tile.Ago(m.Date), Sub: names[m.AppID]}
		}
	}
	return v, nil
}
