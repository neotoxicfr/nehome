package web

import (
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/integ"
	"github.com/neotoxicfr/nehome/internal/tile"
)

type fieldArg struct {
	F       board.FieldView
	Compact bool
	TileID  string
}

type displayArgT struct {
	T     any
	Saved any
}

var funcs = template.FuncMap{
	"displayArg":   func(t, saved any) displayArgT { return displayArgT{t, saved} },
	"optLabel":     optLabel,
	"dict_compact": func(f board.FieldView, compact bool, tile string) fieldArg { return fieldArg{f, compact, tile} },
	"spark":        spark,
	"typesList":    integ.Types,
	"limit":        func(n int, rows [][]tile.Cell) [][]tile.Cell { return rows[:min(n, len(rows))] },
	"more":         func(n int, rows [][]tile.Cell) int { return max(0, len(rows)-n) },
	"pct":          func(f float64) string { return fmt.Sprintf("%.1f", min(max(f, 0), 100)) },
	"placements": func() []tile.Placement {
		return []tile.Placement{tile.OnTile, tile.InDetail, tile.Hidden}
	},
	"placementLabel": func(p tile.Placement) string {
		return map[tile.Placement]string{tile.OnTile: "Toujours visible", tile.InDetail: "Dans le détail", tile.Hidden: "Masqué"}[p]
	},
}

// spark dessine une courbe SVG (attributs seulement : la CSP interdit les
// styles en ligne). Uniquement des nombres calculés ici, donc sûr à injecter.
func spark(pts []float64) template.HTML {
	if len(pts) < 2 {
		return ""
	}
	const w, h = 120.0, 28.0
	top := slices.Max(pts)
	if top <= 0 {
		top = 1
	}
	var b strings.Builder
	step := w / float64(len(pts)-1)
	for i, p := range pts {
		fmt.Fprintf(&b, "%.1f,%.1f ", float64(i)*step, h-1-(max(p, 0)/top)*(h-2))
	}
	line := strings.TrimSpace(b.String())
	return template.HTML(fmt.Sprintf(`<svg class="spark" viewBox="0 0 %.0f %.0f" preserveAspectRatio="none" aria-hidden="true"><polyline points="%s"/><polygon points="0,%.0f %s %.0f,%.0f"/></svg>`, w, h, line, h, line, w, h))
}
