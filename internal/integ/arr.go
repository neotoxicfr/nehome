package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Sonarr et Radarr (API v3), Lidarr et Readarr (API v1) : GET seulement,
// en-tête X-Api-Key. Un seul code pour les quatre : seuls les champs des
// enregistrements changent. File,
// manquants et derniers imports toutes les 30 s ; calendrier, bibliothèque
// (liste complète, plusieurs Mo) et disques toutes les 5 min.
type arr struct {
	radarr   bool
	lidarr   bool // artistes / albums / pistes (API v1)
	readarr  bool // auteurs / livres (API v1)
	url, key string
	hc       *http.Client
}

func newSonarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newArr(false, t, secret)
}

func newRadarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newArr(true, t, secret)
}

func newLidarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newArrV1(&arr{lidarr: true}, t, secret)
}

func newReadarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newArrV1(&arr{readarr: true}, t, secret)
}

func newArr(radarr bool, t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &arr{radarr: radarr, url: strings.TrimRight(t.URL, "/") + "/api/v3", key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

func newArrV1(a *arr, t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	a.url, a.key, a.hc = strings.TrimRight(t.URL, "/")+"/api/v1", key, httpClient(t.InsecureTLS, false)
	return a, nil
}

func (a *arr) noun() string {
	switch {
	case a.radarr:
		return "Films"
	case a.lidarr:
		return "Artistes"
	case a.readarr:
		return "Auteurs"
	}
	return "Séries"
}

func (a *arr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("queue", "File d'attente", tile.Stat, tile.OnTile),
		tile.F("missing", "Manquants", tile.Stat, tile.OnTile),
		tile.F("health", "Santé", tile.Stat, tile.OnTile),
		tile.F("library", a.noun(), tile.Stat, tile.OnTile),
		tile.F("cutoff", "Cutoff non atteint", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("downloads", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("calendar", "Calendrier (7 jours)", tile.Table, tile.InDetail),
		tile.F("history", "Derniers imports", tile.Table, tile.InDetail),
		tile.F("disks", "Espace disque", tile.Bars, tile.InDetail),
		tile.F("tasks", "Tâches planifiées", tile.Table, tile.InDetail),
	}
}

func (a *arr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, a.hc, a.url+"/"+path, out, func(r *http.Request) { r.Header.Set("X-Api-Key", a.key) })
}

func (a *arr) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		v := a.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, 30*time.Second, func() {
		v, err := a.fast(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

type arrPage[T any] struct {
	TotalRecords int `json:"totalRecords"`
	Records      []T `json:"records"`
}

type arrQueueItem struct {
	Title                 string  `json:"title"`
	Status                string  `json:"status"`
	TrackedDownloadStatus string  `json:"trackedDownloadStatus"`
	TrackedDownloadState  string  `json:"trackedDownloadState"`
	ErrorMessage          string  `json:"errorMessage"`
	DownloadClient        string  `json:"downloadClient"`
	Size                  float64 `json:"size"`
	SizeLeft              float64 `json:"sizeleft"`
	Series                struct {
		Title string `json:"title"`
	} `json:"series"`
	Episode struct {
		Season int `json:"seasonNumber"`
		Number int `json:"episodeNumber"`
	} `json:"episode"`
	Movie struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"movie"`
	Artist struct {
		Name string `json:"artistName"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
	} `json:"album"`
	Author struct {
		Name string `json:"authorName"`
	} `json:"author"`
	Book struct {
		Title string `json:"title"`
	} `json:"book"`
	StatusMessages []struct {
		Messages []string `json:"messages"`
	} `json:"statusMessages"`
}

type arrHistoryItem struct {
	SourceTitle string    `json:"sourceTitle"`
	Date        time.Time `json:"date"`
	Quality     struct {
		Quality struct {
			Name string `json:"name"`
		} `json:"quality"`
	} `json:"quality"`
	Series struct {
		Title string `json:"title"`
	} `json:"series"`
	Episode struct {
		Season int `json:"seasonNumber"`
		Number int `json:"episodeNumber"`
	} `json:"episode"`
	Movie struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"movie"`
	Artist struct {
		Name string `json:"artistName"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
	} `json:"album"`
	Author struct {
		Name string `json:"authorName"`
	} `json:"author"`
	Book struct {
		Title string `json:"title"`
	} `json:"book"`
}

// arrPair : « Artiste – Album » ou « Auteur – Livre » (le seul nom si le titre manque).
func arrPair(who, what string) string {
	if what == "" {
		return who
	}
	return who + " – " + what
}

// name : « Série S01E08 » ou « Film (2026) ».
func (h arrHistoryItem) name() string {
	switch {
	case h.Artist.Name != "":
		return arrPair(h.Artist.Name, h.Album.Title)
	case h.Author.Name != "":
		return arrPair(h.Author.Name, h.Book.Title)
	case h.Movie.Title != "":
		return fmt.Sprintf("%s (%d)", h.Movie.Title, h.Movie.Year)
	case h.Series.Title != "":
		return fmt.Sprintf("%s S%02dE%02d", h.Series.Title, h.Episode.Season, h.Episode.Number)
	}
	return h.SourceTitle
}

var arrStates = map[string]string{
	"downloading": "Téléchargement", "importPending": "Import en attente", "importBlocked": "Import bloqué",
	"importing": "Import", "imported": "Importé", "failedPending": "Échec (nouvel essai)", "failed": "Échec", "ignored": "Ignoré",
}

func (a *arr) fast(ctx context.Context) (tile.Values, error) {
	var q arrPage[arrQueueItem]
	if err := a.get(ctx, "queue?pageSize=50&includeSeries=true&includeEpisode=true&includeMovie=true&includeArtist=true&includeAlbum=true&includeAuthor=true&includeBook=true", &q); err != nil {
		return nil, err
	}
	v := tile.Values{}
	var left float64
	lvl, bad := "", 0
	tbl := tile.Value{Head: []string{"Titre", "Avancement", "Reste", "État", "Client"}}
	for _, it := range q.Records {
		left += it.SizeLeft
		cl := ""
		switch it.TrackedDownloadStatus {
		case "error":
			cl, lvl = "crit", "crit"
			bad++
		case "warning":
			cl = "warn"
			bad++
			if lvl == "" {
				lvl = "warn"
			}
		}
		if len(tbl.Rows) >= 15 {
			continue
		}
		name := it.Title
		switch {
		case it.Artist.Name != "":
			name = arrPair(it.Artist.Name, it.Album.Title)
		case it.Author.Name != "":
			name = arrPair(it.Author.Name, it.Book.Title)
		case it.Movie.Title != "":
			name = it.Movie.Title
		case it.Series.Title != "" && it.Episode.Number > 0:
			name = fmt.Sprintf("%s S%02dE%02d", it.Series.Title, it.Episode.Season, it.Episode.Number)
		case it.Series.Title != "":
			name = it.Series.Title
		}
		prog := 0.0
		if it.Size > 0 {
			prog = (it.Size - it.SizeLeft) / it.Size * 100
		}
		state := arrStates[it.TrackedDownloadState]
		if state == "" {
			state = it.Status
		}
		tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: name}, {Text: tile.Pct(prog)}, {Text: tile.Bytes(it.SizeLeft)}, {Text: state, Level: cl}, {Text: it.DownloadClient}})
	}
	qv := tile.Value{Text: tile.Num(float64(q.TotalRecords)), Level: lvl}
	if q.TotalRecords > 0 {
		qv.Sub = tile.Bytes(left) + " restants"
		if bad > 0 {
			qv.Sub += fmt.Sprintf(" · %d à vérifier", bad)
		}
	}
	v["queue"] = qv
	v["downloads"] = tbl

	// Le reste est facultatif : une requête en échec n'efface pas la tuile.
	var m, c arrPage[struct{}]
	if a.get(ctx, "wanted/missing?pageSize=1", &m) == nil {
		v["missing"] = tile.Value{Text: tile.Num(float64(m.TotalRecords))}
	}
	if a.get(ctx, "wanted/cutoff?pageSize=1", &c) == nil {
		v["cutoff"] = tile.Value{Text: tile.Num(float64(c.TotalRecords))}
	}
	var hl []struct{ Type, Message string }
	if a.get(ctx, "health", &hl) == nil {
		hv := tile.Value{Text: "OK", Level: "ok"}
		if len(hl) > 0 {
			hv = tile.Value{Text: fmt.Sprintf("%d alerte%s", len(hl), arrPlural(len(hl))), Level: "warn", Sub: hl[0].Message}
			for _, h := range hl {
				if h.Type == "error" {
					hv.Level = "crit"
				}
			}
		}
		v["health"] = hv
	}
	var h arrPage[arrHistoryItem]
	if a.get(ctx, "history?pageSize=10&eventType=3&includeSeries=true&includeEpisode=true&includeMovie=true&includeArtist=true&includeAlbum=true&includeAuthor=true&includeBook=true", &h) == nil {
		tv := tile.Value{Head: []string{"Titre", "Qualité", "Importé"}}
		for _, it := range h.Records {
			tv.Rows = append(tv.Rows, []tile.Cell{{Text: it.name()}, {Text: it.Quality.Quality.Name}, {Text: tile.Ago(it.Date)}})
		}
		v["history"] = tv
	}
	return v, nil
}

