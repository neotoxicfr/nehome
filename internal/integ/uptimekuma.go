package integ

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Uptime Kuma 2 : pas d'API REST, on lit les pages de statut publiques
// (options.slugs, séparés par des virgules, « plex » par défaut). Avec la clé
// API facultative (rôle « key »), /metrics ajoute tous les moniteurs, même
// ceux qu'aucune page de statut ne publie.
type uptimeKuma struct {
	url, key string
	slugs    []string
	hc       *http.Client
}

func newUptimeKuma(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	u := &uptimeKuma{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	for _, s := range strings.Split(t.Options["slugs"], ",") {
		if s = strings.TrimSpace(s); s != "" {
			u.slugs = append(u.slugs, s)
		}
	}
	if len(u.slugs) == 0 {
		u.slugs = []string{"plex"}
	}
	if _, ok := t.Secrets["key"]; ok { // facultatif, mais s'il est déclaré il doit exister
		var err error
		if u.key, err = secret("key"); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (u *uptimeKuma) Fields() []tile.Field {
	return []tile.Field{
		tile.F("summary", "État", tile.Stat, tile.OnTile),
		tile.F("uptime", "Disponibilité (24 h)", tile.Stat, tile.OnTile),
		tile.F("latency", "Latence moyenne", tile.Stat, tile.OnTile),
		tile.F("incident", "Incident en cours", tile.Stat, tile.InDetail),
		tile.F("monitors", "Moniteurs (en panne en premier)", tile.Table, tile.InDetail),
		tile.F("metrics", "Tous les moniteurs (clé API)", tile.Table, tile.InDetail),
	}
}

func (u *uptimeKuma) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := u.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type kumaMon struct {
	name   string
	status int // 0 panne, 1 ok, 2 en attente, 3 maintenance
	uptime float64
	ping   float64
	hasUp  bool
}

func (m kumaMon) state() (string, string) {
	switch m.status {
	case 1:
		return "en ligne", "ok"
	case 0:
		return "en panne", "crit"
	case 2:
		return "en attente", "warn"
	}
	return "maintenance", ""
}

func (u *uptimeKuma) poll(ctx context.Context) (tile.Values, error) {
	var mons []kumaMon
	seen := map[string]bool{}
	var incident string
	var firstErr error
	for _, slug := range u.slugs {
		var page struct {
			Incidents []struct {
				Title string `json:"title"`
			} `json:"incidents"`
			Groups []struct {
				Monitors []struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"monitorList"`
			} `json:"publicGroupList"`
		}
		var hb struct {
			Beats map[string][]struct {
				Status int      `json:"status"`
				Ping   *float64 `json:"ping"`
			} `json:"heartbeatList"`
			Uptime map[string]float64 `json:"uptimeList"`
		}
		if err := getJSON(ctx, u.hc, u.url+"/api/status-page/"+slug, &page, nil); err != nil {
			firstErr = err
			continue
		}
		if err := getJSON(ctx, u.hc, u.url+"/api/status-page/heartbeat/"+slug, &hb, nil); err != nil {
			firstErr = err
			continue
		}
		if len(page.Incidents) > 0 && incident == "" {
			incident = page.Incidents[0].Title
		}
		for _, g := range page.Groups {
			for _, m := range g.Monitors {
				id := strconv.Itoa(m.ID)
				if seen[id] {
					continue
				}
				seen[id] = true
				km := kumaMon{name: m.Name, status: 2}
				if b := hb.Beats[id]; len(b) > 0 {
					last := b[len(b)-1]
					km.status = last.Status
					if last.Ping != nil {
						km.ping = *last.Ping
					}
				}
				if up, ok := hb.Uptime[id+"_24"]; ok {
					km.uptime, km.hasUp = up*100, true
				}
				mons = append(mons, km)
			}
		}
	}
	if len(mons) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.SliceStable(mons, func(i, j int) bool {
		a, b := mons[i], mons[j]
		if (a.status == 1) != (b.status == 1) {
			return a.status != 1
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})

	down, warn := 0, 0
	var upSum, pingSum float64
	var upN, pingN int
	tab := tile.Value{Head: []string{"Moniteur", "État", "Dispo 24 h", "Latence"}}
	for _, m := range mons {
		switch m.status {
		case 0:
			down++
		case 2:
			warn++
		}
		if m.hasUp {
			upSum += m.uptime
			upN++
		}
		if m.ping > 0 {
			pingSum += m.ping
			pingN++
		}
		if len(tab.Rows) < 15 {
			st, lvl := m.state()
			up, ping := "", ""
			if m.hasUp {
				up = tile.Pct(m.uptime)
			}
			if m.ping > 0 {
				ping = tile.Num(m.ping) + " ms"
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: m.name, Level: lvl}, {Text: st, Level: lvl}, {Text: up}, {Text: ping}})
		}
	}
	v := tile.Values{"monitors": tab}
	switch {
	case down > 0:
		v["summary"] = tile.Value{Text: fmt.Sprintf("%d en panne", down), Level: "crit", Sub: fmt.Sprintf("sur %d moniteurs", len(mons))}
	case warn > 0:
		v["summary"] = tile.Value{Text: fmt.Sprintf("%d en attente", warn), Level: "warn", Sub: fmt.Sprintf("sur %d moniteurs", len(mons))}
	default:
		v["summary"] = tile.Value{Text: "tout fonctionne", Level: "ok", Sub: fmt.Sprintf("%d moniteurs", len(mons))}
	}
	if upN > 0 {
		avg := upSum / float64(upN)
		v["uptime"] = tile.Value{Text: tile.Pct(avg), Level: tile.Level(100-avg, 1, 5)}
	}
	if pingN > 0 {
		v["latency"] = tile.Value{Text: tile.Num(pingSum/float64(pingN)) + " ms"}
	}
	v["incident"] = tile.Value{Text: "aucun"}
	if incident != "" {
		v["incident"] = tile.Value{Text: incident, Level: "warn"}
	}
	if u.key != "" { // secondaire
		if all, err := u.metrics(ctx); err == nil {
			v["metrics"] = all
		}
	}
	return v, nil
}

// metrics lit /metrics (Prometheus, auth basique : utilisateur vide, mot de
// passe = clé API) : monitor_status et monitor_response_time par moniteur.
func (u *uptimeKuma) metrics(ctx context.Context) (tile.Value, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url+"/metrics", nil)
	if err != nil {
		return tile.Value{}, err
	}
	req.SetBasicAuth("", u.key)
	resp, err := u.hc.Do(req)
	if err != nil {
		return tile.Value{}, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return tile.Value{}, statusError{resp.StatusCode, u.url + "/metrics"}
	}
	byName := map[string]*kumaMon{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 8<<20))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		var metric string
		switch {
		case strings.HasPrefix(line, "monitor_status{"):
			metric = "status"
		case strings.HasPrefix(line, "monitor_response_time{"):
			metric = "ping"
		default:
			continue
		}
		i, j := strings.Index(line, `monitor_name="`), strings.LastIndex(line, "}")
		if i < 0 || j < i {
			continue
		}
		name, _, _ := strings.Cut(line[i+len(`monitor_name="`):], `"`)
		val, err := strconv.ParseFloat(strings.TrimSpace(line[j+1:]), 64)
		if err != nil {
			continue
		}
		m := byName[name]
		if m == nil {
			m = &kumaMon{name: name, status: 2}
			byName[name] = m
		}
		if metric == "status" {
			m.status = int(val)
		} else if val > 0 {
			m.ping = val
		}
	}
	all := make([]kumaMon, 0, len(byName))
	for _, m := range byName {
		all = append(all, *m)
	}
	sort.Slice(all, func(i, j int) bool {
		if (all[i].status == 1) != (all[j].status == 1) {
			return all[i].status != 1
		}
		return all[i].name < all[j].name
	})
	tab := tile.Value{Head: []string{"Moniteur", "État", "Latence"}}
	for _, m := range all {
		if len(tab.Rows) == 15 {
			break
		}
		st, lvl := m.state()
		ping := ""
		if m.ping > 0 {
			ping = tile.Num(m.ping) + " ms"
		}
		tab.Rows = append(tab.Rows, []tile.Cell{{Text: m.name, Level: lvl}, {Text: st, Level: lvl}, {Text: ping}})
	}
	return tab, sc.Err()
}
