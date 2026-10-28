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

// Tracearr (API publique v1, jeton Bearer). En plus de Tautulli : score de
// confiance des comptes, violations, détail technique des flux (codecs, HDR),
// part de transcodage sur 30 jours. Flux 5 s, compteurs 60 s, 30 jours 5 min.
type tracearr struct {
	url, key string
	hc       *http.Client
}

func newTracearr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	a := &tracearr{url: strings.TrimRight(t.URL, "/") + "/api/v1/public", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if a.key, err = secret("key"); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *tracearr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("streams", "Flux en cours", tile.Stat, tile.OnTile),
		tile.F("today", "Lectures aujourd'hui", tile.Stat, tile.OnTile),
		tile.F("violations", "Violations récentes", tile.Stat, tile.OnTile),
		tile.F("plays", "Lectures (30 j)", tile.Spark, tile.OnTile),
		tile.F("details", "Flux détaillés", tile.Table, tile.InDetail),
		tile.F("quality", "Qualité de lecture (30 j)", tile.Bars, tile.InDetail),
		tile.F("platforms", "Plateformes (30 j)", tile.Table, tile.InDetail),
		tile.F("trust", "Comptes à surveiller", tile.Table, tile.InDetail),
		tile.F("users", "Comptes", tile.Stat, tile.InDetail),
		tile.F("server", "Serveurs", tile.Stat, tile.InDetail),
	}
}

func (a *tracearr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, a.hc, a.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+a.key) })
}

func (a *tracearr) Run(ctx context.Context, emit func(tile.Values, error)) {
	run := func(d time.Duration, f func(context.Context) (tile.Values, error)) {
		every(ctx, d, func() {
			v, err := f(ctx)
			if ctx.Err() == nil {
				emit(v, err)
			}
		})
	}
	go run(time.Minute, a.counters)
	go run(5*time.Minute, a.month)
	run(5*time.Second, a.streams)
}

func (a *tracearr) streams(ctx context.Context) (tile.Values, error) {
	var r struct {
		Data []struct {
			Username, MediaTitle, MediaType, ShowTitle    string
			SeasonNumber, EpisodeNumber, Year             int
			State, VideoDecision, AudioDecision           string
			Bitrate                                       float64 // kbit/s
			Resolution, SourceVideoCodecDisplay           string
			SourceAudioCodecDisplay, AudioChannelsDisplay string
			SourceVideoDetails                            struct{ DynamicRange string }
			IsTranscode                                   bool
		}
		Summary struct {
			Total, Transcodes, DirectStreams, DirectPlays int
			TotalBitrate                                  string
		}
	}
	if err := a.get(ctx, "/streams", &r); err != nil {
		return nil, err
	}
	var rows [][]tile.Cell
	for _, s := range r.Data {
		name := s.MediaTitle
		if s.MediaType == "episode" {
			name = fmt.Sprintf("%s S%02dE%02d", s.ShowTitle, s.SeasonNumber, s.EpisodeNumber)
		}
		video := strings.TrimSpace(s.Resolution + " " + s.SourceVideoCodecDisplay + " " + s.SourceVideoDetails.DynamicRange)
		mode, lvl := "Lecture directe", "ok"
		switch {
		case s.VideoDecision == "transcode":
			mode, lvl = "Transcodage vidéo", "warn"
		case s.AudioDecision == "transcode":
			mode, lvl = "Audio transcodé", ""
		case s.VideoDecision == "copy":
			mode = "Flux direct"
		}
		rows = append(rows, []tile.Cell{{Text: s.Username}, {Text: name}, {Text: video}, {Text: s.SourceAudioCodecDisplay + " " + s.AudioChannelsDisplay},
			{Text: mode, Level: lvl}, {Text: tile.Rate(s.Bitrate * 125)}})
	}
	if len(rows) > 15 {
		rows = rows[:15]
	}
	su := r.Summary
	return tile.Values{
		"streams": {Text: fmt.Sprint(su.Total), Sub: fmt.Sprintf("%d directe(s), %d flux direct, %d transcodage(s) · %s", su.DirectPlays, su.DirectStreams, su.Transcodes, strings.Replace(su.TotalBitrate, ".", ",", 1))},
		"details": {Head: []string{"Utilisateur", "Titre", "Vidéo", "Audio", "Mode", "Débit"}, Rows: rows},
	}, nil
}

