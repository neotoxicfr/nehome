package integ

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// qBittorrent : sync/maindata incrémental (rid) — le premier appel ramène les
// ~4 700 torrents, les suivants seulement ce qui a changé, d'où un sondage à 2 s.
type qbit struct {
	url, user, pass string
	hc              *http.Client

	loginMu  sync.Mutex // login appelé par le sondage et par Do (autre goroutine)
	banUntil time.Time  // pas de nouveau login avant : 5 échecs et qBittorrent bannit l'IP (sous loginMu)
	rid      int64
	torrents map[string]*qbTorrent
	server   qbServer
	down, up *tile.Ring
}

type qbTorrent struct {
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Category string  `json:"category"`
	Tags     string  `json:"tags"`
	Progress float64 `json:"progress"`
	DlSpeed  int64   `json:"dlspeed"`
	UpSpeed  int64   `json:"upspeed"`
	Size     int64   `json:"size"`
	ETA      int64   `json:"eta"`
}

type qbServer struct {
	DlSpeed float64 `json:"dl_info_speed"`
	UpSpeed float64 `json:"up_info_speed"`
	Free    float64 `json:"free_space_on_disk"`
	Ratio   string  `json:"global_ratio"`
	AllDL   float64 `json:"alltime_dl"`
	AllUL   float64 `json:"alltime_ul"`
	Conn    string  `json:"connection_status"`
	Alt     bool    `json:"use_alt_speed_limits"`
	IPv4    string  `json:"last_external_address_v4"`
	IPv6    string  `json:"last_external_address_v6"`
	Peers   int     `json:"total_peer_connections"`
	DlLimit float64 `json:"dl_rate_limit"`
	UpLimit float64 `json:"up_rate_limit"`
}

type qbMain struct {
	Rid         int64                      `json:"rid"`
	FullUpdate  bool                       `json:"full_update"`
	Torrents    map[string]json.RawMessage `json:"torrents"`
	Removed     []string                   `json:"torrents_removed"`
	ServerState json.RawMessage            `json:"server_state"`
}

func newQbit(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	q := &qbit{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, true), down: tile.NewRing(60), up: tile.NewRing(60)}
	// Identifiants facultatifs : qBittorrent peut exempter un sous-réseau.
	if _, ok := t.Secrets["user"]; ok {
		var err error
		if q.user, err = secret("user"); err != nil {
			return nil, err
		}
		if q.pass, err = secret("pass"); err != nil {
			return nil, err
		}
	}
	return q, nil
}

func (q *qbit) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("up", "Envoi", tile.Spark, tile.OnTile),
		tile.F("leech", "En téléchargement", tile.Stat, tile.OnTile),
		tile.F("seed", "En partage", tile.Stat, tile.OnTile),
		tile.F("ratio", "Ratio global", tile.Stat, tile.InDetail),
		tile.F("conn", "Connexion", tile.Stat, tile.InDetail),
		tile.F("alt", "Limite alternative", tile.Stat, tile.InDetail),
		tile.F("limits", "Limites", tile.Stat, tile.InDetail),
		tile.F("free", "Espace libre", tile.Stat, tile.InDetail),
		tile.F("total", "Total depuis l'origine", tile.Stat, tile.InDetail),
		tile.F("peers", "Pairs connectés", tile.Stat, tile.InDetail),
		tile.F("errors", "En erreur", tile.Stat, tile.InDetail),
		tile.F("stalled", "Téléchargements bloqués", tile.Stat, tile.InDetail),
		tile.F("dl", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("topup", "Envois les plus actifs", tile.Table, tile.InDetail),
		tile.F("cats", "Par catégorie", tile.Table, tile.InDetail),
		tile.F("tags", "Par étiquette", tile.Table, tile.InDetail),
	}
}

// Run : toutes les 2 s (navigateur ouvert). Quand qBittorrent ne répond plus
// (vérification de fichiers, redémarrage), sa WebUI est bloquée : on espace
// les requêtes (4 s, 8 s… jusqu'à 2 min) au lieu d'insister toutes les 2 s.
func (q *qbit) Run(ctx context.Context, emit func(tile.Values, error)) {
	var backoff time.Duration
	every(ctx, 2*time.Second, func() {
		if err := q.sync(ctx); err != nil {
			if ctx.Err() == nil {
				emit(nil, err)
			}
			backoff = min(max(2*backoff, 4*time.Second), qbMaxBackoff)
			t := time.NewTimer(backoff)
			defer t.Stop()
			select {
			case <-ctx.Done():
			case <-t.C:
			}
			return
		}
		backoff = 0
		emit(q.values(), nil)
	})
}

