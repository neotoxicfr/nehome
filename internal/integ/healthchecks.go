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

// Healthchecks (healthchecks.io ou auto-hébergé) : GET /api/v3/checks/, clé
// API en LECTURE SEULE (rôle key, Paramètres du projet > Clés API), en-tête
// X-Api-Key. Pour le service hébergé, url = https://healthchecks.io.
// Relevé toutes les 30 s.
type healthchecks struct {
	url, key string
	hc       *http.Client
}

func newHealthchecks(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	h := &healthchecks{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	var err error
	if h.key, err = secret("key"); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *healthchecks) Fields() []tile.Field {
	return []tile.Field{
		tile.F("state", "État", tile.Stat, tile.OnTile),
		tile.F("down", "En panne", tile.Stat, tile.OnTile),
		tile.F("up", "En ligne", tile.Stat, tile.OnTile),
		tile.F("grace", "En délai de grâce", tile.Stat, tile.InDetail),
		tile.F("paused", "En pause", tile.Stat, tile.InDetail),
		tile.F("checks", "Checks (pannes d'abord)", tile.Table, tile.InDetail),
	}
}

func (h *healthchecks) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := h.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (h *healthchecks) poll(ctx context.Context) (tile.Values, error) {
	var r struct {
		Checks []struct {
			Name     string
			Status   string
			LastPing *time.Time `json:"last_ping"`
			NextPing *time.Time `json:"next_ping"`
			Tags     string
		}
	}
	err := getJSON(ctx, h.hc, h.url+"/api/v3/checks/", &r, func(q *http.Request) { q.Header.Set("X-Api-Key", h.key) })
	if err != nil {
		return nil, err
	}
	count := map[string]int{}
	rank := map[string]int{"down": 0, "grace": 1, "new": 2, "paused": 3, "up": 4}
	cs := r.Checks
	for _, c := range cs {
		count[c.Status]++
	}
	sort.SliceStable(cs, func(i, j int) bool { return rank[cs[i].Status] < rank[cs[j].Status] })
	var rows [][]tile.Cell
	for _, c := range cs {
		if len(rows) == 15 {
			break
		}
		lvl, label := "", c.Status
		switch c.Status {
		case "down":
			lvl, label = "crit", "En panne"
		case "grace":
			lvl, label = "warn", "Délai de grâce"
		case "up":
			lvl, label = "ok", "En ligne"
		case "paused":
			label = "En pause"
		case "new":
			label = "Nouveau"
		}
		last := "jamais"
		if c.LastPing != nil {
			last = tile.Ago(*c.LastPing)
		}
		rows = append(rows, []tile.Cell{{Text: c.Name}, {Text: label, Level: lvl}, {Text: last}, {Text: c.Tags}})
	}
	state := tile.Value{Text: "Tout va bien", Level: "ok", Sub: fmt.Sprintf("%d checks", len(cs))}
	switch {
	case count["down"] > 0:
		state = tile.Value{Text: fmt.Sprintf("%d en panne", count["down"]), Level: "crit", Sub: fmt.Sprintf("%d checks", len(cs))}
	case count["grace"] > 0:
		state = tile.Value{Text: fmt.Sprintf("%d en retard", count["grace"]), Level: "warn", Sub: fmt.Sprintf("%d checks", len(cs))}
	}
	down := tile.Value{Text: fmt.Sprint(count["down"]), Level: "ok"}
	if count["down"] > 0 {
		down.Level = "crit"
	}
	return tile.Values{
		"state":  state,
		"down":   down,
		"up":     {Text: fmt.Sprint(count["up"])},
		"grace":  {Text: fmt.Sprint(count["grace"])},
		"paused": {Text: fmt.Sprint(count["paused"])},
		"checks": {Head: []string{"Check", "État", "Dernier ping", "Étiquettes"}, Rows: rows},
	}, nil
}
