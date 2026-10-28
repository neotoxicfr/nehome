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

// Bazarr (sous-titres) : clé d'API en en-tête X-API-KEY. Badges et santé
// chaque minute ; version et fournisseurs toutes les 5 min.
type bazarr struct {
	url, key string
	hc       *http.Client
}

func newBazarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &bazarr{url: strings.TrimRight(t.URL, "/") + "/api", key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (b *bazarr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("episodes", "Épisodes sans sous-titres", tile.Stat, tile.OnTile),
		tile.F("movies", "Films sans sous-titres", tile.Stat, tile.OnTile),
		tile.F("providers", "Fournisseurs en pause", tile.Stat, tile.OnTile),
		tile.F("health", "Santé", tile.Stat, tile.OnTile),
		tile.F("links", "Liaison Sonarr / Radarr", tile.Stat, tile.InDetail),
		tile.F("provider_list", "Fournisseurs de sous-titres", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (b *bazarr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, b.hc, b.url+path, out, func(r *http.Request) { r.Header.Set("X-API-KEY", b.key) })
}

func (b *bazarr) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		v := b.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := b.fast(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (b *bazarr) fast(ctx context.Context) (tile.Values, error) {
	// Les compteurs « episodes » et « movies » sont les éléments sans sous-titres.
	var bd struct {
		Episodes  float64
		Movies    float64
		Providers float64
		Sonarr    string `json:"sonarr_signalr"`
		Radarr    string `json:"radarr_signalr"`
	}
	if err := b.get(ctx, "/badges", &bd); err != nil {
		return nil, err
	}
	v := tile.Values{
		"episodes": {Text: tile.Num(bd.Episodes)},
		"movies":   {Text: tile.Num(bd.Movies)},
	}
	pv := tile.Value{Text: tile.Num(bd.Providers)}
	if bd.Providers > 0 {
		pv.Level = "warn"
	}
	v["providers"] = pv
	if bd.Sonarr != "" || bd.Radarr != "" {
		lv := tile.Value{Text: "OK", Level: "ok", Sub: fmt.Sprintf("Sonarr : %s · Radarr : %s", bazarrDash(bd.Sonarr), bazarrDash(bd.Radarr))}
		if (bd.Sonarr != "" && bd.Sonarr != "LIVE") || (bd.Radarr != "" && bd.Radarr != "LIVE") {
			lv.Text, lv.Level = "Dégradée", "warn"
		}
		v["links"] = lv
	}
	var hl struct {
		Data []struct{ Object, Issue string }
	}
	if b.get(ctx, "/system/health", &hl) == nil {
		hv := tile.Value{Text: "OK", Level: "ok"}
		if n := len(hl.Data); n > 0 {
			hv = tile.Value{Text: fmt.Sprintf("%d alerte%s", n, arrPlural(n)), Level: "warn", Sub: hl.Data[0].Issue}
		}
		v["health"] = hv
	}
	return v, nil
}

func bazarrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (b *bazarr) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var st struct {
		Data struct {
			Version string  `json:"bazarr_version"`
			Sonarr  string  `json:"sonarr_version"`
			Radarr  string  `json:"radarr_version"`
			Start   float64 `json:"start_time"` // secondes Unix
		}
	}
	if b.get(ctx, "/system/status", &st) == nil {
		sub := ""
		if st.Data.Start > 0 {
			sub = "en marche depuis " + tile.Dur(time.Since(time.Unix(int64(st.Data.Start), 0)))
		}
		v["version"] = tile.Value{Text: st.Data.Version, Sub: sub}
	}
	var pr struct {
		Data []struct {
			Name   string
			Status string
			Retry  string
		}
	}
	if b.get(ctx, "/providers", &pr) == nil {
		tv := tile.Value{Head: []string{"Fournisseur", "État", "Nouvel essai"}}
		for _, p := range pr.Data {
			cl := ""
			if p.Status != "" && p.Status != "Good" {
				cl = "warn"
			}
			if len(tv.Rows) < 15 {
				tv.Rows = append(tv.Rows, []tile.Cell{{Text: p.Name}, {Text: p.Status, Level: cl}, {Text: p.Retry}})
			}
		}
		v["provider_list"] = tv
	}
	return v
}
