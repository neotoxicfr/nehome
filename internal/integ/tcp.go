package integ

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// tcp : teste l'ouverture de ports TCP (connexion puis fermeture immédiate,
// rien n'est envoyé). Toutes les 30 s, délai de 3 s par cible.
type tcpCheck struct {
	targets []tcpTarget
	timeout time.Duration
}

type tcpTarget struct{ name, addr string }

type tcpResult struct {
	tcpTarget
	lat time.Duration
	err error
}

func newTCP(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	c := &tcpCheck{timeout: 3 * time.Second}
	for _, item := range strings.Split(t.Options["targets"], ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, addr, ok := strings.Cut(item, "=")
		if !ok {
			addr = name
		}
		name, addr = strings.TrimSpace(name), strings.TrimSpace(addr)
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" {
			return nil, fmt.Errorf("options.targets : %q n'est pas de la forme hôte:port", addr)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("options.targets : port %q invalide", port)
		}
		c.targets = append(c.targets, tcpTarget{name: name, addr: addr})
	}
	if len(c.targets) == 0 {
		return nil, errors.New("options.targets manquant (nom=hôte:port, séparés par des virgules)")
	}
	return c, nil
}

func (c *tcpCheck) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "En panne", tile.Stat, tile.OnTile),
		tile.F("up", "Joignables", tile.Stat, tile.OnTile),
		tile.F("latency", "Latence moyenne", tile.Stat, tile.OnTile),
		tile.F("targets", "Ports", tile.Table, tile.InDetail),
	}
}

func (c *tcpCheck) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v := c.check(ctx)
		if ctx.Err() == nil {
			emit(v, nil)
		}
	})
}

func (c *tcpCheck) check(ctx context.Context) tile.Values {
	res := make([]tcpResult, len(c.targets))
	var wg sync.WaitGroup
	for i, t := range c.targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = c.dial(ctx, t)
		}()
	}
	wg.Wait()

	var down []string
	var sum, worst time.Duration
	var worstName string
	for _, r := range res {
		if r.err != nil {
			down = append(down, r.name)
			continue
		}
		sum += r.lat
		if r.lat >= worst {
			worst, worstName = r.lat, r.name
		}
	}
	up := len(res) - len(down)
	v := tile.Values{
		"up": {Text: fmt.Sprintf("%d / %d", up, len(res)), Level: levelIf(up == len(res), "ok", "crit")},
	}
	dv := tile.Value{Text: strconv.Itoa(len(down)), Level: "ok"}
	if len(down) > 0 {
		dv.Level, dv.Sub = "crit", genClip(strings.Join(down, ", "), 80)
	}
	v["down"] = dv
	if up > 0 {
		v["latency"] = tile.Value{Text: tcpMS(sum / time.Duration(up)), Sub: "max " + tcpMS(worst) + " (" + worstName + ")"}
	}

	// Cibles en panne d'abord, puis l'ordre de la configuration.
	sort.SliceStable(res, func(i, j int) bool { return res[i].err != nil && res[j].err == nil })
	rows := [][]tile.Cell{}
	for _, r := range res {
		if len(rows) == 15 {
			break
		}
		state, lat := tile.Cell{Text: "ouvert", Level: "ok"}, tile.Cell{Text: tcpMS(r.lat)}
		if r.err != nil {
			state, lat = tile.Cell{Text: tcpReason(r.err), Level: "crit"}, tile.Cell{Text: "—"}
		}
		rows = append(rows, []tile.Cell{{Text: r.name}, {Text: r.addr}, state, lat})
	}
	v["targets"] = tile.Value{Head: []string{"Nom", "Adresse", "État", "Latence"}, Rows: rows}
	return v
}

func (c *tcpCheck) dial(ctx context.Context, t tcpTarget) tcpResult {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", t.addr)
	r := tcpResult{tcpTarget: t, lat: time.Since(start), err: err}
	if err == nil {
		conn.Close()
	}
	return r
}

func tcpMS(d time.Duration) string { return tile.Num(float64(d.Microseconds())/1000) + " ms" }

// tcpReason résume l'erreur de connexion en quelques mots.
func tcpReason(err error) string {
	var dns *net.DNSError
	var ne net.Error
	switch {
	case errors.As(err, &dns):
		if dns.IsNotFound {
			return "nom inconnu"
		}
		return "DNS en échec"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refusé"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "injoignable"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "délai dépassé"
	}
	return "en échec"
}
