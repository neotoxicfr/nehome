package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// CrowdSec (LAPI, compte « machine »). Le jeton de login dure environ une
// heure ; /v1/decisions est réservé aux bouncers (403 pour une machine) : les
// décisions actives se lisent donc dans les alertes (has_active_decision).
type crowdsec struct {
	url, machine, pass string
	hc                 *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
	bans  []string // IP bannies localement, choix de l'action « unban »
}

func newCrowdSec(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	c := &crowdsec{url: strings.TrimRight(t.URL, "/") + "/v1", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if c.machine, err = secret("user"); err != nil {
		return nil, err
	}
	if c.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *crowdsec) Fields() []tile.Field {
	return []tile.Field{
		tile.F("alerts", "Alertes (24 h)", tile.Stat, tile.OnTile),
		tile.F("decisions", "Bannissements actifs", tile.Stat, tile.OnTile),
		tile.F("appsec", "Alertes AppSec (24 h)", tile.Stat, tile.OnTile),
		tile.F("curve", "Alertes par heure", tile.Spark, tile.InDetail),
		tile.F("countries", "Pays sources", tile.Table, tile.InDetail),
		tile.F("scenarios", "Scénarios", tile.Table, tile.InDetail),
		tile.F("recent", "Dernières alertes", tile.Table, tile.InDetail),
		tile.F("banned", "Bannissements en cours", tile.Table, tile.InDetail),
	}
}

type csAlert struct {
	Kind      string `json:"kind"`
	Scenario  string `json:"scenario"`
	CreatedAt string `json:"created_at"`
	Decisions []struct {
		Duration string `json:"duration"`
		Type     string `json:"type"`
		Origin   string `json:"origin"`
	} `json:"decisions"`
	Source struct {
		IP     string `json:"ip"`
		Value  string `json:"value"`
		CN     string `json:"cn"`
		ASName string `json:"as_name"`
	} `json:"source"`
}

// login renouvelle le jeton s'il expire dans moins de 5 min.
func (c *crowdsec) login(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.exp) > 5*time.Minute {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{"machine_id": c.machine, "password": c.pass})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/watchers/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return "", statusError{resp.StatusCode, c.url + "/watchers/login"}
	}
	var r struct {
		Token  string `json:"token"`
		Expire string `json:"expire"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil || r.Token == "" {
		return "", fmt.Errorf("login CrowdSec : réponse inattendue")
	}
	c.token = r.Token
	c.exp, _ = time.Parse(time.RFC3339, r.Expire)
	if c.exp.IsZero() {
		c.exp = time.Now().Add(30 * time.Minute)
	}
	return c.token, nil
}

func (c *crowdsec) get(ctx context.Context, path string, out any) error {
	for try := 0; ; try++ {
		tok, err := c.login(ctx)
		if err != nil {
			return err
		}
		err = getJSON(ctx, c.hc, c.url+"/"+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) })
		if se, ok := err.(statusError); ok && se.Code == http.StatusUnauthorized && try == 0 {
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			continue
		}
		return err
	}
}

func (c *crowdsec) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 2*time.Minute, func() { // deux listes jusqu'à 1 000 alertes : 2 min suffisent pour un compteur sur 24 h
		v, err := c.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// csTable trie un compteur par valeur décroissante (puis nom, pour la stabilité).
func csTable(count map[string]int, head string, n int) tile.Value {
	type kv struct {
		k string
		n int
	}
	var l []kv
	for k, v := range count {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].n > l[j].n || l[i].n == l[j].n && l[i].k < l[j].k })
	v := tile.Value{Head: []string{head, "Alertes"}}
	for i, e := range l {
		if i == n {
			break
		}
		v.Rows = append(v.Rows, []tile.Cell{{Text: e.k}, {Text: netInt(float64(e.n))}})
	}
	return v
}

func (c *crowdsec) poll(ctx context.Context) (tile.Values, error) {
	var day []csAlert
	if err := c.get(ctx, "alerts?since=24h&limit=1000", &day); err != nil {
		return nil, err
	}
	countries, scenarios := map[string]int{}, map[string]int{}
	hours := make([]float64, 24)
	local, appsec := 0, 0
	rec := tile.Value{Head: []string{"Quand", "Scénario", "IP", "Pays", "Bans"}}
	sort.SliceStable(day, func(i, j int) bool { return day[i].CreatedAt > day[j].CreatedAt })
	for _, a := range day {
		if a.Kind == "capi" { // import de la liste communautaire, pas une attaque vue ici
			continue
		}
		local++
		isWAF := a.Kind == "waf" || strings.Contains(a.Scenario, "appsec")
		if isWAF {
			appsec++
		}
		cn := a.Source.CN
		if cn == "" {
			cn = "?"
		}
		countries[cn]++
		scenarios[strings.TrimPrefix(a.Scenario, "crowdsecurity/")]++
		t, _ := time.Parse(time.RFC3339, a.CreatedAt)
		if h := int(time.Since(t).Hours()); h >= 0 && h < 24 {
			hours[23-h]++
		}
		if len(rec.Rows) < 15 {
			ip := a.Source.IP
			if ip == "" {
				ip = a.Source.Value
			}
			ago := strings.TrimPrefix(tile.Ago(t), "il y a ")
			scen := strings.TrimPrefix(a.Scenario, "crowdsecurity/")
			if isWAF {
				scen += " (AppSec)"
			}
			rec.Rows = append(rec.Rows, []tile.Cell{{Text: ago}, {Text: scen}, {Text: ip}, {Text: cn}, {Text: netInt(float64(len(a.Decisions)))}})
		}
	}
	v := tile.Values{
		"alerts":    {Text: netInt(float64(local)), Sub: fmt.Sprintf("%d hors AppSec", local-appsec)},
		"appsec":    {Text: netInt(float64(appsec))},
		"curve":     {Points: hours, Text: netInt(float64(local))},
		"countries": csTable(countries, "Pays", 15),
		"scenarios": csTable(scenarios, "Scénario", 15),
		"recent":    rec,
	}

	// Secondaire : l'échec n'efface pas le reste. Les alertes « capi » (listes
	// communautaires) portent des milliers de bans : on ne compte que le local.
	var act []csAlert
	if c.get(ctx, "alerts?has_active_decision=true&limit=1000", &act) == nil {
		n := 0
		var ips []string
		ban := tile.Value{Head: []string{"IP", "Scénario", "Pays", "Durée restante"}}
		for _, a := range act {
			if a.Kind == "capi" {
				continue
			}
			if a.Source.Value != "" && !slices.Contains(ips, a.Source.Value) {
				ips = append(ips, a.Source.Value)
			}
			for _, d := range a.Decisions {
				n++
				if len(ban.Rows) < 15 {
					ban.Rows = append(ban.Rows, []tile.Cell{{Text: a.Source.Value}, {Text: strings.TrimPrefix(a.Scenario, "crowdsecurity/")}, {Text: a.Source.CN}, {Text: d.Duration}})
				}
			}
		}
		c.mu.Lock()
		c.bans = ips
		c.mu.Unlock()
		v["decisions"] = tile.Value{Text: netInt(float64(n)), Sub: "locaux (hors listes communautaires)"}
		v["banned"] = ban
	}
	return v, nil
}

func (c *crowdsec) Actions() []tile.Action {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bans) == 0 {
		return nil
	}
	return []tile.Action{{ID: "unban", Label: "Débannir une IP", Confirm: "Supprimer les décisions actives de cette IP ?", Choices: slices.Clone(c.bans)}}
}

// Do : DELETE /v1/decisions?ip= (route JWT des machines, comme cscli
// decisions delete). Seules les IP bannies localement sont proposées.
func (c *crowdsec) Do(ctx context.Context, id, ip string) (string, error) {
	c.mu.Lock()
	ok := id == "unban" && slices.Contains(c.bans, ip)
	c.mu.Unlock()
	if !ok {
		return "", errUnknownAction
	}
	for try := 0; ; try++ {
		tok, err := c.login(ctx)
		if err != nil {
			return "", err
		}
		err = callAction(ctx, c.hc, http.MethodDelete, c.url+"/decisions?ip="+url.QueryEscape(ip), nil,
			func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) })
		if se, ok := err.(statusError); ok && se.Code == http.StatusUnauthorized && try == 0 {
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			continue
		}
		if err != nil {
			return "", err
		}
		return ip + " débannie", nil
	}
}