func arrPlural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

var arrDays = [...]string{"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam."}

func arrDate(t time.Time) string {
	t = t.In(arrLoc)
	return fmt.Sprintf("%s %02d/%02d %02d:%02d", arrDays[t.Weekday()], t.Day(), int(t.Month()), t.Hour(), t.Minute())
}

var arrLoc = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Paris"); err == nil {
		return l
	}
	return time.Local
}()

type arrCalItem struct {
	Title      string    `json:"title"`
	AirDateUTC time.Time `json:"airDateUtc"`
	Season     int       `json:"seasonNumber"`
	Number     int       `json:"episodeNumber"`
	HasFile    bool      `json:"hasFile"`
	Series     struct {
		Title string `json:"title"`
	} `json:"series"`
	InCinemas, DigitalRelease, PhysicalRelease time.Time
	ReleaseDate                                time.Time // Lidarr, Readarr
	Artist                                     struct {
		Name string `json:"artistName"`
	} `json:"artist"`
	Author struct {
		Name string `json:"authorName"`
	} `json:"author"`
	Statistics struct {
		PercentOfTracks float64
		BookFileCount   int
	} `json:"statistics"`
}

func (a *arr) calendar(ctx context.Context) (tile.Value, error) {
	now := time.Now()
	var cal []arrCalItem
	q := fmt.Sprintf("calendar?start=%s&end=%s&includeSeries=true&includeArtist=true&includeAuthor=true", now.Format("2006-01-02"), now.AddDate(0, 0, 7).Format("2006-01-02"))
	if err := a.get(ctx, q, &cal); err != nil {
		return tile.Value{}, err
	}
	type row struct {
		at   time.Time
		cols []tile.Cell
	}
	var rows []row
	for _, c := range cal {
		var r row
		state := tile.Cell{Text: "Attendu"}
		if c.HasFile {
			state = tile.Cell{Text: "Téléchargé", Level: "ok"}
		}
		if a.lidarr || a.readarr {
			who := c.Artist.Name
			if a.readarr {
				who = c.Author.Name
			}
			r.at = c.ReleaseDate
			got := c.Statistics.PercentOfTracks >= 100 || c.Statistics.BookFileCount > 0
			state = tile.Cell{Text: "Attendu"}
			switch {
			case got:
				state = tile.Cell{Text: "Téléchargé", Level: "ok"}
			case r.at.Before(now):
				state = tile.Cell{Text: "Manquant", Level: "warn"}
			}
			r.cols = []tile.Cell{{Text: r.at.In(arrLoc).Format("02/01")}, {Text: who}, {Text: c.Title}, state}
		} else if a.radarr {
			// Date de sortie retenue : la première qui tombe dans la fenêtre.
			label := ""
			for _, d := range []struct {
				t time.Time
				l string
			}{{c.InCinemas, "Cinéma"}, {c.DigitalRelease, "Numérique"}, {c.PhysicalRelease, "Disque"}} {
				if !d.t.IsZero() && d.t.After(now.AddDate(0, 0, -1)) && d.t.Before(now.AddDate(0, 0, 8)) && (r.at.IsZero() || d.t.Before(r.at)) {
					r.at, label = d.t, d.l
				}
			}
			if r.at.IsZero() {
				continue
			}
			if !c.HasFile && r.at.Before(now) {
				state = tile.Cell{Text: "Manquant", Level: "warn"}
			}
			r.cols = []tile.Cell{{Text: r.at.In(arrLoc).Format("02/01")}, {Text: c.Title}, {Text: label}, state}
		} else {
			r.at = c.AirDateUTC
			if !c.HasFile && r.at.Before(now) {
				state = tile.Cell{Text: "Manquant", Level: "warn"}
			}
			r.cols = []tile.Cell{{Text: arrDate(r.at)}, {Text: c.Series.Title}, {Text: fmt.Sprintf("S%02dE%02d", c.Season, c.Number)}, state}
		}
		rows = append(rows, r)
	}
	slices.SortStableFunc(rows, func(x, y row) int { return x.at.Compare(y.at) })
	v := tile.Value{Head: []string{"Date", "Titre", "Épisode", "État"}}
	switch {
	case a.radarr:
		v.Head[2] = "Sortie"
	case a.lidarr:
		v.Head[1], v.Head[2] = "Artiste", "Album"
	case a.readarr:
		v.Head[1], v.Head[2] = "Auteur", "Livre"
	}
	for _, r := range rows {
		if len(v.Rows) < 15 {
			v.Rows = append(v.Rows, r.cols)
		}
	}
	return v, nil
}

