package integ

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Tautulli : la clé API ne passe que par la query string (redact/cleanErr
// l'enlèvent des erreurs). Flux toutes les 5 s, historique 60 s, statistiques
// lourdes (accueil sur 7 jours, lectures par jour) 5 min.
type tautulli struct {
	url, key string
	hc       *http.Client
}

func newTautulli(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	a := &tautulli{url: strings.TrimRight(t.URL, "/") + "/api/v2", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if a.key, err = secret("key"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *tautulli) Fields() []tile.Field {
	return []tile.Field{
		tile.F("streams", "Flux en cours", tile.Stat, tile.OnTile),
		tile.F("bandwidth", "Bande passante", tile.Stat, tile.OnTile),
		tile.F("plays", "Lectures (14 j)", tile.Spark, tile.OnTile),
		tile.F("peak", "Pic de flux simultanés (7 j)", tile.Stat, tile.InDetail),
		tile.F("popular_movies", "Films populaires (7 j)", tile.Table, tile.InDetail),
		tile.F("popular_tv", "Séries populaires (7 j)", tile.Table, tile.InDetail),
		tile.F("top_users", "Utilisateurs les plus actifs (7 j)", tile.Table, tile.InDetail),
		tile.F("history", "Historique récent", tile.Table, tile.InDetail),
	}
}

type tautulliResp[T any] struct {
	Response struct {
		Result, Message string
		Data            T
	}
}

// tautulliGet appelle une commande ; un « result » autre que success devient une erreur.
func tautulliGet[T any](ctx context.Context, a *tautulli, cmd string, args url.Values, out *T) error {
	q := url.Values{"apikey": {a.key}, "cmd": {cmd}}
	for k, v := range args {
		q[k] = v
	}
	var r tautulliResp[T]
	if err := getJSON(ctx, a.hc, a.url+"?"+q.Encode(), &r, nil); err != nil {
		return err
	}
	if r.Response.Result != "success" {
		return fmt.Errorf("%s : %s", cmd, r.Response.Message)
	}
	*out = r.Response.Data
	return nil
}

func (a *tautulli) Run(ctx context.Context, emit func(tile.Values, error)) {
	run := func(d time.Duration, f func(context.Context) (tile.Values, error)) {
		every(ctx, d, func() {
			v, err := f(ctx)
			if ctx.Err() == nil {
				emit(v, err)
			}
		})
	}
	go run(time.Minute, a.history)
	go run(5*time.Minute, a.stats)
	run(5*time.Second, a.activity)
}

func (a *tautulli) activity(ctx context.Context) (tile.Values, error) {
	var d struct {
		StreamCount    string  `json:"stream_count"`
		DirectPlay     int     `json:"stream_count_direct_play"`
		DirectStream   int     `json:"stream_count_direct_stream"`
		Transcode      int     `json:"stream_count_transcode"`
		TotalBandwidth float64 `json:"total_bandwidth"` // kbit/s
		LanBandwidth   float64 `json:"lan_bandwidth"`
		WanBandwidth   float64 `json:"wan_bandwidth"`
	}
	if err := tautulliGet(ctx, a, "get_activity", nil, &d); err != nil {
		return nil, err
	}
	return tile.Values{
		"streams": {Text: d.StreamCount, Sub: fmt.Sprintf("%d directe(s), %d flux direct, %d transcodage(s)", d.DirectPlay, d.DirectStream, d.Transcode)},
		"bandwidth": {Text: tile.Rate(d.TotalBandwidth * 125),
			Sub: fmt.Sprintf("WAN %s · LAN %s", tile.Rate(d.WanBandwidth*125), tile.Rate(d.LanBandwidth*125))},
	}, nil
}

type tautulliStat struct {
	StatID string `json:"stat_id"`
	Rows   []struct {
		Title         string
		FriendlyName  string  `json:"friendly_name"`
		TotalPlays    any     `json:"total_plays"`
		UsersWatched  any     `json:"users_watched"`
		TotalDuration float64 `json:"total_duration"`
		Count         int
	}
}

func (a *tautulli) stats(ctx context.Context) (tile.Values, error) {
	var st []tautulliStat
	if err := tautulliGet(ctx, a, "get_home_stats", url.Values{"time_range": {"7"}, "stats_count": {"10"}}, &st); err != nil {
		return nil, err
	}
	v := tile.Values{}
	for _, s := range st {
		var rows [][]tile.Cell
		switch s.StatID {
		case "popular_movies", "popular_tv":
			for _, r := range s.Rows {
				rows = append(rows, []tile.Cell{{Text: r.Title}, {Text: tautulliN(r.TotalPlays)}, {Text: tautulliN(r.UsersWatched)}})
			}
			v[s.StatID] = tile.Value{Head: []string{"Titre", "Lectures", "Spectateurs"}, Rows: rows}
		case "top_users":
			for _, r := range s.Rows {
				rows = append(rows, []tile.Cell{{Text: r.FriendlyName}, {Text: tautulliN(r.TotalPlays)}, {Text: tile.Dur(time.Duration(r.TotalDuration) * time.Second)}})
			}
			v[s.StatID] = tile.Value{Head: []string{"Utilisateur", "Lectures", "Durée"}, Rows: rows}
		case "most_concurrent":
			for _, r := range s.Rows {
				if r.Title == "Concurrent Streams" {
					v["peak"] = tile.Value{Text: fmt.Sprint(r.Count)}
				}
				if r.Title == "Concurrent Transcodes" {
					p := v["peak"]
					p.Sub = fmt.Sprintf("dont %d transcodage(s)", r.Count)
					v["peak"] = p
				}
			}
		}
	}
	var pl struct {
		Series []struct {
			Name string
			Data []float64
		}
	}
	if err := tautulliGet(ctx, a, "get_plays_by_date", url.Values{"time_range": {"14"}}, &pl); err == nil {
		for _, s := range pl.Series {
			if s.Name == "Total" && len(s.Data) > 0 {
				v["plays"] = tile.Value{Text: tile.Num(s.Data[len(s.Data)-1]) + " aujourd'hui", Points: s.Data}
			}
		}
	}
	return v, nil
}

func (a *tautulli) history(ctx context.Context) (tile.Values, error) {
	var h struct {
		Data []struct {
			Started           int64
			FriendlyName      string `json:"friendly_name"`
			FullTitle         string `json:"full_title"`
			PercentComplete   int    `json:"percent_complete"`
			TranscodeDecision string `json:"transcode_decision"`
			Platform          string
		}
	}
	if err := tautulliGet(ctx, a, "get_history", url.Values{"length": {"10"}}, &h); err != nil {
		return nil, err
	}
	var rows [][]tile.Cell
	for _, r := range h.Data {
		mode := map[string]string{"direct play": "Lecture directe", "copy": "Flux direct", "transcode": "Transcodage"}[r.TranscodeDecision]
		rows = append(rows, []tile.Cell{{Text: r.FriendlyName}, {Text: r.FullTitle}, {Text: strconv.Itoa(r.PercentComplete) + " %"}, {Text: mode}, {Text: tile.Ago(time.Unix(r.Started, 0))}})
	}
	return tile.Values{"history": {Head: []string{"Utilisateur", "Titre", "Vu", "Mode", "Début"}, Rows: rows}}, nil
}

func tautulliN(v any) string {
	if f, ok := v.(float64); ok {
		return tile.Num(f)
	}
	return ""
}
