package integ

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Seerr (API v1, clé en en-tête). Tout est relevé chaque minute. Les titres
// des demandes ne sont pas dans /request : on les résout par TMDB, avec cache.
type seerr struct {
	url, key string
	hc       *http.Client

	mu      sync.Mutex
	titles  map[string]string // « movie/603 » → titre
	pending []string          // « #12 Matrix » : choix des actions, rempli par le sondage
}

func newSeerr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	s := &seerr{url: strings.TrimRight(t.URL, "/") + "/api/v1", hc: httpClient(t.InsecureTLS, false), titles: map[string]string{}}
	var err error
	if s.key, err = secret("key"); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *seerr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("pending", "Demandes en attente", tile.Stat, tile.OnTile),
		tile.F("processing", "Demandes en cours", tile.Stat, tile.OnTile),
		tile.F("issues", "Problèmes ouverts", tile.Stat, tile.OnTile),
		tile.F("total", "Demandes au total", tile.Stat, tile.InDetail),
		tile.F("requests", "Demandes à traiter", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (s *seerr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, s.hc, s.url+path, out, func(r *http.Request) { r.Header.Set("X-Api-Key", s.key) })
}

func (s *seerr) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := s.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type seerrRequest struct {
	ID        int
	Status    int
	Type      string
	CreatedAt time.Time
	Media     struct {
		TmdbID int
		Status int
	}
	RequestedBy struct{ DisplayName string }
}

// title résout un titre ; en cas d'échec, un repli lisible qui n'est pas mis en cache.
func (s *seerr) title(ctx context.Context, typ string, id int) string {
	key := fmt.Sprintf("%s/%d", typ, id)
	s.mu.Lock()
	t, ok := s.titles[key]
	s.mu.Unlock()
	if ok {
		return t
	}
	var r struct{ Title, Name string }
	if s.get(ctx, "/"+key, &r) != nil {
		return "TMDB " + key
	}
	if t = r.Title + r.Name; t == "" {
		return "TMDB " + key
	}
	s.mu.Lock()
	s.titles[key] = t
	s.mu.Unlock()
	return t
}

func (s *seerr) poll(ctx context.Context) (tile.Values, error) {
	var c struct{ Total, Movie, TV, Pending, Approved, Declined, Processing, Available, Completed int }
	if err := s.get(ctx, "/request/count", &c); err != nil {
		return nil, err
	}
	pend := tile.Value{Text: fmt.Sprint(c.Pending), Level: "ok"}
	if c.Pending > 0 {
		pend.Level = "warn"
	}
	v := tile.Values{
		"pending":    pend,
		"processing": {Text: fmt.Sprint(c.Processing), Sub: fmt.Sprintf("%d approuvées", c.Approved)},
		"total":      {Text: fmt.Sprint(c.Total), Sub: fmt.Sprintf("%d films, %d séries · %d disponibles, %d refusée(s)", c.Movie, c.TV, c.Completed, c.Declined)},
	}

	// Le reste est facultatif : une route en erreur n'efface pas la tuile.
	var is struct{ Total, Open, Closed int }
	if s.get(ctx, "/issue/count", &is) == nil {
		iv := tile.Value{Text: fmt.Sprint(is.Open), Sub: fmt.Sprintf("%d résolus", is.Closed), Level: "ok"}
		if is.Open > 0 {
			iv.Level = "warn"
		}
		v["issues"] = iv
	}

	var st struct {
		Version         string
		UpdateAvailable bool
	}
	if s.get(ctx, "/status", &st) == nil {
		ver := tile.Value{Text: st.Version}
		if st.UpdateAvailable {
			ver.Sub, ver.Level = "mise à jour disponible", "warn"
		}
		v["version"] = ver
	}

	var rows [][]tile.Cell
	for _, f := range []string{"pending", "processing"} {
		var l struct{ Results []seerrRequest }
		if s.get(ctx, "/request?take=15&sort=added&filter="+f, &l) != nil {
			continue
		}
		if f == "pending" {
			var choices []string
			for _, r := range l.Results {
				if r.Status == 1 {
					choices = append(choices, fmt.Sprintf("#%d %s", r.ID, s.title(ctx, r.Type, r.Media.TmdbID)))
				}
			}
			s.mu.Lock()
			s.pending = choices
			s.mu.Unlock()
		}
		for _, r := range l.Results {
			if len(rows) == 15 {
				break
			}
			state, lvl := "En cours", ""
			switch {
			case r.Status == 1:
				state, lvl = "En attente", "warn"
			case r.Media.Status == 4:
				state = "Partielle"
			}
			kind := "Film"
			if r.Type == "tv" {
				kind = "Série"
			}
			rows = append(rows, []tile.Cell{{Text: s.title(ctx, r.Type, r.Media.TmdbID)}, {Text: kind}, {Text: r.RequestedBy.DisplayName}, {Text: tile.Ago(r.CreatedAt)}, {Text: state, Level: lvl}})
		}
	}
	v["requests"] = tile.Value{Head: []string{"Titre", "Type", "Demandeur", "Demandé", "État"}, Rows: rows}
	return v, nil
}

// Actions : rien à proposer tant qu'aucune demande n'attend.
func (s *seerr) Actions() []tile.Action {
	s.mu.Lock()
	choices := slices.Clone(s.pending)
	s.mu.Unlock()
	if len(choices) == 0 {
		return nil
	}
	return []tile.Action{
		{ID: "approve", Label: "Approuver", Choices: choices},
		{ID: "decline", Label: "Refuser", Choices: slices.Clone(choices), Confirm: "Refuser cette demande ?"},
	}
}

// Do : POST /api/v1/request/{id}/approve|decline ; arg = « #<id> <titre> ».
func (s *seerr) Do(ctx context.Context, id, arg string) (string, error) {
	var msg string
	switch id {
	case "approve":
		msg = "Demande approuvée"
	case "decline":
		msg = "Demande refusée"
	default:
		return "", errUnknownAction
	}
	num, _, _ := strings.Cut(strings.TrimPrefix(arg, "#"), " ")
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 {
		return "", errors.New("demande introuvable")
	}
	err = callAction(ctx, s.hc, http.MethodPost, fmt.Sprintf("%s/request/%d/%s", s.url, n, id), nil, func(r *http.Request) { r.Header.Set("X-Api-Key", s.key) })
	if err != nil {
		return "", err
	}
	return msg, nil
}
