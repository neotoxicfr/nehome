package integ

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Jellyfin et Emby : même API, clé d'API en en-tête X-Emby-Token. Emby
// attend le préfixe /emby, Jellyfin non. Sessions toutes les 10 s ;
// compteurs de bibliothèque, informations serveur et journal d'activité
// toutes les 5 min.
type jellyfin struct {
	emby     bool
	url, key string
	hc       *http.Client
}

func newJellyfin(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newJelly(false, t, secret)
}

func newEmby(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newJelly(true, t, secret)
}

func newJelly(emby bool, t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(t.URL, "/")
	if emby {
		u += "/emby"
	}
	return &jellyfin{emby: emby, url: u, key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (j *jellyfin) Fields() []tile.Field {
	return []tile.Field{
		tile.F("streams", "Lectures en cours", tile.Stat, tile.OnTile),
		tile.F("transcodes", "Transcodages", tile.Stat, tile.OnTile),
		tile.F("movies", "Films", tile.Stat, tile.OnTile),
		tile.F("series", "Séries", tile.Stat, tile.OnTile),
		tile.F("episodes", "Épisodes", tile.Stat, tile.InDetail),
		tile.F("music", "Musique", tile.Stat, tile.InDetail),
		tile.F("books", "Livres", tile.Stat, tile.InDetail),
		tile.F("sessions", "Sessions en lecture", tile.Table, tile.InDetail),
		tile.F("activity", "Activité récente", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (j *jellyfin) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, j.hc, j.url+path, out, func(r *http.Request) {
		if j.emby {
			r.Header.Set("X-Emby-Token", j.key)
		} else { // Jellyfin 12 : X-Emby-Token n'est plus lu par défaut
			r.Header.Set("Authorization", `MediaBrowser Token="`+j.key+`"`)
		}
	})
}

func (j *jellyfin) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		v := j.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, 10*time.Second, func() {
		v, err := j.sessions(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (j *jellyfin) sessions(ctx context.Context) (tile.Values, error) {
	var ss []struct {
		UserName   string
		Client     string
		DeviceName string
		PlayState  struct {
			PositionTicks float64
			IsPaused      bool
			PlayMethod    string
		}
		NowPlayingItem *struct {
			Name         string
			SeriesName   string
			RunTimeTicks float64
		}
	}
	if err := j.get(ctx, "/Sessions", &ss); err != nil {
		return nil, err
	}
	n, trans := 0, 0
	tbl := tile.Value{Head: []string{"Utilisateur", "Titre", "Avancement", "Mode", "Appareil"}}
	for _, s := range ss {
		it := s.NowPlayingItem
		if it == nil {
			continue
		}
		n++
		mode := "Lecture directe"
		switch s.PlayState.PlayMethod {
		case "Transcode":
			trans++
			mode = "Transcodage"
		case "DirectStream":
			mode = "Flux direct"
		}
		if s.PlayState.IsPaused {
			mode = "En pause"
		}
		name := it.Name
		if it.SeriesName != "" {
			name = it.SeriesName + " – " + name
		}
		prog := ""
		if it.RunTimeTicks > 0 {
			prog = tile.Pct(s.PlayState.PositionTicks / it.RunTimeTicks * 100)
		}
		dev := s.DeviceName
		if s.Client != "" {
			dev = s.Client + " · " + dev
		}
		if len(tbl.Rows) < 15 {
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: s.UserName}, {Text: name}, {Text: prog}, {Text: mode}, {Text: dev}})
		}
	}
	sv := tile.Value{Text: tile.Num(float64(n))}
	if n > 0 {
		sv.Sub = fmt.Sprintf("%d transcodage%s", trans, arrPlural(trans))
	}
	return tile.Values{"streams": sv, "transcodes": {Text: tile.Num(float64(trans))}, "sessions": tbl}, nil
}

func (j *jellyfin) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var c struct {
		MovieCount, SeriesCount, EpisodeCount, SongCount, AlbumCount, ArtistCount, BookCount float64
	}
	if j.get(ctx, "/Items/Counts", &c) == nil {
		v["movies"] = tile.Value{Text: tile.Num(c.MovieCount)}
		v["series"] = tile.Value{Text: tile.Num(c.SeriesCount)}
		v["episodes"] = tile.Value{Text: tile.Num(c.EpisodeCount)}
		if c.SongCount > 0 {
			v["music"] = tile.Value{Text: tile.Num(c.SongCount), Sub: fmt.Sprintf("%s albums · %s artistes", tile.Num(c.AlbumCount), tile.Num(c.ArtistCount))}
		}
		if c.BookCount > 0 {
			v["books"] = tile.Value{Text: tile.Num(c.BookCount)}
		}
	}
	var info struct {
		ServerName         string
		Version            string
		HasPendingRestart  bool
		HasUpdateAvailable bool
	}
	if j.get(ctx, "/System/Info", &info) == nil {
		vv := tile.Value{Text: info.Version, Sub: info.ServerName}
		switch {
		case info.HasPendingRestart:
			vv.Level, vv.Sub = "warn", "redémarrage en attente"
		case info.HasUpdateAvailable:
			vv.Level, vv.Sub = "warn", "mise à jour disponible"
		}
		v["version"] = vv
	}
	var act struct {
		Items []struct {
			Name     string
			Severity string
			Date     time.Time
		}
	}
	if j.get(ctx, "/System/ActivityLog/Entries?limit=10", &act) == nil {
		tv := tile.Value{Head: []string{"Événement", "Quand"}}
		for _, e := range act.Items {
			cl := ""
			switch e.Severity {
			case "Warn", "Warning":
				cl = "warn"
			case "Error", "Fatal":
				cl = "crit"
			}
			tv.Rows = append(tv.Rows, []tile.Cell{{Text: e.Name, Level: cl}, {Text: tile.Ago(e.Date)}})
		}
		v["activity"] = tv
	}
	return v
}
