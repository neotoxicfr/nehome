package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Contrôleur UniFi Network. UniFi OS (UDM, Cloud Gateway, Cloud Key Gen2+) :
// login sur /api/auth/login, API sous /proxy/network. Contrôleur logiciel
// classique : /api/login, API à la racine. Le mode est détecté au premier login
// (ou forcé par options.unifi_os: "true"/"false"). options.site : site (défaut
// « default »). Compte local dédié, rôle en lecture seule, sans MFA.
type unifi struct {
	url, user, pass, site string
	os                    string // "", "true", "false"
	prefix                string
	hc                    *http.Client
	logged                bool
}

func newUniFi(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	u := &unifi{url: strings.TrimRight(t.URL, "/"), site: t.Options["site"], os: t.Options["unifi_os"], hc: httpClient(t.InsecureTLS, true)}
	if u.site == "" {
		u.site = "default"
	}
	var err error
	if u.user, err = secret("user"); err != nil {
		return nil, err
	}
	if u.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return u, nil
}

func (u *unifi) Fields() []tile.Field {
	return []tile.Field{
		tile.F("clients", "Clients connectés", tile.Stat, tile.OnTile),
		tile.F("devices", "Appareils", tile.Stat, tile.OnTile),
		tile.F("wan", "Internet", tile.Stat, tile.OnTile),
		tile.F("throughput", "Débit WAN", tile.Stat, tile.OnTile),
		tile.F("health", "Santé par sous-système", tile.Table, tile.InDetail),
		tile.F("devtable", "Appareils (détail)", tile.Table, tile.InDetail),
		tile.F("clienttable", "Clients les plus actifs", tile.Table, tile.InDetail),
	}
}

func (u *unifi) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{"username": u.user, "password": u.pass, "remember": false})
	tries := [][2]string{{"/api/auth/login", "/proxy/network"}, {"/api/login", ""}}
	switch u.os {
	case "true":
		tries = tries[:1]
	case "false":
		tries = tries[1:]
	}
	var err error
	for _, t := range tries {
		err = infPost(ctx, u.hc, u.url+t[0], "application/json", body, &struct{}{}, nil)
		if err == nil {
			u.prefix, u.logged = t[1], true
			return nil
		}
		if se, ok := err.(statusError); !ok || se.Code != http.StatusNotFound {
			break // identifiants refusés ou panne : inutile d'essayer l'autre mode
		}
	}
	return err
}

