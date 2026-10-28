package integ

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Plex (GET seulement, jeton en en-tête). Lectures en cours toutes les 5 s ;
// ajouts, bibliothèques et activités toutes les minutes (comptes : 5 min).
type plex struct {
	url, token string
	hc         *http.Client
}

func newPlex(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	p := &plex{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	var err error
	if p.token, err = secret("token"); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *plex) Fields() []tile.Field {
	return []tile.Field{
		tile.F("streams", "Lectures en cours", tile.Stat, tile.OnTile),
		tile.F("bandwidth", "Débit sortant", tile.Stat, tile.OnTile),
		tile.F("movies", "Films", tile.Stat, tile.OnTile),
		tile.F("shows", "Séries et animés", tile.Stat, tile.OnTile),
		tile.F("sessions", "Détail des lectures", tile.Table, tile.InDetail),
		tile.F("recent", "Ajouts récents", tile.Table, tile.InDetail),
		tile.F("libraries", "Bibliothèques", tile.Table, tile.InDetail),
		tile.F("activities", "Activités du serveur", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (p *plex) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, p.hc, p.url+path, out, func(r *http.Request) {
		r.Header.Set("X-Plex-Token", p.token)
		r.Header.Set("Accept", "application/json")
	})
}

func (p *plex) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 3)
	go every(ctx, 5*time.Second, func() {
		v, err := p.sessions(ctx)
		if ctx.Err() == nil {
			e[0](v, err)
		}
	})
	go every(ctx, 5*time.Minute, func() {
		if v, err := p.libraries(ctx); err == nil && ctx.Err() == nil {
			e[1](v, nil)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := p.slow(ctx)
		if ctx.Err() == nil {
			e[2](v, err)
		}
	})
}

type plexSession struct {
	Type, Title, GrandparentTitle string
	Index, ParentIndex            int
	ViewOffset, Duration          float64
	User                          struct{ Title string }
	Player                        struct{ Title, Product, State string }
	Session                       struct{ Bandwidth float64 } // kbit/s
	TranscodeSession              *struct {
		VideoDecision, AudioDecision string
		TranscodeHwEncoding          string
		TranscodeHwFullPipeline      bool
	}
	Media []struct{ Part []struct{ Decision string } }
}

// plexName : « Série S02E66 », « Série – saison 2 » ou « Film ».
func plexName(typ, title, show string, season, ep int) string {
	switch typ {
	case "episode":
		return fmt.Sprintf("%s S%02dE%02d", show, season, ep)
	case "season":
		return fmt.Sprintf("%s, saison %d", show, ep)
	}
	return title
}

// plexMode : mode de lecture et couleur (le transcodage logiciel coûte du CPU).
func plexMode(s plexSession) (string, string) {
	if ts := s.TranscodeSession; ts != nil {
		if ts.VideoDecision == "transcode" {
			if ts.TranscodeHwEncoding != "" || ts.TranscodeHwFullPipeline {
				return "Transcodage matériel", ""
			}
			return "Transcodage logiciel", "warn"
		}
		if ts.AudioDecision == "transcode" {
			return "Audio transcodé", ""
		}
	}
	if len(s.Media) > 0 && len(s.Media[0].Part) > 0 && s.Media[0].Part[0].Decision == "copy" {
		return "Flux direct", "ok"
	}
	return "Lecture directe", "ok"
}

func (p *plex) sessions(ctx context.Context) (tile.Values, error) {
	var r struct {
		MediaContainer struct{ Metadata []plexSession }
	}
	if err := p.get(ctx, "/status/sessions", &r); err != nil {
		return nil, err
	}
	var rows [][]tile.Cell
	var kbps float64
	var direct, hw, sw int
	for _, s := range r.MediaContainer.Metadata {
		mode, lvl := plexMode(s)
		switch {
		case strings.HasPrefix(mode, "Transcodage m"):
			hw++
		case strings.HasPrefix(mode, "Transcodage l"):
			sw++
		case lvl == "ok":
			direct++
		}
		kbps += s.Session.Bandwidth
		pct := 0.0
		if s.Duration > 0 {
			pct = s.ViewOffset / s.Duration * 100
		}
		state := ""
		if s.Player.State == "paused" {
			state = " (pause)"
		}
		rows = append(rows, []tile.Cell{
			{Text: s.User.Title}, {Text: plexName(s.Type, s.Title, s.GrandparentTitle, s.ParentIndex, s.Index)},
			{Text: tile.Pct(pct) + state}, {Text: s.Player.Title + " · " + s.Player.Product},
			{Text: mode, Level: lvl}, {Text: tile.Rate(s.Session.Bandwidth * 125)},
		})
	}
	n := len(rows)
	sub := "aucune lecture"
	if n > 0 {
		sub = fmt.Sprintf("%d directe(s), %d transcodage(s) matériel, %d logiciel", direct, hw, sw)
	}
	lvl := ""
	if sw > 0 {
		lvl = "warn"
	}
	if len(rows) > 15 {
		rows = rows[:15]
	}
	return tile.Values{
		"streams":   {Text: fmt.Sprint(n), Sub: sub, Level: lvl},
		"bandwidth": {Text: tile.Rate(kbps * 125)},
		"sessions":  {Head: []string{"Utilisateur", "Titre", "Avancement", "Lecteur", "Mode", "Débit"}, Rows: rows},
	}, nil
}

// slow : ajouts récents, activités, version. Un échec secondaire n'efface rien.
func (p *plex) slow(ctx context.Context) (tile.Values, error) {
	var id struct{ MediaContainer struct{ Version string } }
	if err := p.get(ctx, "/identity", &id); err != nil {
		return nil, err
	}
	ver, _, _ := strings.Cut(id.MediaContainer.Version, "-")
	v := tile.Values{"version": {Text: ver}}

	var rec struct {
		MediaContainer struct {
			Metadata []struct {
				Type, Title, GrandparentTitle, ParentTitle, LibrarySectionTitle string
				Index, ParentIndex, Year                                        int
				AddedAt                                                         int64
			}
		}
	}
	if p.get(ctx, "/library/recentlyAdded?X-Plex-Container-Start=0&X-Plex-Container-Size=10", &rec) == nil {
		var rows [][]tile.Cell
		for _, m := range rec.MediaContainer.Metadata {
			show := m.GrandparentTitle
			if m.Type == "season" {
				show = m.ParentTitle
			}
			name := plexName(m.Type, m.Title, show, m.ParentIndex, m.Index)
			if m.Type == "movie" && m.Year > 0 {
				name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
			}
			rows = append(rows, []tile.Cell{{Text: name}, {Text: m.LibrarySectionTitle}, {Text: tile.Ago(time.Unix(m.AddedAt, 0))}})
		}
		v["recent"] = tile.Value{Head: []string{"Titre", "Bibliothèque", "Ajouté"}, Rows: rows}
	}

	var act struct {
		MediaContainer struct {
			Activity []struct{ Title, Subtitle string }
		}
	}
	if p.get(ctx, "/activities", &act) == nil {
		a := act.MediaContainer.Activity
		if len(a) == 0 {
			v["activities"] = tile.Value{Text: "aucune", Level: "ok"}
		} else {
			var names []string
			for _, x := range a {
				names = append(names, x.Title)
			}
			v["activities"] = tile.Value{Text: fmt.Sprint(len(a)), Sub: strings.Join(names, ", ")}
		}
	}
	return v, nil
}

// libraries : comptes par section (une requête vide par section, totalSize).
func (p *plex) libraries(ctx context.Context) (tile.Values, error) {
	var secs struct {
		MediaContainer struct {
			Directory []struct{ Key, Type, Title string }
		}
	}
	if err := p.get(ctx, "/library/sections", &secs); err != nil {
		return nil, err
	}
	count := func(key, extra string) int {
		var r struct{ MediaContainer struct{ TotalSize int } }
		q := url.Values{"X-Plex-Container-Start": {"0"}, "X-Plex-Container-Size": {"0"}}
		p.get(ctx, "/library/sections/"+key+"/all?"+q.Encode()+extra, &r)
		return r.MediaContainer.TotalSize
	}
	var rows [][]tile.Cell
	var movies, shows, eps int
	for _, d := range secs.MediaContainer.Directory {
		n := count(d.Key, "")
		cell := []tile.Cell{{Text: d.Title}, {Text: fmt.Sprint(n)}, {}}
		switch d.Type {
		case "movie":
			movies += n
			cell[2].Text = "films"
		case "show":
			e := count(d.Key, "&type=4")
			shows += n
			eps += e
			cell[2].Text = fmt.Sprintf("séries, %d épisodes", e)
		default:
			cell[2].Text = "éléments"
		}
		rows = append(rows, cell)
	}
	return tile.Values{
		"movies":    {Text: fmt.Sprint(movies)},
		"shows":     {Text: fmt.Sprint(shows), Sub: fmt.Sprintf("%d épisodes", eps)},
		"libraries": {Head: []string{"Bibliothèque", "Nombre", "Contenu"}, Rows: rows},
	}, nil
}

func (p *plex) Actions() []tile.Action {
	return []tile.Action{{ID: "scan", Label: "Analyser les bibliothèques"}}
}

// Do : GET /library/sections/{key}/refresh pour chaque bibliothèque.
func (p *plex) Do(ctx context.Context, id, _ string) (string, error) {
	if id != "scan" {
		return "", errUnknownAction
	}
	var secs struct {
		MediaContainer struct{ Directory []struct{ Key string } }
	}
	if err := p.get(ctx, "/library/sections", &secs); err != nil {
		return "", err
	}
	n := 0
	for _, d := range secs.MediaContainer.Directory {
		if d.Key == "" {
			continue
		}
		err := callAction(ctx, p.hc, http.MethodGet, p.url+"/library/sections/"+url.PathEscape(d.Key)+"/refresh", nil, func(r *http.Request) { r.Header.Set("X-Plex-Token", p.token) })
		if err != nil {
			return "", err
		}
		n++
	}
	return fmt.Sprintf("Analyse lancée sur %d bibliothèque(s)", n), nil
}