const qbMaxBackoff = 2 * time.Minute

func (q *qbit) login(ctx context.Context) error {
	q.loginMu.Lock()
	defer q.loginMu.Unlock()
	if q.user == "" {
		return errors.New("qBittorrent refuse l'accès (403) et aucun identifiant n'est configuré")
	}
	if time.Now().Before(q.banUntil) {
		return errors.New("identifiants refusés par qBittorrent")
	}
	form := url.Values{"username": {q.user}, "password": {q.pass}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, q.url+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := q.hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connexion qBittorrent refusée (HTTP %d)", resp.StatusCode)
	}
	// un mauvais mot de passe répond 200 avec « Fails. »
	if strings.TrimSpace(string(body)) != "Ok." {
		q.banUntil = time.Now().Add(5 * time.Minute)
		return errors.New("identifiants refusés par qBittorrent")
	}
	return nil
}

func (q *qbit) sync(ctx context.Context) error {
	var m qbMain
	u := fmt.Sprintf("%s/api/v2/sync/maindata?rid=%d", q.url, q.rid)
	err := getJSON(ctx, q.hc, u, &m, nil)
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusForbidden {
		if err := q.login(ctx); err != nil {
			return err
		}
		q.rid = 0
		u = fmt.Sprintf("%s/api/v2/sync/maindata?rid=0", q.url)
		err = getJSON(ctx, q.hc, u, &m, nil)
	}
	if err != nil {
		return err
	}
	if m.FullUpdate || q.torrents == nil {
		q.torrents = map[string]*qbTorrent{}
		q.server = qbServer{}
	}
	for h, raw := range m.Torrents {
		t := q.torrents[h]
		if t == nil {
			t = &qbTorrent{}
			q.torrents[h] = t
		}
		if err := json.Unmarshal(raw, t); err != nil {
			return err
		}
	}
	for _, h := range m.Removed {
		delete(q.torrents, h)
	}
	if len(m.ServerState) > 0 {
		if err := json.Unmarshal(m.ServerState, &q.server); err != nil {
			return err
		}
	}
	q.rid = m.Rid
	return nil
}

var (
	qbConn  = map[string]string{"connected": "connecté", "firewalled": "derrière un pare-feu", "disconnected": "déconnecté"}
	qbLeech = map[string]bool{"downloading": true, "metaDL": true, "forcedMetaDL": true, "forcedDL": true, "stalledDL": true, "queuedDL": true, "checkingDL": true, "allocating": true}
	qbSeed  = map[string]bool{"uploading": true, "stalledUP": true, "forcedUP": true, "queuedUP": true, "checkingUP": true}
)