func (a *tracearr) counters(ctx context.Context) (tile.Values, error) {
	var today struct {
		TodayPlays, TodaySessions, ActiveUsersToday, AlertsLast24h int
		WatchTimeHours                                             float64
	}
	if err := a.get(ctx, "/stats/today", &today); err != nil {
		return nil, err
	}
	v := tile.Values{"today": {
		Text: fmt.Sprint(today.TodayPlays),
		Sub:  fmt.Sprintf("%d sessions, %d comptes, %s h", today.TodaySessions, today.ActiveUsersToday, tile.Num(today.WatchTimeHours)),
	}}

	// Le reste est facultatif : une route en erreur n'efface pas la tuile.
	var st struct{ TotalUsers, TotalSessions, RecentViolations int }
	if a.get(ctx, "/stats", &st) == nil {
		viol := tile.Value{Text: fmt.Sprint(st.RecentViolations), Level: "ok"}
		if st.RecentViolations > 0 {
			viol.Level = "crit"
		}
		var vl struct{ Meta struct{ Total int } }
		if a.get(ctx, "/violations?pageSize=1", &vl) == nil {
			viol.Sub = fmt.Sprintf("%d au total", vl.Meta.Total)
		}
		v["violations"] = viol
		v["users"] = tile.Value{Text: fmt.Sprint(st.TotalUsers), Sub: fmt.Sprintf("%d sessions enregistrées", st.TotalSessions)}
	}

	var h struct {
		Servers []struct {
			Name, Type    string
			Online        bool
			ActiveStreams int
		}
		Version string
	}
	if a.get(ctx, "/health", &h) == nil {
		var names []string
		lvl := "ok"
		for _, s := range h.Servers {
			state := "en ligne"
			if !s.Online {
				state, lvl = "hors ligne", "crit"
			}
			names = append(names, fmt.Sprintf("%s (%s, %s)", s.Name, s.Type, state))
		}
		v["server"] = tile.Value{Text: fmt.Sprintf("%d serveur(s)", len(h.Servers)), Sub: strings.Join(names, ", ") + " · Tracearr " + h.Version, Level: lvl}
	}
	return v, nil
}

// month : activité, qualité, plateformes (30 j) et comptes dont la confiance baisse.
func (a *tracearr) month(ctx context.Context) (tile.Values, error) {
	var act struct {
		Plays []struct {
			Date  string
			Count float64
		}
		Platforms []struct {
			Platform string
			Count    int
		}
		Quality struct {
			DirectPlayPercent, DirectStreamPercent, TranscodePercent, AudioTranscodePercent float64
			Total                                                                           int
		}
	}
	if err := a.get(ctx, "/activity", &act); err != nil {
		return nil, err
	}
	v := tile.Values{}
	days := map[string]float64{} // une ligne par serveur et par date : on additionne
	var order []string
	for _, p := range act.Plays {
		d := p.Date[:min(10, len(p.Date))]
		if _, ok := days[d]; !ok {
			order = append(order, d)
		}
		days[d] += p.Count
	}
	sort.Strings(order)
	var pts []float64
	for _, d := range order {
		pts = append(pts, days[d])
	}
	if len(pts) > 0 {
		v["plays"] = tile.Value{Text: tile.Num(pts[len(pts)-1]) + " aujourd'hui", Points: pts}
	}
	q := act.Quality
	v["quality"] = tile.Value{Text: fmt.Sprintf("%d sessions", q.Total), Bars: []tile.Bar{
		{Label: "Lecture directe", Pct: q.DirectPlayPercent, Text: tile.Pct(q.DirectPlayPercent), Level: "ok"},
		{Label: "Flux direct", Pct: q.DirectStreamPercent, Text: tile.Pct(q.DirectStreamPercent)},
		{Label: "Transcodage", Pct: q.TranscodePercent, Text: tile.Pct(q.TranscodePercent), Level: tile.Level(q.TranscodePercent, 30, 50)},
	}}
	var rows [][]tile.Cell
	for i, p := range act.Platforms {
		if i == 15 {
			break
		}
		rows = append(rows, []tile.Cell{{Text: p.Platform}, {Text: fmt.Sprint(p.Count)}})
	}
	v["platforms"] = tile.Value{Head: []string{"Plateforme", "Sessions"}, Rows: rows}

	var u struct {
		Data []struct {
			Username, DisplayName, Role string
			TrustScore, TotalViolations int
			LastActivityAt              *time.Time
		}
	}
	if a.get(ctx, "/users?pageSize=100", &u) == nil {
		rows = nil
		sort.SliceStable(u.Data, func(i, j int) bool { return u.Data[i].TrustScore < u.Data[j].TrustScore })
		for _, x := range u.Data {
			if x.TrustScore >= 100 && x.TotalViolations == 0 {
				continue
			}
			last := "jamais"
			if x.LastActivityAt != nil {
				last = tile.Ago(*x.LastActivityAt)
			}
			rows = append(rows, []tile.Cell{{Text: x.DisplayName}, {Text: fmt.Sprint(x.TrustScore), Level: tile.Level(100-float64(x.TrustScore), 10, 30)}, {Text: fmt.Sprint(x.TotalViolations)}, {Text: last}})
		}
		if len(rows) > 15 {
			rows = rows[:15]
		}
		v["trust"] = tile.Value{Head: []string{"Compte", "Confiance", "Violations", "Dernière activité"}, Rows: rows}
	}
	return v, nil
}
