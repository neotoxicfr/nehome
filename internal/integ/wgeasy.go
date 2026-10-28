package integ

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// wg-easy, versions 14 et 15. Le choix suit les secrets déclarés :
//   - rôle pass seul : v14, /api/wireguard/client, mot de passe en en-tête
//     Authorization (c'est la forme prévue pour les clients d'API) ;
//   - rôles user + pass : v15, /api/client, authentification Basic.
//
// Un client est « connecté » si son dernier handshake a moins de 3 min. Les
// compteurs de trafic sont cumulés depuis le démarrage de l'interface.
// Relevé toutes les 15 s.
type wgeasy struct {
	url, user, pass string
	v15             bool
	hc              *http.Client
}

func newWGEasy(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	w := &wgeasy{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if w.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	// la version suit la DÉCLARATION du rôle user : un secret illisible est une erreur, pas un repli en v14
	if _, w.v15 = t.Secrets["user"]; w.v15 {
		if w.user, err = secret("user"); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (w *wgeasy) Fields() []tile.Field {
	return []tile.Field{
		tile.F("online", "Clients connectés", tile.Stat, tile.OnTile),
		tile.F("rx", "Reçu (cumul)", tile.Stat, tile.OnTile),
		tile.F("tx", "Envoyé (cumul)", tile.Stat, tile.OnTile),
		tile.F("clients", "Clients", tile.Stat, tile.InDetail),
		tile.F("client_list", "Clients WireGuard", tile.Table, tile.InDetail),
	}
}

func (w *wgeasy) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 15*time.Second, func() {
		v, err := w.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (w *wgeasy) poll(ctx context.Context) (tile.Values, error) {
	var cs []struct {
		Name      string
		Enabled   bool
		Address   string     // v14
		IPv4      string     `json:"ipv4Address"`
		Handshake *time.Time `json:"latestHandshakeAt"`
		Rx        float64    `json:"transferRx"`
		Tx        float64    `json:"transferTx"`
	}
	path := "/wireguard/client"
	if w.v15 {
		path = "/client"
	}
	err := getJSON(ctx, w.hc, w.url+path, &cs, func(r *http.Request) {
		if w.v15 {
			r.SetBasicAuth(w.user, w.pass)
		} else {
			r.Header.Set("Authorization", w.pass)
		}
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
	on := 0
	var rx, tx float64
	var rows [][]tile.Cell
	for _, c := range cs {
		rx += c.Rx
		tx += c.Tx
		state, lvl, hs := "Désactivé", "", "jamais"
		if c.Handshake != nil && !c.Handshake.IsZero() {
			hs = tile.Ago(*c.Handshake)
		}
		switch {
		case !c.Enabled:
		case c.Handshake != nil && time.Since(*c.Handshake) < 3*time.Minute:
			on++
			state, lvl = "Connecté", "ok"
		default:
			state = "Inactif"
		}
		addr := c.Address
		if addr == "" {
			addr = c.IPv4
		}
		if len(rows) < 15 {
			rows = append(rows, []tile.Cell{{Text: c.Name}, {Text: addr}, {Text: state, Level: lvl}, {Text: hs}, {Text: tile.Bytes(c.Rx)}, {Text: tile.Bytes(c.Tx)}})
		}
	}
	return tile.Values{
		"online":      {Text: fmt.Sprintf("%d / %d", on, len(cs))},
		"rx":          {Text: tile.Bytes(rx)},
		"tx":          {Text: tile.Bytes(tx)},
		"clients":     {Text: fmt.Sprint(len(cs))},
		"client_list": {Head: []string{"Client", "Adresse", "État", "Dernier handshake", "Reçu", "Envoyé"}, Rows: rows},
	}, nil
}
