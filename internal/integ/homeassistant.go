package integ

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Home Assistant : API REST, jeton d'accès longue durée (Bearer). Lecture seule
// (GET /api/config et /api/states). options.entities : entity_id à afficher,
// séparés par des virgules.
type homeassistant struct {
	url, token string
	watch      []string
	hc         *http.Client
}

func newHomeAssistant(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	tok, err := secret("token")
	if err != nil {
		return nil, err
	}
	h := &homeassistant{url: strings.TrimRight(t.URL, "/") + "/api", token: tok, hc: httpClient(t.InsecureTLS, false)}
	for _, e := range strings.Split(t.Options["entities"], ",") {
		if e = strings.TrimSpace(e); e != "" {
			h.watch = append(h.watch, e)
		}
	}
	return h, nil
}

func (h *homeassistant) Fields() []tile.Field {
	return []tile.Field{
		tile.F("watched", "Entités suivies", tile.Table, tile.OnTile),
		tile.F("entities", "Entités", tile.Stat, tile.OnTile),
		tile.F("unavailable", "Entités indisponibles", tile.Stat, tile.OnTile),
		tile.F("state", "État du système", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("domains", "Entités par domaine", tile.Table, tile.InDetail),
		tile.F("unavailablelist", "Entités indisponibles (liste)", tile.Table, tile.InDetail),
	}
}

func (h *homeassistant) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, h.hc, h.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+h.token) })
}

func (h *homeassistant) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := h.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (h *homeassistant) poll(ctx context.Context) (tile.Values, error) {
	var st []struct {
		ID    string `json:"entity_id"`
		State string `json:"state"`
		Attr  struct {
			Name string `json:"friendly_name"`
			Unit string `json:"unit_of_measurement"`
		} `json:"attributes"`
	}
	if err := h.get(ctx, "/states", &st); err != nil {
		return nil, err
	}
	byDom := map[string]int{}
	bad := tile.Value{Head: []string{"Entité", "État"}}
	nbad := 0
	idx := map[string]int{}
	for i, s := range st {
		dom, _, _ := strings.Cut(s.ID, ".")
		byDom[dom]++
		idx[s.ID] = i
		if s.State == "unavailable" {
			nbad++
			if len(bad.Rows) < 15 {
				n := s.Attr.Name
				if n == "" {
					n = s.ID
				}
				bad.Rows = append(bad.Rows, []tile.Cell{{Text: n}, {Text: "indisponible", Level: "warn"}})
			}
		}
	}
	doms := make([]string, 0, len(byDom))
	for d := range byDom {
		doms = append(doms, d)
	}
	sort.Slice(doms, func(i, j int) bool {
		return byDom[doms[i]] > byDom[doms[j]] || byDom[doms[i]] == byDom[doms[j]] && doms[i] < doms[j]
	})
	dt := tile.Value{Head: []string{"Domaine", "Entités"}}
	for _, d := range doms {
		dt.Rows = append(dt.Rows, []tile.Cell{{Text: d}, {Text: strconv.Itoa(byDom[d])}})
	}
	dt.Rows = infRows(dt.Rows)
	un := tile.Value{Text: "aucune", Level: "ok"}
	if nbad > 0 {
		un = tile.Value{Text: strconv.Itoa(nbad), Level: "warn"}
	}
	v := tile.Values{
		"entities":        {Text: strconv.Itoa(len(st)), Sub: strconv.Itoa(len(byDom)) + " domaines"},
		"unavailable":     un,
		"domains":         dt,
		"unavailablelist": bad,
	}
	if len(h.watch) > 0 {
		w := tile.Value{Head: []string{"Entité", "État"}}
		for _, id := range h.watch {
			i, ok := idx[id]
			if !ok {
				w.Rows = append(w.Rows, []tile.Cell{{Text: id}, {Text: "introuvable", Level: "warn"}})
				continue
			}
			s := st[i]
			n := s.Attr.Name
			if n == "" {
				n = id
			}
			txt, lv := s.State, ""
			if s.State == "unavailable" {
				txt, lv = "indisponible", "warn"
			} else if s.Attr.Unit != "" {
				txt += " " + s.Attr.Unit
			}
			w.Rows = append(w.Rows, []tile.Cell{{Text: n}, {Text: txt, Level: lv}})
		}
		v["watched"] = w
	}
	var cfg struct {
		Version string `json:"version"`
		State   string `json:"state"`
		Name    string `json:"location_name"`
	}
	if h.get(ctx, "/config", &cfg) == nil {
		v["version"] = tile.Value{Text: cfg.Version, Sub: cfg.Name}
		lv := "ok"
		if cfg.State != "RUNNING" {
			lv = "warn"
		}
		v["state"] = tile.Value{Text: strings.ToLower(cfg.State), Level: lv}
	}
	return v, nil
}
