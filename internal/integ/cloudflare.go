package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Cloudflare (API GraphQL Analytics, lecture seule : POST sur /graphql ne
// modifie rien). Jeton restreint : Analytics Lire + Pare-feu Lire (+ Zone Lire
// si l'identifiant de zone n'est pas donné en options.zone). Requêtes lourdes
// et quota limité : toutes les 5 min.
type cloudflare struct {
	base, token, zone, zoneName string
	hc                          *http.Client

	mu sync.Mutex
}

func newCloudflare(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	c := &cloudflare{base: "https://api.cloudflare.com/client/v4", zone: t.Options["zone"], zoneName: t.Options["zone_name"], hc: httpClient(t.InsecureTLS, false)}
	if t.URL != "" { // surcharge pour les tests
		c.base = strings.TrimRight(t.URL, "/")
	}
	if c.zone == "" && c.zoneName == "" {
		return nil, errors.New("options.zone (identifiant) ou options.zone_name (domaine) requis")
	}
	var err error
	if c.token, err = secret("token"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *cloudflare) Fields() []tile.Field {
	return []tile.Field{
		tile.F("requests", "Requêtes (24 h)", tile.Stat, tile.OnTile),
		tile.F("threats", "Menaces (24 h)", tile.Stat, tile.OnTile),
		tile.F("cache", "Part servie par le cache", tile.Meter, tile.OnTile),
		tile.F("bandwidth", "Bande passante (24 h)", tile.Stat, tile.OnTile),
		tile.F("curve", "Requêtes par heure", tile.Spark, tile.InDetail),
		tile.F("firewall", "Derniers événements pare-feu", tile.Table, tile.InDetail),
	}
}

func (c *cloudflare) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 5*time.Minute, func() {
		v, err := c.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// call exécute une requête REST ou GraphQL ; le jeton n'est que dans l'en-tête.
func (c *cloudflare) call(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return statusError{resp.StatusCode, c.base + path}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

func (c *cloudflare) zoneID(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zone != "" {
		return c.zone, nil
	}
	var r struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.call(ctx, http.MethodGet, "/zones?name="+c.zoneName, nil, &r); err != nil {
		return "", err
	}
	if len(r.Result) == 0 {
		return "", fmt.Errorf("zone %q introuvable (options.zone ou droit Zone Lire)", c.zoneName)
	}
	c.zone = r.Result[0].ID
	return c.zone, nil
}

const cfQuery = `query($zone: string!, $from: Time!, $to: Time!, $fwFrom: Time!) {
  viewer {
    zones(filter: {zoneTag: $zone}) {
      httpRequests1hGroups(limit: 48, orderBy: [datetime_ASC], filter: {datetime_geq: $from, datetime_lt: $to}) {
        dimensions { datetime }
        sum { requests cachedRequests threats bytes cachedBytes }
      }
      firewallEventsAdaptive(limit: 15, orderBy: [datetime_DESC], filter: {datetime_geq: $fwFrom, datetime_leq: $to}) {
        action clientCountryName clientIP clientRequestPath datetime source ruleId
      }
    }
  }
}`

type cfResp struct {
	Data struct {
		Viewer struct {
			Zones []struct {
				Hours []struct {
					Dim struct {
						Datetime string `json:"datetime"`
					} `json:"dimensions"`
					Sum struct {
						Requests, CachedRequests, Threats, Bytes, CachedBytes float64
					} `json:"sum"`
				} `json:"httpRequests1hGroups"`
				Events []struct {
					Action, ClientCountryName, ClientIP, ClientRequestPath, Datetime, Source, RuleID string
				} `json:"firewallEventsAdaptive"`
			} `json:"zones"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *cloudflare) poll(ctx context.Context) (tile.Values, error) {
	zone, err := c.zoneID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Hour).Add(time.Hour) // heure en cours incluse
	vars := map[string]string{
		"zone": zone, "to": now.Format(time.RFC3339),
		"from":   now.Add(-24 * time.Hour).Format(time.RFC3339),
		"fwFrom": now.Add(-72 * time.Hour).Format(time.RFC3339),
	}
	var r cfResp
	if err := c.call(ctx, http.MethodPost, "/graphql", map[string]any{"query": cfQuery, "variables": vars}, &r); err != nil {
		return nil, err
	}
	if len(r.Errors) > 0 { // le message de Cloudflare ne contient pas le jeton
		return nil, fmt.Errorf("Cloudflare : %s", r.Errors[0].Message)
	}
	if len(r.Data.Viewer.Zones) == 0 {
		return nil, fmt.Errorf("Cloudflare : zone inaccessible avec ce jeton")
	}
	z := r.Data.Viewer.Zones[0]
	var req, cached, thr, bw, cbw float64
	pts := make([]float64, 0, len(z.Hours))
	for _, h := range z.Hours {
		req += h.Sum.Requests
		cached += h.Sum.CachedRequests
		thr += h.Sum.Threats
		bw += h.Sum.Bytes
		cbw += h.Sum.CachedBytes
		pts = append(pts, h.Sum.Requests)
	}
	pct := 0.0
	if req > 0 {
		pct = cached / req * 100
	}
	v := tile.Values{
		"requests":  {Text: netInt(req)},
		"threats":   {Text: netInt(thr), Level: map[bool]string{true: "warn", false: "ok"}[thr > 0]},
		"cache":     {Pct: pct, Text: tile.Pct(pct)},
		"bandwidth": {Text: tile.Bytes(bw), Sub: tile.Bytes(cbw) + " en cache"},
		"curve":     {Points: pts, Text: netInt(req)},
	}
	fw := tile.Value{Head: []string{"Quand", "Action", "Pays", "Source", "Chemin"}}
	for _, e := range z.Events {
		ago := ""
		if t, err := time.Parse(time.RFC3339, e.Datetime); err == nil {
			ago = strings.TrimPrefix(tile.Ago(t), "il y a ")
		}
		lvl := ""
		if e.Action == "block" || e.Action == "managed_challenge" || e.Action == "challenge" {
			lvl = "warn"
		}
		fw.Rows = append(fw.Rows, []tile.Cell{{Text: ago}, {Text: e.Action, Level: lvl}, {Text: e.ClientCountryName}, {Text: cfSrc(e.Source, e.RuleID)}, {Text: e.ClientRequestPath}})
	}
	if len(fw.Rows) == 0 {
		fw.Text = "aucun"
	}
	v["firewall"] = fw
	return v, nil
}

// cfSrc : produit du pare-feu et début de l'identifiant de règle.
func cfSrc(source, rule string) string {
	if len(rule) > 8 {
		rule = rule[:8]
	}
	return strings.TrimSpace(source + " " + rule)
}