func (q *qbit) values() tile.Values {
	s := q.server
	var leech, seed, paused, errs, stalled int
	type agg struct {
		n    int
		size float64
	}
	cats, tags := map[string]*agg{}, map[string]*agg{}
	add := func(m map[string]*agg, k string, size int64) {
		if m[k] == nil {
			m[k] = &agg{}
		}
		m[k].n++
		m[k].size += float64(size)
	}
	var dl, up []*qbTorrent
	for _, t := range q.torrents {
		switch {
		case qbLeech[t.State]:
			leech++
			dl = append(dl, t)
		case qbSeed[t.State]:
			seed++
		case t.State == "error" || t.State == "missingFiles":
			errs++
		default:
			paused++
		}
		if t.State == "stalledDL" {
			stalled++
		}
		if t.UpSpeed > 0 {
			up = append(up, t)
		}
		cat := t.Category
		if cat == "" {
			cat = "(aucune)"
		}
		add(cats, cat, t.Size)
		for tag := range strings.SplitSeq(t.Tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				add(tags, tag, t.Size)
			}
		}
	}

	table := func(m map[string]*agg) [][]tile.Cell {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int { return cmp.Compare(m[b].n, m[a].n) })
		rows := make([][]tile.Cell, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []tile.Cell{{Text: k}, {Text: fmt.Sprint(m[k].n)}, {Text: tile.Bytes(m[k].size)}})
		}
		return rows
	}

	slices.SortFunc(dl, func(a, b *qbTorrent) int { return cmp.Compare(b.Progress, a.Progress) })
	var dlRows [][]tile.Cell
	for _, t := range dl[:min(15, len(dl))] {
		eta := "∞"
		if t.ETA > 0 && t.ETA < 8640000 {
			eta = tile.Dur(time.Duration(t.ETA) * time.Second)
		}
		lvl := ""
		if t.State == "stalledDL" {
			lvl = "warn"
		}
		dlRows = append(dlRows, []tile.Cell{{Text: t.Name}, {Text: tile.Pct(t.Progress * 100)}, {Text: tile.Rate(float64(t.DlSpeed)), Level: lvl}, {Text: eta}})
	}
	slices.SortFunc(up, func(a, b *qbTorrent) int { return cmp.Compare(b.UpSpeed, a.UpSpeed) })
	var upRows [][]tile.Cell
	for _, t := range up[:min(15, len(up))] {
		upRows = append(upRows, []tile.Cell{{Text: t.Name}, {Text: t.Category}, {Text: tile.Rate(float64(t.UpSpeed))}})
	}

	connLvl := "ok"
	if s.Conn != "connected" {
		connLvl = "crit"
	}
	alt, altLvl := "non", ""
	if s.Alt {
		alt, altLvl = "oui", "warn"
	}
	errLvl := ""
	if errs > 0 {
		errLvl = "warn"
	}
	limit := func(f float64) string {
		if f <= 0 {
			return "∞"
		}
		return tile.Rate(f)
	}
	return tile.Values{
		"down":    {Text: tile.Rate(s.DlSpeed), Points: q.down.Push(s.DlSpeed)},
		"up":      {Text: tile.Rate(s.UpSpeed), Points: q.up.Push(s.UpSpeed)},
		"leech":   {Text: fmt.Sprint(leech), Sub: fmt.Sprintf("%d en pause", paused)},
		"seed":    {Text: fmt.Sprint(seed), Sub: fmt.Sprintf("%d au total", len(q.torrents))},
		"ratio":   {Text: strings.Replace(s.Ratio, ".", ",", 1)},
		"conn":    {Text: qbConn[s.Conn], Sub: strings.TrimSpace(s.IPv4 + " " + s.IPv6), Level: connLvl},
		"alt":     {Text: alt, Level: altLvl},
		"limits":  {Text: "↓ " + limit(s.DlLimit) + " · ↑ " + limit(s.UpLimit)},
		"free":    {Text: tile.Bytes(s.Free)},
		"total":   {Text: "↓ " + tile.Bytes(s.AllDL) + " · ↑ " + tile.Bytes(s.AllUL)},
		"peers":   {Text: fmt.Sprint(s.Peers)},
		"errors":  {Text: fmt.Sprint(errs), Level: errLvl},
		"stalled": {Text: fmt.Sprint(stalled)},
		"dl":      {Head: []string{"Nom", "Avancement", "Débit", "Reste"}, Rows: dlRows, Text: "aucun"},
		"topup":   {Head: []string{"Nom", "Catégorie", "Débit"}, Rows: upRows, Text: "aucun envoi"},
		"cats":    {Head: []string{"Catégorie", "Torrents", "Taille"}, Rows: table(cats)},
		"tags":    {Head: []string{"Étiquette", "Torrents", "Taille"}, Rows: table(tags)},
	}
}

// Pas d'arrêt ni de pause de torrents : risque de hit-and-run sur les trackers privés.
func (q *qbit) Actions() []tile.Action {
	return []tile.Action{{ID: "altspeed", Label: "Basculer la limite alternative"}}
}

// Do : POST /api/v2/transfer/toggleSpeedLimitsMode, avec un login si la
// session a expiré (403), comme le sondage.
func (q *qbit) Do(ctx context.Context, id, _ string) (string, error) {
	if id != "altspeed" {
		return "", errUnknownAction
	}
	u := q.url + "/api/v2/transfer/toggleSpeedLimitsMode"
	err := callAction(ctx, q.hc, http.MethodPost, u, nil, nil)
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusForbidden {
		if err := q.login(ctx); err != nil {
			return "", err
		}
		err = callAction(ctx, q.hc, http.MethodPost, u, nil, nil)
	}
	if err != nil {
		return "", err
	}
	return "Limite alternative basculée", nil
}