// slow : chaque requête est indépendante, aucune ne bloque les autres.
func (a *arr) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	if cal, err := a.calendar(ctx); err == nil {
		v["calendar"] = cal
	}
	var st struct {
		Version   string    `json:"version"`
		StartTime time.Time `json:"startTime"`
	}
	if a.get(ctx, "system/status", &st) == nil {
		v["version"] = tile.Value{Text: st.Version, Sub: "en marche depuis " + tile.Dur(time.Since(st.StartTime))}
	}
	var lib []struct {
		Monitored  bool    `json:"monitored"`
		HasFile    bool    `json:"hasFile"`
		SizeOnDisk float64 `json:"sizeOnDisk"`
		Statistics struct {
			SizeOnDisk       float64 `json:"sizeOnDisk"`
			EpisodeFileCount int     `json:"episodeFileCount"`
			TrackFileCount   int     `json:"trackFileCount"`
			BookFileCount    int     `json:"bookFileCount"`
		} `json:"statistics"`
	}
	path := "series"
	switch {
	case a.radarr:
		path = "movie"
	case a.lidarr:
		path = "artist"
	case a.readarr:
		path = "author"
	}
	if a.get(ctx, path, &lib) == nil {
		var size float64
		files, mon := 0, 0
		for _, it := range lib {
			if a.radarr {
				size += it.SizeOnDisk
				if it.HasFile {
					files++
				}
			} else {
				size += it.Statistics.SizeOnDisk
				files += it.Statistics.EpisodeFileCount + it.Statistics.TrackFileCount + it.Statistics.BookFileCount
			}
			if it.Monitored {
				mon++
			}
		}
		unit := "épisodes"
		switch {
		case a.radarr:
			unit = "avec fichier"
		case a.lidarr:
			unit = "pistes"
		case a.readarr:
			unit = "livres"
		}
		v["library"] = tile.Value{Text: tile.Num(float64(len(lib))), Sub: fmt.Sprintf("%s · %s %s · %d suivis", tile.Bytes(size), tile.Num(float64(files)), unit, mon)}
	}
	var disks []struct {
		Path                  string
		FreeSpace, TotalSpace float64
	}
	if a.get(ctx, "diskspace", &disks) == nil {
		bv := tile.Value{}
		seen := map[[2]float64]bool{} // plusieurs points de montage d'un même volume
		for _, d := range disks {
			k := [2]float64{d.FreeSpace, d.TotalSpace}
			if d.TotalSpace <= 0 || seen[k] {
				continue
			}
			seen[k] = true
			pct := (d.TotalSpace - d.FreeSpace) / d.TotalSpace * 100
			bv.Bars = append(bv.Bars, tile.Bar{Label: d.Path, Text: tile.Bytes(d.FreeSpace) + " libres / " + tile.Bytes(d.TotalSpace), Pct: pct, Level: tile.Level(pct, 85, 95)})
		}
		v["disks"] = bv
	}
	var tasks []struct {
		Name          string
		LastExecution time.Time
		NextExecution time.Time
	}
	if a.get(ctx, "system/task", &tasks) == nil {
		tv := tile.Value{Head: []string{"Tâche", "Dernière", "Prochaine"}}
		for _, t := range tasks {
			next := ""
			if !t.NextExecution.IsZero() {
				next = arrDate(t.NextExecution)
			}
			tv.Rows = append(tv.Rows, []tile.Cell{{Text: t.Name}, {Text: tile.Ago(t.LastExecution)}, {Text: next}})
		}
		v["tasks"] = tv
	}
	return v
}

