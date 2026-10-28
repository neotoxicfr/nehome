package integ

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Deluge : Web JSON-RPC (/json). auth.login pose le cookie _session_id (jar),
// puis web.update_ui ramène torrents et statistiques en un seul appel.
type deluge struct {
	url, pass string
	hc        *http.Client
	loggedIn  bool
	id        int

	down, up *tile.Ring
}

type dgTorrent struct {
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"` // 0 à 100
	DlRate   int64   `json:"download_payload_rate"`
	UpRate   int64   `json:"upload_payload_rate"`
	ETA      int64   `json:"eta"`
	Size     int64   `json:"total_size"`
	Ratio    float64 `json:"ratio"`
}

type dgUpdate struct {
	Connected bool                 `json:"connected"`
	Torrents  map[string]dgTorrent `json:"torrents"`
	Stats     struct {
		MaxDown   float64 `json:"max_download"` // Kio/s, -1 = illimité
		MaxUp     float64 `json:"max_upload"`
		MaxConn   int     `json:"max_num_connections"`
		Conn      int     `json:"num_connections"`
		DownRate  float64 `json:"download_rate"`
		UpRate    float64 `json:"upload_rate"`
		DHT       int     `json:"dht_nodes"`
		Incoming  bool    `json:"has_incoming_connections"`
		FreeSpace float64 `json:"free_space"`
	} `json:"stats"`
}

var errDgAuth = errors.New("session Deluge expirée")

func newDeluge(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	pass, err := secret("pass")
	if err != nil {
		return nil, err
	}
	return &deluge{url: strings.TrimRight(t.URL, "/") + "/json", pass: pass, hc: httpClient(t.InsecureTLS, true), down: tile.NewRing(60), up: tile.NewRing(60)}, nil
}

func (d *deluge) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("up", "Envoi", tile.Spark, tile.OnTile),
		tile.F("leech", "En téléchargement", tile.Stat, tile.OnTile),
		tile.F("seed", "En partage", tile.Stat, tile.OnTile),
		tile.F("incoming", "Connexions entrantes", tile.Stat, tile.InDetail),
		tile.F("conn", "Connexions", tile.Stat, tile.InDetail),
		tile.F("dht", "Nœuds DHT", tile.Stat, tile.InDetail),
		tile.F("limits", "Limites", tile.Stat, tile.InDetail),
		tile.F("free", "Espace libre", tile.Stat, tile.InDetail),
		tile.F("errors", "En erreur", tile.Stat, tile.InDetail),
		tile.F("paused", "En pause", tile.Stat, tile.InDetail),
		tile.F("dl", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("topup", "Envois les plus actifs", tile.Table, tile.InDetail),
	}
}

func (d *deluge) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 3*time.Second, func() {
		u, err := d.update(ctx)
		if errors.Is(err, errDgAuth) { // session perdue : un seul nouvel essai
			d.loggedIn = false
			u, err = d.update(ctx)
		}
		if err != nil {
			if ctx.Err() == nil {
				emit(nil, err)
			}
			return
		}
		emit(d.values(u), nil)
	})
}

func (d *deluge) rpc(ctx context.Context, method string, params []any, out any) error {
	d.id++
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if _, err := dlPost(ctx, d.hc, d.url, map[string]any{"method": method, "params": params, "id": d.id}, &env, nil); err != nil {
		return err
	}
	if env.Error != nil {
		if env.Error.Code == 1 || strings.Contains(env.Error.Message, "Not authenticated") {
			return errDgAuth
		}
		return fmt.Errorf("Deluge : %s", env.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

func (d *deluge) update(ctx context.Context) (*dgUpdate, error) {
	if !d.loggedIn {
		var ok bool
		if err := d.rpc(ctx, "auth.login", []any{d.pass}, &ok); err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("connexion Deluge refusée (mot de passe)")
		}
		d.loggedIn = true
	}
	keys := []string{"name", "state", "progress", "download_payload_rate", "upload_payload_rate", "eta", "total_size", "ratio"}
	var u dgUpdate
	if err := d.rpc(ctx, "web.update_ui", []any{keys, map[string]any{}}, &u); err != nil {
		return nil, err
	}
	if !u.Connected {
		return nil, errors.New("l'interface web Deluge n'est reliée à aucun démon")
	}
	return &u, nil
}

func (d *deluge) values(u *dgUpdate) tile.Values {
	var leech, seed, paused, errs int
	var dl, up []dgTorrent
	for _, t := range u.Torrents {
		switch t.State {
		case "Downloading":
			leech++
			dl = append(dl, t)
		case "Seeding":
			seed++
		case "Paused":
			paused++
		case "Error":
			errs++
		}
		if t.UpRate > 0 {
			up = append(up, t)
		}
	}
	slices.SortFunc(dl, func(a, b dgTorrent) int { return cmp.Compare(b.Progress, a.Progress) })
	var dlRows [][]tile.Cell
	for _, t := range dl[:min(15, len(dl))] {
		eta := "∞"
		if t.ETA > 0 && t.ETA < 8640000 {
			eta = tile.Dur(time.Duration(t.ETA) * time.Second)
		}
		dlRows = append(dlRows, []tile.Cell{{Text: t.Name}, {Text: tile.Pct(t.Progress)}, {Text: tile.Rate(float64(t.DlRate))}, {Text: eta}})
	}
	slices.SortFunc(up, func(a, b dgTorrent) int { return cmp.Compare(b.UpRate, a.UpRate) })
	var upRows [][]tile.Cell
	for _, t := range up[:min(15, len(up))] {
		upRows = append(upRows, []tile.Cell{{Text: t.Name}, {Text: tile.Num(t.Ratio)}, {Text: tile.Rate(float64(t.UpRate))}})
	}
	s := u.Stats
	limit := func(kib float64) string {
		if kib < 0 {
			return "∞"
		}
		return tile.Rate(kib * 1024)
	}
	in, inLvl := "oui", "ok"
	if !s.Incoming {
		in, inLvl = "non", "warn"
	}
	errLvl := ""
	if errs > 0 {
		errLvl = "warn"
	}
	return tile.Values{
		"down":     {Text: tile.Rate(s.DownRate), Points: d.down.Push(s.DownRate)},
		"up":       {Text: tile.Rate(s.UpRate), Points: d.up.Push(s.UpRate)},
		"leech":    {Text: fmt.Sprint(leech), Sub: fmt.Sprintf("%d en pause", paused)},
		"seed":     {Text: fmt.Sprint(seed), Sub: fmt.Sprintf("%d au total", len(u.Torrents))},
		"incoming": {Text: in, Level: inLvl},
		"conn":     {Text: fmt.Sprintf("%d / %d", s.Conn, s.MaxConn)},
		"dht":      {Text: fmt.Sprint(s.DHT)},
		"limits":   {Text: "↓ " + limit(s.MaxDown) + " · ↑ " + limit(s.MaxUp)},
		"free":     {Text: tile.Bytes(s.FreeSpace)},
		"errors":   {Text: fmt.Sprint(errs), Level: errLvl},
		"paused":   {Text: fmt.Sprint(paused)},
		"dl":       {Head: []string{"Nom", "Avancement", "Débit", "Reste"}, Rows: dlRows, Text: "aucun"},
		"topup":    {Head: []string{"Nom", "Ratio", "Débit"}, Rows: upRows, Text: "aucun envoi"},
	}
}
