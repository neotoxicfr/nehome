package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Pi-hole v6 (API REST, session par mot de passe ou mot de passe d'application)
// ou v5 (options.api: "5", api.php avec jeton). La clé v5 passe dans l'URL, donc
// toutes les erreurs sont nettoyées par redact/cleanErr.
type pihole struct {
	url, secret string
	v5          bool
	hc          *http.Client
	sid         string
	open        bool // accès accepté (sid vide si Pi-hole n'a pas de mot de passe)
}

func newPiHole(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	p := &pihole{url: strings.TrimRight(t.URL, "/"), v5: t.Options["api"] == "5", hc: httpClient(t.InsecureTLS, false)}
	role := "pass"
	if p.v5 {
		role = "token"
	}
	var err error
	if p.secret, err = secret(role); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *pihole) Fields() []tile.Field {
	return []tile.Field{
		tile.F("queries", "Requêtes (24 h)", tile.Stat, tile.OnTile),
		tile.F("blocked", "Bloquées", tile.Meter, tile.OnTile),
		tile.F("blocking", "Blocage", tile.Stat, tile.OnTile),
		tile.F("gravity", "Domaines en liste de blocage", tile.Stat, tile.OnTile),
		tile.F("clients", "Clients actifs", tile.Stat, tile.InDetail),
		tile.F("cached", "Réponses en cache", tile.Stat, tile.InDetail),
		tile.F("forwarded", "Requêtes transmises", tile.Stat, tile.InDetail),
		tile.F("unique", "Domaines uniques", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("topclients", "Clients les plus actifs", tile.Table, tile.InDetail),
		tile.F("topdomains", "Domaines les plus interrogés", tile.Table, tile.InDetail),
		tile.F("topblocked", "Domaines les plus bloqués", tile.Table, tile.InDetail),
	}
}

func (p *pihole) Run(ctx context.Context, emit func(tile.Values, error)) {
	defer p.logout()
	every(ctx, 30*time.Second, func() {
		var v tile.Values
		var err error
		if p.v5 {
			v, err = p.poll5(ctx)
		} else {
			v, err = p.poll6(ctx)
		}
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// logout libère la place de session (sinon 429 no_seats à force de redémarrages).
func (p *pihole) logout() {
	if p.sid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.url+"/api/auth", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-FTL-SID", p.sid)
	if resp, err := p.hc.Do(req); err == nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
	}
	p.sid, p.open = "", false
}

// --- v6 ---

func (p *pihole) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"password": p.secret})
	var r struct {
		Session struct {
			Valid   bool   `json:"valid"`
			SID     string `json:"sid"`
			Message string `json:"message"`
		} `json:"session"`
	}
	if err := infPost(ctx, p.hc, p.url+"/api/auth", "application/json", body, &r, nil); err != nil {
		return err
	}
	if !r.Session.Valid {
		return fmt.Errorf("connexion refusée : %s", r.Session.Message)
	}
	p.sid, p.open = r.Session.SID, true
	return nil
}

// get6 relance un login si la session a expiré (401).
func (p *pihole) get6(ctx context.Context, path string, out any) error {
	for try := 0; ; try++ {
		if !p.open {
			if err := p.login(ctx); err != nil {
				return err
			}
		}
		err := getJSON(ctx, p.hc, p.url+path, out, func(r *http.Request) {
			if p.sid != "" {
				r.Header.Set("X-FTL-SID", p.sid)
			}
		})
		if se, ok := err.(statusError); ok && se.Code == http.StatusUnauthorized && try == 0 {
			p.sid, p.open = "", false
			continue
		}
		return err
	}
}

func phTable(head string, rows [][2]string) tile.Value {
	v := tile.Value{Head: []string{head, "Requêtes"}}
	for _, r := range rows {
		v.Rows = append(v.Rows, []tile.Cell{{Text: r[0]}, {Text: r[1]}})
	}
	v.Rows = infRows(v.Rows)
	if len(v.Rows) == 0 {
		v.Text = "aucun"
	}
	return v
}

func (p *pihole) poll6(ctx context.Context) (tile.Values, error) {
	var s struct {
		Queries struct {
			Total     float64 `json:"total"`
			Blocked   float64 `json:"blocked"`
			Percent   float64 `json:"percent_blocked"`
			Unique    float64 `json:"unique_domains"`
			Forwarded float64 `json:"forwarded"`
			Cached    float64 `json:"cached"`
		} `json:"queries"`
		Clients struct {
			Active float64 `json:"active"`
			Total  float64 `json:"total"`
		} `json:"clients"`
		Gravity struct {
			Domains float64 `json:"domains_being_blocked"`
			Last    float64 `json:"last_update"`
		} `json:"gravity"`
	}
	if err := p.get6(ctx, "/api/stats/summary", &s); err != nil {
		return nil, err
	}
	q := s.Queries
	g := tile.Value{Text: netInt(s.Gravity.Domains)}
	if s.Gravity.Last > 0 {
		g.Sub = "mise à jour " + tile.Ago(time.Unix(int64(s.Gravity.Last), 0))
	}
	v := tile.Values{
		"queries":   {Text: netInt(q.Total), Sub: netInt(q.Blocked) + " bloquées"},
		"blocked":   {Pct: q.Percent, Text: tile.Pct(q.Percent), Sub: netInt(q.Blocked) + " requêtes"},
		"gravity":   g,
		"clients":   {Text: netInt(s.Clients.Active), Sub: netInt(s.Clients.Total) + " connus"},
		"cached":    {Text: netInt(q.Cached)},
		"forwarded": {Text: netInt(q.Forwarded)},
		"unique":    {Text: netInt(q.Unique)},
	}
	var b struct {
		Blocking string  `json:"blocking"`
		Timer    float64 `json:"timer"`
	}
	if p.get6(ctx, "/api/dns/blocking", &b) == nil {
		bl := tile.Value{Text: "actif", Level: "ok"}
		if b.Blocking != "enabled" {
			bl = tile.Value{Text: "désactivé", Level: "crit"}
			if b.Timer > 0 {
				bl.Sub = "reprise dans " + tile.Dur(time.Duration(b.Timer)*time.Second)
			}
		}
		v["blocking"] = bl
	}
	var td struct {
		Domains []struct {
			Domain string  `json:"domain"`
			Count  float64 `json:"count"`
		} `json:"domains"`
	}
	for _, c := range []struct{ id, q string }{{"topdomains", "false"}, {"topblocked", "true"}} {
		td.Domains = nil
		if p.get6(ctx, "/api/stats/top_domains?count=15&blocked="+c.q, &td) == nil {
			var rows [][2]string
			for _, d := range td.Domains {
				rows = append(rows, [2]string{d.Domain, netInt(d.Count)})
			}
			v[c.id] = phTable("Domaine", rows)
		}
	}
	var tc struct {
		Clients []struct {
			IP    string  `json:"ip"`
			Name  string  `json:"name"`
			Count float64 `json:"count"`
		} `json:"clients"`
	}
	if p.get6(ctx, "/api/stats/top_clients?count=15", &tc) == nil {
		var rows [][2]string
		for _, c := range tc.Clients {
			n := c.Name
			if n == "" {
				n = c.IP
			}
			rows = append(rows, [2]string{n, netInt(c.Count)})
		}
		v["topclients"] = phTable("Client", rows)
	}
	var ver struct {
		Version struct {
			Core struct {
				Local struct {
					Version string `json:"version"`
				} `json:"local"`
			} `json:"core"`
		} `json:"version"`
	}
	if p.get6(ctx, "/api/info/version", &ver) == nil && ver.Version.Core.Local.Version != "" {
		v["version"] = tile.Value{Text: ver.Version.Core.Local.Version}
	}
	return v, nil
}

// --- v5 ---

func (p *pihole) get5(ctx context.Context, query string, out any) error {
	return getJSON(ctx, p.hc, p.url+"/admin/api.php?"+query+"&auth="+url.QueryEscape(p.secret), out, nil)
}

func (p *pihole) poll5(ctx context.Context) (tile.Values, error) {
	var s struct {
		Domains   float64 `json:"domains_being_blocked"`
		Queries   float64 `json:"dns_queries_today"`
		Blocked   float64 `json:"ads_blocked_today"`
		Percent   float64 `json:"ads_percentage_today"`
		Unique    float64 `json:"unique_domains"`
		Forwarded float64 `json:"queries_forwarded"`
		Cached    float64 `json:"queries_cached"`
		Clients   float64 `json:"unique_clients"`
		Status    string  `json:"status"`
		Gravity   struct {
			Abs float64 `json:"absolute"`
		} `json:"gravity_last_updated"`
	}
	if err := p.get5(ctx, "summaryRaw", &s); err != nil {
		return nil, err
	}
	g := tile.Value{Text: netInt(s.Domains)}
	if s.Gravity.Abs > 0 {
		g.Sub = "mise à jour " + tile.Ago(time.Unix(int64(s.Gravity.Abs), 0))
	}
	bl := tile.Value{Text: "actif", Level: "ok"}
	if s.Status != "enabled" {
		bl = tile.Value{Text: "désactivé", Level: "crit"}
	}
	v := tile.Values{
		"queries":   {Text: netInt(s.Queries), Sub: netInt(s.Blocked) + " bloquées"},
		"blocked":   {Pct: s.Percent, Text: tile.Pct(s.Percent), Sub: netInt(s.Blocked) + " requêtes"},
		"gravity":   g,
		"blocking":  bl,
		"clients":   {Text: netInt(s.Clients)},
		"cached":    {Text: netInt(s.Cached)},
		"forwarded": {Text: netInt(s.Forwarded)},
		"unique":    {Text: netInt(s.Unique)},
	}
	var top struct {
		Queries map[string]float64 `json:"top_queries"`
		Ads     map[string]float64 `json:"top_ads"`
	}
	if p.get5(ctx, "topItems=15", &top) == nil {
		v["topdomains"] = phMap("Domaine", top.Queries)
		v["topblocked"] = phMap("Domaine", top.Ads)
	}
	var src struct {
		Sources map[string]float64 `json:"top_sources"`
	}
	if p.get5(ctx, "getQuerySources=15", &src) == nil {
		v["topclients"] = phMap("Client", src.Sources)
	}
	var ver struct {
		Version string `json:"version"`
	}
	if p.get5(ctx, "version", &ver) == nil && ver.Version != "" {
		v["version"] = tile.Value{Text: ver.Version}
	}
	return v, nil
}

// phMap trie un dictionnaire nom → compte (clients v5 : « nom|ip »).
func phMap(head string, m map[string]float64) tile.Value {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] || m[keys[i]] == m[keys[j]] && keys[i] < keys[j] })
	var rows [][2]string
	for _, k := range keys {
		n, _, _ := strings.Cut(k, "|")
		rows = append(rows, [2]string{n, netInt(m[k])})
	}
	return phTable(head, rows)
}