// Prowlarr (API v1, clé en en-tête) : indexeurs, statistiques sur 24 h et
// santé. Indexeurs et santé chaque minute, statistiques et version toutes les 5 min.
type prowlarr struct {
	url, key string
	hc       *http.Client
}

func newProwlarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &prowlarr{url: strings.TrimRight(t.URL, "/") + "/api/v1", key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (p *prowlarr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("indexers", "Indexeurs actifs", tile.Stat, tile.OnTile),
		tile.F("queries", "Recherches (24 h)", tile.Stat, tile.OnTile),
		tile.F("grabs", "Grabs (24 h)", tile.Stat, tile.OnTile),
		tile.F("health", "Santé", tile.Stat, tile.OnTile),
		tile.F("failed", "Recherches en échec (24 h)", tile.Stat, tile.InDetail),
		tile.F("down", "Indexeurs indisponibles", tile.Table, tile.InDetail),
		tile.F("stats", "Statistiques par indexeur (24 h)", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (p *prowlarr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, p.hc, p.url+"/"+path, out, func(r *http.Request) { r.Header.Set("X-Api-Key", p.key) })
}

func (p *prowlarr) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		v := p.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := p.fast(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (p *prowlarr) fast(ctx context.Context) (tile.Values, error) {
	var idx []struct {
		ID     int
		Name   string
		Enable bool
	}
	if err := p.get(ctx, "indexer", &idx); err != nil {
		return nil, err
	}
	var st []struct {
		IndexerID    int       `json:"indexerId"`
		DisabledTill time.Time `json:"disabledTill"`
	}
	statusOK := p.get(ctx, "indexerstatus", &st) == nil
	blocked := map[int]time.Time{}
	for _, s := range st {
		if s.DisabledTill.After(time.Now()) {
			blocked[s.IndexerID] = s.DisabledTill
		}
	}
	enabled, down := 0, tile.Value{Head: []string{"Indexeur", "Désactivé jusqu'à"}}
	for _, i := range idx {
		if !i.Enable {
			continue
		}
		enabled++
		if t, ok := blocked[i.ID]; ok && len(down.Rows) < 15 {
			down.Rows = append(down.Rows, []tile.Cell{{Text: i.Name, Level: "warn"}, {Text: arrDate(t)}})
		}
	}
	iv := tile.Value{Text: tile.Num(float64(enabled)), Sub: fmt.Sprintf("sur %d configurés", len(idx))}
	v := tile.Values{"indexers": iv}
	if statusOK {
		if n := len(blocked); n > 0 {
			iv.Level, iv.Sub = "warn", fmt.Sprintf("%d indisponible%s · %d configurés", n, arrPlural(n), len(idx))
			v["indexers"] = iv
		}
		v["down"] = down
	}
	p.health(ctx, v)
	return v, nil
}

func (p *prowlarr) health(ctx context.Context, v tile.Values) {
	var hl []struct{ Type, Message string }
	if p.get(ctx, "health", &hl) != nil {
		return
	}
	hv := tile.Value{Text: "OK", Level: "ok"}
	if len(hl) > 0 {
		hv = tile.Value{Text: fmt.Sprintf("%d alerte%s", len(hl), arrPlural(len(hl))), Level: "warn", Sub: hl[0].Message}
		for _, h := range hl {
			if h.Type == "error" {
				hv.Level = "crit"
			}
		}
	}
	v["health"] = hv
}

func (p *prowlarr) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var st struct {
		Version   string
		StartTime time.Time
	}
	if p.get(ctx, "system/status", &st) == nil {
		v["version"] = tile.Value{Text: st.Version, Sub: "en marche depuis " + tile.Dur(time.Since(st.StartTime))}
	}
	now := time.Now().UTC()
	var stats struct {
		Indexers []struct {
			Name          string  `json:"indexerName"`
			AvgResponse   float64 `json:"averageResponseTime"`
			Queries       int     `json:"numberOfQueries"`
			Grabs         int     `json:"numberOfGrabs"`
			FailedQueries int     `json:"numberOfFailedQueries"`
			FailedGrabs   int     `json:"numberOfFailedGrabs"`
		} `json:"indexers"`
	}
	q := fmt.Sprintf("indexerstats?startDate=%s&endDate=%s", now.Add(-24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339))
	if p.get(ctx, q, &stats) == nil {
		var queries, grabs, failed int
		tv := tile.Value{Head: []string{"Indexeur", "Recherches", "Grabs", "Échecs", "Réponse"}}
		for _, i := range stats.Indexers {
			queries += i.Queries
			grabs += i.Grabs
			failed += i.FailedQueries
			if len(tv.Rows) < 15 {
				cl := ""
				if i.FailedQueries+i.FailedGrabs > 0 {
					cl = "warn"
				}
				tv.Rows = append(tv.Rows, []tile.Cell{{Text: i.Name}, {Text: tile.Num(float64(i.Queries))}, {Text: tile.Num(float64(i.Grabs))},
					{Text: tile.Num(float64(i.FailedQueries + i.FailedGrabs)), Level: cl}, {Text: fmt.Sprintf("%.0f ms", i.AvgResponse)}})
			}
		}
		fl := ""
		if queries > 0 && float64(failed)/float64(queries) > 0.2 {
			fl = "warn"
		}
		v["queries"] = tile.Value{Text: tile.Num(float64(queries))}
		v["grabs"] = tile.Value{Text: tile.Num(float64(grabs))}
		v["failed"] = tile.Value{Text: tile.Num(float64(failed)), Level: fl}
		v["stats"] = tv
	}
	return v
}

// callAction envoie une commande (body encodé en JSON s'il n'est pas nil) et
// accepte toute réponse 2xx. Corps de réponse ignoré, lu de façon bornée ;
// l'erreur ne contient ni secret ni chaîne de requête.
func callAction(ctx context.Context, c *http.Client, method, url string, body any, set func(*http.Request)) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if set != nil {
		set(req)
	}
	resp, err := c.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError{resp.StatusCode, redact(url)}
	}
	return nil
}

