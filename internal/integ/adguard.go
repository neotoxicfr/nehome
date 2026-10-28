package integ

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// AdGuard Home (API /control, auth basique ; seules les actions écrivent). Une tuile par
// instance (NAS, Raspberry Pi, site distant…) : seule l'URL et les secrets changent.
type adguard struct {
	url, user, pass string
	hc              *http.Client
}

func newAdGuard(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	a := &adguard{url: strings.TrimRight(t.URL, "/") + "/control", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if a.user, err = secret("user"); err != nil {
		return nil, err
	}
	if a.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *adguard) Fields() []tile.Field {
	return []tile.Field{
		tile.F("queries", "Requêtes (24 h)", tile.Stat, tile.OnTile),
		tile.F("blocked", "Bloquées", tile.Meter, tile.OnTile),
		tile.F("curve", "Requêtes par heure", tile.Spark, tile.OnTile),
		tile.F("protection", "Protection", tile.Stat, tile.OnTile),
		tile.F("avg", "Temps moyen de traitement", tile.Stat, tile.InDetail),
		tile.F("upstream", "Serveur amont", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("rules", "Règles de filtrage", tile.Stat, tile.InDetail),
		tile.F("clients", "Clients les plus actifs", tile.Table, tile.InDetail),
		tile.F("domains", "Domaines les plus interrogés", tile.Table, tile.InDetail),
		tile.F("blockedtop", "Domaines les plus bloqués", tile.Table, tile.InDetail),
		tile.F("lists", "Listes de filtrage", tile.Table, tile.InDetail),
	}
}

func (a *adguard) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, a.hc, a.url+"/"+path, out, func(r *http.Request) { r.SetBasicAuth(a.user, a.pass) })
}

