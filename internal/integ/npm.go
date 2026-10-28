package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Nginx Proxy Manager : POST /api/tokens (identity/secret, rôles user et pass)
// donne un jeton Bearer relancé sur 401. Utiliser de préférence un compte
// dédié avec le droit « voir » (view only) sur hôtes et certificats. Seuls des
// GET servent ensuite. Relevé toutes les 5 min.
type npm struct {
	url, user, pass string
	hc              *http.Client
	mu              sync.Mutex
	token           string
}

func newNPM(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &npm{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if n.user, err = secret("user"); err != nil {
		return nil, err
	}
	if n.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *npm) Fields() []tile.Field {
	return []tile.Field{
		tile.F("proxy", "Hôtes proxy", tile.Stat, tile.OnTile),
		tile.F("certs", "Certificats", tile.Stat, tile.OnTile),
		tile.F("expiry", "Prochaine expiration", tile.Stat, tile.OnTile),
		tile.F("other", "Redirections, flux, 404", tile.Stat, tile.InDetail),
		tile.F("cert_list", "Certificats (expiration la plus proche)", tile.Table, tile.InDetail),
		tile.F("host_list", "Hôtes désactivés", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (n *npm) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{"identity": n.user, "secret": n.pass})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url+"/tokens", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connexion refusée (HTTP %d)", resp.StatusCode)
	}
	var r struct{ Token string }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil || r.Token == "" {
		return errors.New("connexion : jeton absent de la réponse")
	}
	n.mu.Lock()
	n.token = r.Token
	n.mu.Unlock()
	return nil
}

func (n *npm) get(ctx context.Context, path string, out any) error {
	do := func() error {
		n.mu.Lock()
		tok := n.token
		n.mu.Unlock()
		return getJSON(ctx, n.hc, n.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) })
	}
	err := do()
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusUnauthorized {
		if err = n.login(ctx); err == nil {
			err = do()
		}
	}
	return err
}

func (n *npm) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 5*time.Minute, func() {
		v, err := n.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// npmTime lit expires_on (« 2026-12-01 10:00:00 » ou ISO selon la version).
func npmTime(s string) time.Time {
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (n *npm) poll(ctx context.Context) (tile.Values, error) {
	var hosts []struct {
		DomainNames []string `json:"domain_names"`
		Forward     string   `json:"forward_host"`
		Port        int      `json:"forward_port"`
		Enabled     any
	}
	if err := n.get(ctx, "/nginx/proxy-hosts", &hosts); err != nil {
		return nil, err
	}
	on, off := 0, 0
	var offRows [][]tile.Cell
	for _, h := range hosts {
		// enabled : booléen ou 0/1 selon la version.
		if e, _ := h.Enabled.(bool); e || h.Enabled == float64(1) {
			on++
			continue
		}
		off++
		if len(offRows) < 15 {
			offRows = append(offRows, []tile.Cell{{Text: strings.Join(h.DomainNames, ", ")}, {Text: fmt.Sprintf("%s:%d", h.Forward, h.Port)}})
		}
	}
	pv := tile.Value{Text: fmt.Sprintf("%d actifs", on)}
	if off > 0 {
		pv.Sub = fmt.Sprintf("%d désactivés", off)
	}
	v := tile.Values{"proxy": pv}
	if off > 0 {
		v["host_list"] = tile.Value{Head: []string{"Domaines", "Destination"}, Rows: offRows}
	}

	var certs []struct {
		Name     string   `json:"nice_name"`
		Domains  []string `json:"domain_names"`
		Expires  string   `json:"expires_on"`
		Provider string
	}
	if n.get(ctx, "/nginx/certificates", &certs) == nil {
		sort.SliceStable(certs, func(i, j int) bool { return npmTime(certs[i].Expires).Before(npmTime(certs[j].Expires)) })
		v["certs"] = tile.Value{Text: fmt.Sprint(len(certs))}
		var rows [][]tile.Cell
		for _, c := range certs {
			t := npmTime(c.Expires)
			exp, lvl := "-", ""
			if !t.IsZero() {
				d := time.Until(t)
				exp = "dans " + tile.Dur(d)
				lvl = "ok"
				switch {
				case d < 0:
					exp, lvl = "expiré", "crit"
				case d < 7*24*time.Hour:
					lvl = "crit"
				case d < 21*24*time.Hour:
					lvl = "warn"
				}
				if len(rows) == 0 {
					v["expiry"] = tile.Value{Text: exp, Sub: c.Name, Level: lvl}
					v["certs"] = tile.Value{Text: fmt.Sprint(len(certs)), Level: lvl}
				}
			}
			if len(rows) < 15 {
				name := c.Name
				if name == "" {
					name = strings.Join(c.Domains, ", ")
				}
				rows = append(rows, []tile.Cell{{Text: name}, {Text: c.Provider}, {Text: exp, Level: lvl}})
			}
		}
		v["cert_list"] = tile.Value{Head: []string{"Certificat", "Fournisseur", "Expire"}, Rows: rows}
	}

	var redir, streams, dead []json.RawMessage
	e1 := n.get(ctx, "/nginx/redirection-hosts", &redir)
	e2 := n.get(ctx, "/nginx/streams", &streams)
	e3 := n.get(ctx, "/nginx/dead-hosts", &dead)
	if e1 == nil && e2 == nil && e3 == nil {
		v["other"] = tile.Value{Text: fmt.Sprintf("%d / %d / %d", len(redir), len(streams), len(dead))}
	}
	var ver struct {
		Version struct{ Major, Minor, Revision int }
	}
	if n.get(ctx, "/", &ver) == nil && ver.Version.Major > 0 {
		v["version"] = tile.Value{Text: fmt.Sprintf("%d.%d.%d", ver.Version.Major, ver.Version.Minor, ver.Version.Revision)}
	}
	return v, nil
}
