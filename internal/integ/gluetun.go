package integ

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Gluetun (serveur de contrôle v3). Sans auth tant que le fichier de rôles
// n'en impose pas ; une tuile par instance, l'URL en config.
type gluetun struct {
	url, key string // key : rôle apikey du serveur de contrôle (facultatif, exigé pour PUT)
	hc       *http.Client
}

func newGluetun(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g := &gluetun{url: strings.TrimRight(t.URL, "/") + "/v1", hc: httpClient(t.InsecureTLS, false)}
	if _, ok := t.Secrets["key"]; ok {
		var err error
		if g.key, err = secret("key"); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *gluetun) auth(r *http.Request) {
	if g.key != "" {
		r.Header.Set("X-API-Key", g.key)
	}
}

func (g *gluetun) Fields() []tile.Field {
	return []tile.Field{
		tile.F("vpn", "VPN", tile.Stat, tile.OnTile),
		tile.F("ip", "IP publique", tile.Stat, tile.OnTile),
		tile.F("location", "Localisation", tile.Stat, tile.OnTile),
		tile.F("port", "Port transféré", tile.Stat, tile.OnTile),
		tile.F("org", "Opérateur", tile.Stat, tile.InDetail),
		tile.F("hostname", "Nom d'hôte", tile.Stat, tile.InDetail),
		tile.F("timezone", "Fuseau horaire", tile.Stat, tile.InDetail),
		tile.F("dns", "DNS du VPN", tile.Stat, tile.InDetail),
	}
}

func (g *gluetun) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, g.hc, g.url+"/"+path, out, g.auth)
}

// Actions : sans clé, gluetun (≥ v3.40) refuse PUT par défaut ; pas de bouton.
func (g *gluetun) Actions() []tile.Action {
	if g.key == "" {
		return nil
	}
	return []tile.Action{{ID: "vpnrestart", Label: "Relancer le VPN", Confirm: "Couper puis relancer le tunnel VPN ? Les téléchargements s'interrompent quelques secondes."}}
}

// Do : PUT /v1/vpn/status {"status":"stopped"} puis {"status":"running"}
// (serveur de contrôle v3). La relance est tentée même si l'arrêt échoue.
func (g *gluetun) Do(ctx context.Context, id, _ string) (string, error) {
	if id != "vpnrestart" || g.key == "" {
		return "", fmt.Errorf("action inconnue : %s", id)
	}
	errStop := g.setVPN(ctx, "stopped")
	if err := g.setVPN(ctx, "running"); err != nil {
		return "", err
	}
	if errStop != nil {
		return "VPN relancé (l'arrêt a échoué : " + errStop.Error() + ")", nil
	}
	return "VPN relancé", nil
}

func (g *gluetun) setVPN(ctx context.Context, status string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	u := g.url + "/vpn/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, strings.NewReader(`{"status":"`+status+`"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	g.auth(req)
	hc := *g.hc
	hc.Timeout = 0 // borné par le contexte : l'arrêt du tunnel peut durer
	resp, err := hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New("gluetun refuse PUT /v1/vpn/status (clé « key » absente ou rôle sans PUT /v1/vpn/status)")
	case resp.StatusCode/100 != 2:
		return statusError{resp.StatusCode, redact(u)}
	}
	return nil
}

func (g *gluetun) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := g.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (g *gluetun) poll(ctx context.Context) (tile.Values, error) {
	var vpn struct {
		Status string `json:"status"`
	}
	if err := g.get(ctx, "vpn/status", &vpn); err != nil {
		return nil, err
	}
	v := tile.Values{}
	if vpn.Status == "running" {
		v["vpn"] = tile.Value{Text: "connecté", Level: "ok"}
	} else {
		v["vpn"] = tile.Value{Text: vpn.Status, Level: "crit"}
	}
	var ip struct {
		Region, Country, City, Hostname, Organization, Timezone string
		Pub                                                     string `json:"public_ip"`
		Postal                                                  string `json:"postal_code"`
	}
	if g.get(ctx, "publicip/ip", &ip) == nil {
		v["ip"] = tile.Value{Text: ip.Pub}
		loc := tile.Value{Text: ip.Country}
		if ip.City != "" {
			loc.Text = ip.City + ", " + ip.Country
		}
		loc.Sub = ip.Region
		v["location"] = loc
		v["org"] = tile.Value{Text: ip.Organization}
		v["hostname"] = tile.Value{Text: ip.Hostname}
		v["timezone"] = tile.Value{Text: ip.Timezone}
	} else if vpn.Status == "running" {
		v["ip"] = tile.Value{Text: "inconnue"} // l'IP n'est pas encore résolue
	} else { // VPN coupé : ne pas laisser l'ancienne IP affichée
		v["ip"], v["location"] = tile.Value{Text: "—"}, tile.Value{Text: "—"}
		v["port"] = tile.Value{Text: "—"}
	}
	var pf struct {
		Port  int   `json:"port"`
		Ports []int `json:"ports"`
	}
	if g.get(ctx, "portforward", &pf) == nil {
		ports := pf.Ports
		if len(ports) == 0 && pf.Port > 0 {
			ports = []int{pf.Port}
		}
		if len(ports) == 0 {
			v["port"] = tile.Value{Text: "aucun"}
		} else {
			s := make([]string, len(ports))
			for i, p := range ports {
				s[i] = strconv.Itoa(p)
			}
			v["port"] = tile.Value{Text: strings.Join(s, ", "), Level: "ok"}
		}
	}
	var dns struct {
		Status string `json:"status"`
	}
	if g.get(ctx, "dns/status", &dns) == nil {
		v["dns"] = tile.Value{Text: dns.Status}
	}
	return v, nil
}
