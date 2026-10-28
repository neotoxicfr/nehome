package integ

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// MikroTik (REST RouterOS 7, GET seulement). Chaque requête écrit une ligne
// « logged in » dans le journal du routeur : débit WAN toutes les 5 s, le reste
// toutes les 60 s.
type mikrotik struct {
	url, user, pass, wan string
	hc                   *http.Client

	mu       sync.Mutex
	lastRx   float64
	lastTx   float64
	lastAt   time.Time
	down, up *tile.Ring
}

type rosMap = map[string]string

func newMikroTik(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	m := &mikrotik{url: strings.TrimRight(t.URL, "/") + "/rest", hc: httpClient(t.InsecureTLS, false), wan: t.Options["wan"], down: tile.NewRing(60), up: tile.NewRing(60)}
	if m.wan == "" {
		m.wan = "ether1"
	}
	var err error
	if m.user, err = secret("user"); err != nil {
		return nil, err
	}
	if m.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *mikrotik) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "WAN réception", tile.Spark, tile.OnTile),
		tile.F("up", "WAN envoi", tile.Spark, tile.OnTile),
		tile.F("cpu", "CPU", tile.Meter, tile.OnTile),
		tile.F("temp", "Température", tile.Stat, tile.OnTile),
		tile.F("uptime", "En marche depuis", tile.Stat, tile.OnTile),
		tile.F("leases", "Baux DHCP", tile.Stat, tile.InDetail),
		tile.F("ram", "Mémoire", tile.Meter, tile.InDetail),
		tile.F("hdd", "Stockage", tile.Meter, tile.InDetail),
		tile.F("version", "RouterOS", tile.Stat, tile.InDetail),
		tile.F("wanip", "IP publique", tile.Stat, tile.InDetail),
		tile.F("ddns", "DDNS", tile.Stat, tile.InDetail),
		tile.F("ipv6", "Préfixe IPv6 (LAN)", tile.Stat, tile.InDetail),
		tile.F("socks", "SOCKS", tile.Stat, tile.InDetail),
		tile.F("psu", "Alimentation", tile.Stat, tile.InDetail),
		tile.F("wifi", "Clients Wi-Fi", tile.Table, tile.InDetail),
		tile.F("wg", "Pairs WireGuard", tile.Table, tile.InDetail),
		tile.F("clients", "Baux DHCP actifs", tile.Table, tile.InDetail),
		tile.F("logs", "Journal (avertissements, erreurs)", tile.Table, tile.InDetail),
	}
}

func (m *mikrotik) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, m.hc, m.url+"/"+path, out, func(r *http.Request) { r.SetBasicAuth(m.user, m.pass) })
}

