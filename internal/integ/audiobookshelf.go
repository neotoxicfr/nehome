package integ

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Audiobookshelf : jeton d'API en Bearer. Sessions ouvertes toutes les 15 s ;
// bibliothèques et version toutes les 5 min.
type audiobookshelf struct {
	url, token string
	hc         *http.Client
}

func newAudiobookshelf(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	tok, err := secret("token")
	if err != nil {
		return nil, err
	}
	return &audiobookshelf{url: strings.TrimRight(t.URL, "/"), token: tok, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (a *audiobookshelf) Fields() []tile.Field {
	return []tile.Field{
		tile.F("items", "Éléments", tile.Stat, tile.OnTile),
		tile.F("streams", "Écoutes en cours", tile.Stat, tile.OnTile),
		tile.F("duration", "Durée totale", tile.Stat, tile.OnTile),
		tile.F("size", "Taille", tile.Stat, tile.OnTile),
		tile.F("authors", "Auteurs", tile.Stat, tile.InDetail),
		tile.F("libraries", "Bibliothèques", tile.Table, tile.InDetail),
		tile.F("sessions", "Sessions ouvertes", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (a *audiobookshelf) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, a.hc, a.url+path, out, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+a.token) })
}

func (a *audiobookshelf) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		v := a.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, 15*time.Second, func() {
		v, err := a.sessions(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (a *audiobookshelf) sessions(ctx context.Context) (tile.Values, error) {
	var o struct {
		UsersOnline  []struct{ ID, Username string }
		OpenSessions []struct {
			UserID        string `json:"userId"`
			DisplayTitle  string
			DisplayAuthor string
			CurrentTime   float64
			Duration      float64
			DeviceInfo    struct {
				ClientName string
				DeviceName string
			}
		}
	}
	if err := a.get(ctx, "/api/users/online", &o); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, u := range o.UsersOnline {
		names[u.ID] = u.Username
	}
	tbl := tile.Value{Head: []string{"Utilisateur", "Titre", "Avancement", "Appareil"}}
	for _, s := range o.OpenSessions {
		prog := ""
		if s.Duration > 0 {
			prog = tile.Pct(s.CurrentTime / s.Duration * 100)
		}
		dev := s.DeviceInfo.ClientName
		if s.DeviceInfo.DeviceName != "" {
			dev = strings.TrimPrefix(dev+" · "+s.DeviceInfo.DeviceName, " · ")
		}
		if len(tbl.Rows) < 15 {
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: cmp.Or(names[s.UserID], s.UserID)}, {Text: arrPair(s.DisplayAuthor, s.DisplayTitle)}, {Text: prog}, {Text: dev}})
		}
	}
	return tile.Values{"streams": {Text: tile.Num(float64(len(o.OpenSessions)))}, "sessions": tbl}, nil
}

func (a *audiobookshelf) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var st struct {
		ServerVersion string `json:"serverVersion"`
	}
	if a.get(ctx, "/status", &st) == nil && st.ServerVersion != "" {
		v["version"] = tile.Value{Text: st.ServerVersion}
	}
	var libs struct {
		Libraries []struct {
			ID, Name, MediaType string
		}
	}
	if a.get(ctx, "/api/libraries", &libs) != nil {
		return v
	}
	var items, dur, size, authors float64
	tbl := tile.Value{Head: []string{"Bibliothèque", "Type", "Éléments", "Durée", "Taille"}}
	ok := false
	for _, l := range libs.Libraries {
		var s struct {
			TotalItems    float64
			TotalAuthors  float64
			TotalDuration float64 // secondes
			TotalSize     float64
		}
		if a.get(ctx, "/api/libraries/"+l.ID+"/stats", &s) != nil {
			continue
		}
		ok = true
		items += s.TotalItems
		dur += s.TotalDuration
		size += s.TotalSize
		authors += s.TotalAuthors
		kind := "Livres"
		if l.MediaType == "podcast" {
			kind = "Podcasts"
		}
		if len(tbl.Rows) < 15 {
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: l.Name}, {Text: kind}, {Text: tile.Num(s.TotalItems)},
				{Text: tile.Dur(time.Duration(s.TotalDuration) * time.Second)}, {Text: tile.Bytes(s.TotalSize)}})
		}
	}
	if ok {
		v["items"] = tile.Value{Text: tile.Num(items), Sub: fmt.Sprintf("%d bibliothèque%s", len(libs.Libraries), arrPlural(len(libs.Libraries)))}
		v["duration"] = tile.Value{Text: fmt.Sprintf("%s h", tile.Num(dur/3600))}
		v["size"] = tile.Value{Text: tile.Bytes(size)}
		v["authors"] = tile.Value{Text: tile.Num(authors)}
		v["libraries"] = tbl
	}
	return v
}