func (u *unifi) get(ctx context.Context, path string, out any) error {
	for try := 0; ; try++ {
		if !u.logged {
			if err := u.login(ctx); err != nil {
				return err
			}
		}
		var r struct {
			Data json.RawMessage `json:"data"`
		}
		err := getJSON(ctx, u.hc, fmt.Sprintf("%s%s/api/s/%s/%s", u.url, u.prefix, u.site, path), &r, nil)
		if se, ok := err.(statusError); ok && se.Code == http.StatusUnauthorized && try == 0 {
			u.logged = false
			continue
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(r.Data, out)
	}
}

func (u *unifi) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := u.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (u *unifi) poll(ctx context.Context) (tile.Values, error) {
	var h []struct {
		Sub      string  `json:"subsystem"`
		Status   string  `json:"status"`
		Users    float64 `json:"num_user"`
		Guests   float64 `json:"num_guest"`
		APs      float64 `json:"num_ap"`
		Switches float64 `json:"num_sw"`
		Gateways float64 `json:"num_gw"`
		Adopted  float64 `json:"num_adopted"`
		Discon   float64 `json:"num_disconnected"`
		Pending  float64 `json:"num_pending"`
		Sta      float64 `json:"num_sta"`
		WanIP    string  `json:"wan_ip"`
		Rx       float64 `json:"rx_bytes-r"`
		Tx       float64 `json:"tx_bytes-r"`
		Latency  float64 `json:"latency"`
	}
	if err := u.get(ctx, "stat/health", &h); err != nil {
		return nil, err
	}
	v := tile.Values{}
	names := map[string]string{"wlan": "Wi-Fi", "lan": "Réseau filaire", "wan": "Internet (WAN)", "www": "Accès Internet", "vpn": "VPN"}
	ht := tile.Value{Head: []string{"Sous-système", "État", "Détail"}}
	for _, s := range h {
		lv := "warn"
		switch s.Status {
		case "ok":
			lv = "ok"
		case "error":
			lv = "crit"
		}
		name := names[s.Sub]
		if name == "" {
			name = s.Sub
		}
		detail := ""
		switch s.Sub {
		case "wlan":
			detail = fmt.Sprintf("%.0f clients · %.0f points d'accès", s.Users+s.Guests, s.APs)
			v["clients"] = tile.Value{Text: netInt(s.Users + s.Guests), Sub: fmt.Sprintf("%.0f invités", s.Guests)}
		case "lan":
			detail = fmt.Sprintf("%.0f clients · %.0f switchs", s.Users+s.Guests, s.Switches)
		case "wan":
			detail = s.WanIP
			wv := tile.Value{Text: "en ligne", Level: "ok", Sub: s.WanIP}
			if s.Status != "ok" {
				wv = tile.Value{Text: "problème", Level: lv, Sub: s.Status}
			}
			v["wan"] = wv
			v["throughput"] = tile.Value{Text: "↓ " + tile.Rate(s.Rx), Sub: "↑ " + tile.Rate(s.Tx)}
		case "www":
			if s.Latency > 0 {
				detail = "latence " + tile.Num(s.Latency) + " ms"
			}
		}
		ht.Rows = append(ht.Rows, []tile.Cell{{Text: name}, {Text: s.Status, Level: lv}, {Text: detail}})
	}
	v["health"] = ht

	var devs []struct {
		Name    string  `json:"name"`
		Model   string  `json:"model"`
		Type    string  `json:"type"`
		State   int     `json:"state"` // 1 = connecté
		Version string  `json:"version"`
		Uptime  float64 `json:"uptime"`
		Sta     float64 `json:"num_sta"`
		Upgr    bool    `json:"upgradable"`
	}
	if u.get(ctx, "stat/device", &devs) == nil {
		on := 0
		dt := tile.Value{Head: []string{"Appareil", "Modèle", "État", "Clients", "Uptime", "Micrologiciel"}}
		for _, d := range devs {
			st, lv := "déconnecté", "crit"
			if d.State == 1 {
				on++
				st, lv = "connecté", "ok"
			}
			fw := d.Version
			if d.Upgr {
				fw += " (mise à jour)"
			}
			dt.Rows = append(dt.Rows, []tile.Cell{{Text: d.Name}, {Text: d.Model}, {Text: st, Level: lv}, {Text: strconv.FormatFloat(d.Sta, 'f', 0, 64)}, {Text: tile.Dur(time.Duration(d.Uptime) * time.Second)}, {Text: fw}})
		}
		dt.Rows = infRows(dt.Rows)
		dv := tile.Value{Text: fmt.Sprintf("%d / %d", on, len(devs)), Sub: "connectés", Level: "ok"}
		if on < len(devs) {
			dv.Level = "crit"
		}
		v["devices"] = dv
		v["devtable"] = dt
	}

	var sta []struct {
		Name  string  `json:"name"`
		Host  string  `json:"hostname"`
		IP    string  `json:"ip"`
		Wired bool    `json:"is_wired"`
		ESSID string  `json:"essid"`
		Rx    float64 `json:"rx_bytes-r"`
		Tx    float64 `json:"tx_bytes-r"`
	}
	if u.get(ctx, "stat/sta", &sta) == nil {
		sort.SliceStable(sta, func(i, j int) bool { return sta[i].Rx+sta[i].Tx > sta[j].Rx+sta[j].Tx })
		ct := tile.Value{Head: []string{"Client", "Adresse", "Accès", "Débit"}}
		for _, c := range sta {
			n := c.Name
			if n == "" {
				n = c.Host
			}
			acc := "Wi-Fi " + c.ESSID
			if c.Wired {
				acc = "filaire"
			}
			ct.Rows = append(ct.Rows, []tile.Cell{{Text: n}, {Text: c.IP}, {Text: strings.TrimSpace(acc)}, {Text: "↓ " + tile.Rate(c.Rx) + " ↑ " + tile.Rate(c.Tx)}})
		}
		ct.Rows = infRows(ct.Rows)
		v["clienttable"] = ct
	}
	return v, nil
}