func (m *mikrotik) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Second, func() {
		v, err := m.traffic(ctx)
		if ctx.Err() == nil && (v != nil || err != nil) {
			e[0](v, err)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := m.slow(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func (m *mikrotik) traffic(ctx context.Context) (tile.Values, error) {
	var ifs []rosMap
	if err := m.get(ctx, "interface?name="+m.wan+"&.proplist=rx-byte,tx-byte", &ifs); err != nil {
		return nil, err
	}
	if len(ifs) == 0 {
		return nil, fmt.Errorf("interface WAN %q introuvable", m.wan)
	}
	rx, tx, now := atof(ifs[0]["rx-byte"]), atof(ifs[0]["tx-byte"]), time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	first := m.lastAt.IsZero()
	dt := now.Sub(m.lastAt).Seconds()
	drx, dtx := rx-m.lastRx, tx-m.lastTx
	m.lastRx, m.lastTx, m.lastAt = rx, tx, now
	if first || dt <= 0 || drx < 0 || dtx < 0 { // premier point ou compteur remis à zéro (reboot)
		return nil, nil
	}
	return tile.Values{
		"down": {Text: tile.Rate(drx / dt), Points: m.down.Push(drx / dt)},
		"up":   {Text: tile.Rate(dtx / dt), Points: m.up.Push(dtx / dt)},
	}, nil
}

var rosDurRe = regexp.MustCompile(`(\d+)([wdhms])`)

// rosDur lit une durée RouterOS (« 2d7h43m41s », « 1w3d »).
func rosDur(s string) time.Duration {
	var d time.Duration
	unit := map[string]time.Duration{"w": 7 * 24 * time.Hour, "d": 24 * time.Hour, "h": time.Hour, "m": time.Minute, "s": time.Second}
	for _, p := range rosDurRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(p[1])
		d += time.Duration(n) * unit[p[2]]
	}
	return d
}

func (m *mikrotik) slow(ctx context.Context) (tile.Values, error) {
	var res rosMap
	if err := m.get(ctx, "system/resource", &res); err != nil {
		return nil, err
	}
	v := tile.Values{}
	cpu := atof(res["cpu-load"])
	v["cpu"] = tile.Value{Pct: cpu, Text: tile.Pct(cpu), Level: tile.Level(cpu, 75, 90)}
	if tot := atof(res["total-memory"]); tot > 0 {
		used := tot - atof(res["free-memory"])
		v["ram"] = tile.Value{Pct: used / tot * 100, Text: tile.Bytes(used) + " / " + tile.Bytes(tot)}
	}
	if tot := atof(res["total-hdd-space"]); tot > 0 {
		used := tot - atof(res["free-hdd-space"])
		v["hdd"] = tile.Value{Pct: used / tot * 100, Text: tile.Bytes(used) + " / " + tile.Bytes(tot)}
	}
	v["uptime"] = tile.Value{Text: tile.Dur(rosDur(res["uptime"]))}

	// Le reste est facultatif : un menu en erreur n'efface pas la tuile.
	ver := tile.Value{Text: res["version"]}
	var upd rosMap
	if m.get(ctx, "system/package/update", &upd) == nil {
		if lv := upd["latest-version"]; lv != "" && lv != upd["installed-version"] {
			ver.Sub, ver.Level = "mise à jour "+lv+" disponible", "warn"
		}
	}
	v["version"] = ver

	var health []rosMap
	if m.get(ctx, "system/health", &health) == nil {
		v["temp"] = tile.Value{Text: "—"} // capteur absent : pas de valeur périmée
		for _, h := range health {
			switch h["name"] {
			case "cpu-temperature":
				t := atof(h["value"])
				v["temp"] = tile.Value{Text: tile.Num(t) + " °C", Level: tile.Level(t, 70, 85)}
			case "psu1-voltage":
				v["psu"] = tile.Value{Text: strings.Replace(h["value"], ".", ",", 1) + " V"}
			}
		}
	}

	var cloud rosMap
	if m.get(ctx, "ip/cloud", &cloud) == nil {
		v["wanip"] = tile.Value{Text: cloud["public-address"], Sub: cloud["public-address-ipv6"]}
		lvl := "ok"
		if cloud["status"] != "updated" {
			lvl = "warn"
		}
		v["ddns"] = tile.Value{Text: cloud["dns-name"], Sub: cloud["status"], Level: lvl}
	}

	var addrs []rosMap
	if m.get(ctx, "ipv6/address", &addrs) == nil {
		v["ipv6"] = tile.Value{Text: "—"} // adresse retirée : pas de valeur périmée
		for _, a := range addrs {
			if a["global"] == "true" && a["disabled"] != "true" && a["interface"] == "bridge" {
				v["ipv6"] = tile.Value{Text: a["address"], Sub: "valide " + tile.Dur(rosDur(a["valid"]))}
			}
		}
	}

	var socks rosMap
	if m.get(ctx, "ip/socks", &socks) == nil {
		if socks["enabled"] == "true" {
			v["socks"] = tile.Value{Text: "actif, port " + socks["port"], Sub: "v" + socks["version"] + ", auth " + socks["auth-method"]}
		} else {
			v["socks"] = tile.Value{Text: "inactif"}
		}
	}

	hosts := map[string]string{}
	var leases []rosMap
	if m.get(ctx, "ip/dhcp-server/lease", &leases) == nil {
		var rows [][]tile.Cell
		for _, l := range leases {
			name := l["host-name"]
			if name == "" {
				name = l["comment"]
			}
			hosts[strings.ToUpper(l["mac-address"])] = name
			if l["status"] == "bound" {
				rows = append(rows, []tile.Cell{{Text: name}, {Text: l["active-address"]}, {Text: l["expires-after"]}})
			}
		}
		v["leases"] = tile.Value{Text: fmt.Sprint(len(rows)), Sub: fmt.Sprintf("sur %d", len(leases))}
		v["clients"] = tile.Value{Head: []string{"Nom", "Adresse", "Expire dans"}, Rows: rows}
	}

	var reg []rosMap
	if m.get(ctx, "interface/wifi/registration-table", &reg) == nil {
		var rows [][]tile.Cell
		for _, r := range reg {
			name := hosts[strings.ToUpper(r["mac-address"])]
			if name == "" {
				name = r["mac-address"]
			}
			sig := atof(r["signal"])
			rows = append(rows, []tile.Cell{{Text: name}, {Text: r["interface"]}, {Text: r["signal"] + " dBm", Level: sigLevel(sig)}, {Text: r["tx-rate"]}, {Text: tile.Dur(rosDur(r["uptime"]))}})
		}
		v["wifi"] = tile.Value{Head: []string{"Appareil", "Radio", "Signal", "Débit", "Connecté"}, Rows: rows, Text: "aucun client"}
	}

	var peers []rosMap
	if m.get(ctx, "interface/wireguard/peers", &peers) == nil {
		var rows [][]tile.Cell
		for _, p := range peers {
			name := p["name"]
			if name == "" {
				name = p["comment"]
			}
			hs, lvl := "jamais", "warn"
			if s := p["last-handshake"]; s != "" {
				d := rosDur(s)
				hs, lvl = "il y a "+tile.Dur(d), ""
				if d > 3*time.Minute {
					lvl = "warn"
				} else {
					lvl = "ok"
				}
			}
			if p["disabled"] == "true" {
				hs, lvl = "désactivé", ""
			}
			rows = append(rows, []tile.Cell{{Text: name}, {Text: p["current-endpoint-address"]}, {Text: hs, Level: lvl}, {Text: "↓ " + tile.Bytes(atof(p["rx"])) + " · ↑ " + tile.Bytes(atof(p["tx"]))}})
		}
		v["wg"] = tile.Value{Head: []string{"Pair", "Point d'accès", "Dernier contact", "Volume"}, Rows: rows}
	}

	var logs []rosMap
	if m.get(ctx, "log", &logs) == nil {
		var rows [][]tile.Cell
		for i := len(logs) - 1; i >= 0 && len(rows) < 15; i-- {
			l := logs[i]
			topics := l["topics"]
			lvl := ""
			switch {
			case strings.Contains(topics, "critical") || strings.Contains(topics, "error"):
				lvl = "crit"
			case strings.Contains(topics, "warning"):
				lvl = "warn"
			default:
				continue
			}
			rows = append(rows, []tile.Cell{{Text: l["time"]}, {Text: l["message"], Level: lvl}})
		}
		v["logs"] = tile.Value{Head: []string{"Heure", "Message"}, Rows: rows, Text: "rien à signaler"}
	}
	return v, nil
}

func sigLevel(dbm float64) string {
	switch {
	case dbm == 0:
		return ""
	case dbm >= -65:
		return "ok"
	case dbm >= -75:
		return "warn"
	}
	return "crit"
}