func (a *adguard) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 10*time.Minute, func() { // listes : changent au plus une fois par jour
		if v := a.lists(ctx); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := a.live(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

// agTop : les « top » d'AdGuard sont des listes d'objets à une seule clé.
type agTop []map[string]float64

func (t agTop) table(head string, n int) tile.Value {
	v := tile.Value{Head: []string{head, "Requêtes"}}
	for _, m := range t {
		for k, c := range m {
			if len(v.Rows) < n {
				v.Rows = append(v.Rows, []tile.Cell{{Text: k}, {Text: netInt(c)}})
			}
		}
	}
	if len(v.Rows) == 0 {
		v.Text = "aucun"
	}
	return v
}

// netInt : entier avec espace fine insécable entre les milliers.
func netInt(f float64) string {
	s := strconv.FormatInt(int64(f+0.5), 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 && c != '-' && s[i-1] != '-' {
			b.WriteString(" ")
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (a *adguard) live(ctx context.Context) (tile.Values, error) {
	var st struct {
		Version    string  `json:"version"`
		Running    bool    `json:"running"`
		Protection bool    `json:"protection_enabled"`
		DisabledMs float64 `json:"protection_disabled_duration"`
	}
	if err := a.get(ctx, "status", &st); err != nil {
		return nil, err
	}
	v := tile.Values{"version": {Text: st.Version}}
	switch {
	case !st.Running:
		v["protection"] = tile.Value{Text: "arrêtée", Level: "crit"}
	case !st.Protection:
		p := tile.Value{Text: "désactivée", Level: "crit"}
		if st.DisabledMs > 0 {
			p.Sub = "reprise dans " + tile.Dur(time.Duration(st.DisabledMs)*time.Millisecond)
		}
		v["protection"] = p
	default:
		v["protection"] = tile.Value{Text: "active", Level: "ok"}
	}

	var s struct {
		Queries   float64   `json:"num_dns_queries"`
		Blocked   float64   `json:"num_blocked_filtering"`
		Safe      float64   `json:"num_replaced_safebrowsing"`
		Parental  float64   `json:"num_replaced_parental"`
		SafeSrch  float64   `json:"num_replaced_safesearch"`
		Avg       float64   `json:"avg_processing_time"`
		Hourly    []float64 `json:"dns_queries"`
		Clients   agTop     `json:"top_clients"`
		Domains   agTop     `json:"top_queried_domains"`
		BlockedTp agTop     `json:"top_blocked_domains"`
		UpAvg     agTop     `json:"top_upstreams_avg_time"`
	}
	// Les statistiques sont secondaires : la tuile garde l'état de protection.
	if err := a.get(ctx, "stats", &s); err != nil {
		return v, nil
	}
	blocked := s.Blocked + s.Safe + s.Parental
	v["queries"] = tile.Value{Text: netInt(s.Queries), Sub: netInt(blocked) + " bloquées"}
	pct := 0.0
	if s.Queries > 0 {
		pct = blocked / s.Queries * 100
	}
	v["blocked"] = tile.Value{Pct: pct, Text: tile.Pct(pct), Sub: netInt(blocked) + " requêtes"}
	v["curve"] = tile.Value{Points: s.Hourly, Text: netInt(s.Queries)}
	v["avg"] = tile.Value{Text: tile.Num(s.Avg*1000) + " ms"}
	if len(s.UpAvg) > 0 {
		for k, t := range s.UpAvg[0] {
			v["upstream"] = tile.Value{Text: k, Sub: "réponse moyenne " + tile.Num(t*1000) + " ms"}
		}
	}
	v["clients"] = s.Clients.table("Client", 15)
	v["domains"] = s.Domains.table("Domaine", 15)
	v["blockedtop"] = s.BlockedTp.table("Domaine", 15)
	return v, nil
}

func (a *adguard) lists(ctx context.Context) tile.Values {
	var f struct {
		Filters []struct {
			Name    string `json:"name"`
			Updated string `json:"last_updated"`
			Rules   int    `json:"rules_count"`
			Enabled bool   `json:"enabled"`
		} `json:"filters"`
	}
	if a.get(ctx, "filtering/status", &f) != nil {
		return nil
	}
	tab := tile.Value{Head: []string{"Liste", "Règles", "Mise à jour"}}
	total, n := 0, 0
	for _, l := range f.Filters {
		if !l.Enabled {
			continue
		}
		total += l.Rules
		n++
		if len(tab.Rows) < 15 {
			ago := ""
			if t, err := time.Parse(time.RFC3339, l.Updated); err == nil {
				ago = tile.Ago(t)
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: l.Name}, {Text: netInt(float64(l.Rules))}, {Text: ago}})
		}
	}
	return tile.Values{
		"rules": {Text: netInt(float64(total)), Sub: fmt.Sprintf("%d listes actives", n)},
		"lists": tab,
	}
}

func (a *adguard) Actions() []tile.Action {
	return []tile.Action{
		{ID: "pause10", Label: "Suspendre 10 min"},
		{ID: "pause60", Label: "Suspendre 1 h"},
		{ID: "resume", Label: "Réactiver"},
	}
}

// Do : POST /control/protection (openapi AdGuard Home, SetProtectionRequest :
// duration en millisecondes, seulement avec enabled=false).
func (a *adguard) Do(ctx context.Context, id, _ string) (string, error) {
	var body map[string]any
	var msg string
	switch id {
	case "pause10":
		body, msg = map[string]any{"enabled": false, "duration": 10 * 60 * 1000}, "Protection suspendue 10 min"
	case "pause60":
		body, msg = map[string]any{"enabled": false, "duration": 60 * 60 * 1000}, "Protection suspendue 1 h"
	case "resume":
		body, msg = map[string]any{"enabled": true}, "Protection réactivée"
	default:
		return "", errUnknownAction
	}
	err := callAction(ctx, a.hc, http.MethodPost, a.url+"/protection", body, func(r *http.Request) { r.SetBasicAuth(a.user, a.pass) })
	var se statusError
	if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusMethodNotAllowed) {
		return "", errors.New("AdGuard Home trop ancien : route /control/protection absente, mettez-le à jour")
	}
	if err != nil {
		return "", err
	}
	return msg, nil
}
