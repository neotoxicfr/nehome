package integ

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// nut : onduleur lu directement sur le serveur NUT (upsd, protocole texte,
// port 3493). « LIST VAR <ups> » se lit sans compte : aucun secret, et la
// tuile ne dépend d'aucune interface web (PeaNUT, NUT-CGI…). Lecture seule.
type nut struct {
	addr, ups string
}

func newNUT(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	n := &nut{ups: t.Options["ups"]}
	if n.ups == "" {
		n.ups = "ups"
	}
	if strings.ContainsAny(n.ups, " \t\r\n\"") {
		return nil, fmt.Errorf("options.ups invalide : %q", n.ups)
	}
	host, port := t.Options["host"], "3493"
	if host == "" {
		return nil, errors.New("options.host manquant (adresse du serveur NUT)")
	}
	if p := t.Options["port"]; p != "" {
		if v, err := strconv.Atoi(p); err != nil || v < 1 || v > 65535 {
			return nil, fmt.Errorf("options.port invalide : %q", p)
		}
		port = p
	}
	n.addr = net.JoinHostPort(host, port)
	return n, nil
}

func (n *nut) Fields() []tile.Field {
	return []tile.Field{
		tile.F("status", "État", tile.Stat, tile.OnTile),
		tile.F("charge", "Batterie", tile.Meter, tile.OnTile),
		tile.F("runtime", "Autonomie", tile.Stat, tile.OnTile),
		tile.F("load", "Charge", tile.Meter, tile.OnTile),
		tile.F("input", "Tension d'entrée", tile.Stat, tile.InDetail),
		tile.F("output", "Tension de sortie", tile.Stat, tile.InDetail),
		tile.F("battv", "Tension batterie", tile.Stat, tile.InDetail),
		tile.F("test", "Dernier autotest", tile.Stat, tile.InDetail),
		tile.F("model", "Modèle", tile.Stat, tile.InDetail),
	}
}

func (n *nut) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		vars, err := n.list(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			emit(nil, err)
			return
		}
		emit(nutValues(vars), nil)
	})
}

// list : LIST VAR puis LOGOUT, 5 s en tout.
func (n *nut) list(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", n.addr)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer conn.Close()
	dl, _ := ctx.Deadline()
	conn.SetDeadline(dl)
	if _, err := fmt.Fprintf(conn, "LIST VAR %s\nLOGOUT\n", n.ups); err != nil {
		return nil, cleanErr(err)
	}
	return parseNUT(bufio.NewScanner(conn), n.ups)
}

func parseNUT(sc *bufio.Scanner, ups string) (map[string]string, error) {
	vars := map[string]string{}
	prefix := "VAR " + ups + " "
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "ERR "):
			return nil, fmt.Errorf("NUT refuse : %s", strings.TrimPrefix(line, "ERR ")) // UNKNOWN-UPS, ACCESS-DENIED…
		case strings.HasPrefix(line, prefix):
			k, v, ok := strings.Cut(strings.TrimPrefix(line, prefix), " ")
			if ok {
				vars[k] = strings.TrimSpace(strings.Trim(v, `"`))
			}
		case strings.HasPrefix(line, "END LIST VAR"):
			return vars, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, cleanErr(err)
	}
	return nil, errors.New("réponse NUT incomplète")
}

// nutFlags : drapeaux de ups.status, du plus grave au plus bénin.
var nutFlags = []struct{ flag, text, level string }{
	{"LB", "batterie faible", "crit"},
	{"OVER", "surcharge", "crit"},
	{"RB", "batterie à remplacer", "crit"},
	{"FSD", "arrêt forcé", "crit"},
	{"OFF", "sortie coupée", "crit"},
	{"OB", "sur batterie", "warn"},
	{"BYPASS", "en dérivation", "warn"},
	{"CAL", "calibrage", "warn"},
	{"BOOST", "tension relevée", ""},
	{"TRIM", "tension abaissée", ""},
	{"CHRG", "en charge", ""},
	{"OL", "sur secteur", "ok"},
}

func nutValues(v map[string]string) tile.Values {
	out := tile.Values{}
	num := func(k string) (float64, bool) {
		f, err := strconv.ParseFloat(v[k], 64)
		return f, err == nil
	}
	if s, ok := v["ups.status"]; ok {
		flags := strings.Fields(s)
		var texts []string
		level := ""
		for _, f := range nutFlags {
			for _, g := range flags {
				if g == f.flag {
					texts = append(texts, f.text)
					if level == "" || level == "ok" {
						level = f.level
					}
				}
			}
		}
		if len(texts) == 0 {
			texts = []string{s}
		}
		out["status"] = tile.Value{Text: texts[0], Sub: strings.Join(texts[1:], " · "), Level: level}
	}
	if c, ok := num("battery.charge"); ok {
		low, okLow := num("battery.charge.low")
		warn, okWarn := num("battery.charge.warning")
		lvl := ""
		switch {
		case okLow && c <= low:
			lvl = "crit"
		case okWarn && c <= warn:
			lvl = "warn"
		}
		out["charge"] = tile.Value{Pct: c, Text: tile.Pct(c), Level: lvl}
	}
	if r, ok := num("battery.runtime"); ok {
		lvl := ""
		if low, ok := num("battery.runtime.low"); ok && r <= low {
			lvl = "crit"
		}
		out["runtime"] = tile.Value{Text: tile.Dur(time.Duration(r) * time.Second), Level: lvl}
	}
	if l, ok := num("ups.load"); ok {
		val := tile.Value{Pct: l, Text: tile.Pct(l), Level: tile.Level(l, 80, 95)}
		if p, ok := num("ups.realpower.nominal"); ok && p > 0 {
			val.Sub = "≈ " + tile.Num(l*p/100) + " W sur " + tile.Num(p)
		}
		out["load"] = val
	}
	for id, k := range map[string]string{"input": "input.voltage", "output": "output.voltage", "battv": "battery.voltage"} {
		if f, ok := num(k); ok {
			out[id] = tile.Value{Text: tile.Num(f) + " V"}
		}
	}
	if t := v["ups.test.result"]; t != "" {
		out["test"] = tile.Value{Text: t}
	}
	if m := strings.TrimSpace(v["device.model"]); m != "" {
		out["model"] = tile.Value{Text: strings.TrimSpace(v["device.mfr"] + " " + m)}
	}
	return out
}