var errUnknownAction = errors.New("action inconnue")

func (a *arr) missingCommand() string {
	switch {
	case a.radarr:
		return "MissingMoviesSearch"
	case a.lidarr:
		return "MissingAlbumSearch"
	case a.readarr:
		return "MissingBookSearch"
	}
	return "MissingEpisodeSearch"
}

func (a *arr) Actions() []tile.Action {
	return []tile.Action{
		{ID: "rss", Label: "Synchroniser les flux RSS"},
		{ID: "refresh", Label: "Rafraîchir la file"},
		{ID: "missing", Label: "Chercher les manquants", Confirm: "Lancer une recherche de tous les manquants ? Elle interroge tous les indexeurs."},
	}
}

// Do lance une commande *arr (POST /command), exécutée en tâche de fond par le service.
func (a *arr) Do(ctx context.Context, id, _ string) (string, error) {
	var name, msg string
	switch id {
	case "rss":
		name, msg = "RssSync", "Synchronisation RSS lancée"
	case "refresh":
		name, msg = "RefreshMonitoredDownloads", "Rafraîchissement de la file lancé"
	case "missing":
		name, msg = a.missingCommand(), "Recherche des manquants lancée"
	default:
		return "", errUnknownAction
	}
	err := callAction(ctx, a.hc, http.MethodPost, a.url+"/command", map[string]string{"name": name}, func(r *http.Request) { r.Header.Set("X-Api-Key", a.key) })
	if err != nil {
		return "", err
	}
	return msg, nil
}
