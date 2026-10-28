package integ

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Navidrome via l'API Subsonic (1.16.1). Authentification par jeton salé :
// t = md5(mot de passe + sel), s = sel, tirés à chaque requête (spec Subsonic).
// Le jeton passe dans la chaîne de requête, imposé par la spec : redact et
// cleanErr la retirent des erreurs. Lecture en cours toutes les 10 s, scan
// chaque minute, artistes et albums toutes les 5 min.
type navidrome struct {
	url, user, pass string
	hc              *http.Client
}

func newNavidrome(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &navidrome{url: strings.TrimRight(t.URL, "/") + "/rest", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if n.user = t.Options["user"]; n.user == "" { // identifiant non secret : option, sinon secret
		if n.user, err = secret("user"); err != nil {
			return nil, err
		}
	}
	if n.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *navidrome) Fields() []tile.Field {
	return []tile.Field{
		tile.F("playing", "Lectures en cours", tile.Stat, tile.OnTile),
		tile.F("tracks", "Morceaux", tile.Stat, tile.OnTile),
		tile.F("albums", "Albums", tile.Stat, tile.OnTile),
		tile.F("artists", "Artistes", tile.Stat, tile.OnTile),
		tile.F("scan", "Scan de la bibliothèque", tile.Stat, tile.InDetail),
		tile.F("now_playing", "En cours d'écoute", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

// subsonic enveloppe toutes les réponses.
type subsonic struct {
	Resp struct {
		Status        string
		ServerVersion string `json:"serverVersion"`
		Type          string
		Error         struct {
			Code    int
			Message string
		}
		NowPlaying struct {
			Entry []struct {
				Title, Artist, Album, Username, PlayerName string
				MinutesAgo                                 int
			}
		}
		ScanStatus struct {
			Scanning    bool
			Count       float64
			FolderCount float64
			LastScan    time.Time
		}
		Artists struct {
			Index []struct {
				Artist []struct{ AlbumCount float64 }
			}
		}
	} `json:"subsonic-response"`
}

func (n *navidrome) get(ctx context.Context, method string) (*subsonic, error) {
	salt := make([]byte, 8)
	rand.Read(salt)
	s := hex.EncodeToString(salt)
	sum := md5.Sum([]byte(n.pass + s))
	q := url.Values{"u": {n.user}, "t": {hex.EncodeToString(sum[:])}, "s": {s}, "v": {"1.16.1"}, "c": {"nehome"}, "f": {"json"}}
	var out subsonic
	if err := getJSON(ctx, n.hc, n.url+"/"+method+"?"+q.Encode(), &out, nil); err != nil {
		return nil, err
	}
	if out.Resp.Status != "ok" {
		return nil, fmt.Errorf("Subsonic %s : %s (code %d)", method, out.Resp.Error.Message, out.Resp.Error.Code)
	}
	return &out, nil
}

func (n *navidrome) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 3)
	go every(ctx, 5*time.Minute, func() {
		v := n.slow(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	go every(ctx, time.Minute, func() {
		if v := n.scan(ctx); ctx.Err() == nil && len(v) > 0 {
			e[1](v, nil)
		}
	})
	every(ctx, 10*time.Second, func() {
		v, err := n.nowPlaying(ctx)
		if ctx.Err() == nil {
			e[2](v, err)
		}
	})
}

func (n *navidrome) nowPlaying(ctx context.Context) (tile.Values, error) {
	r, err := n.get(ctx, "getNowPlaying")
	if err != nil {
		return nil, err
	}
	e := r.Resp.NowPlaying.Entry
	tbl := tile.Value{Head: []string{"Utilisateur", "Morceau", "Album", "Lecteur", "Depuis"}}
	for _, x := range e {
		if len(tbl.Rows) < 15 {
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: x.Username}, {Text: arrPair(x.Artist, x.Title)}, {Text: x.Album}, {Text: x.PlayerName},
				{Text: tile.Dur(time.Duration(x.MinutesAgo) * time.Minute)}})
		}
	}
	v := tile.Values{"playing": {Text: tile.Num(float64(len(e)))}, "now_playing": tbl}
	if r.Resp.ServerVersion != "" {
		v["version"] = tile.Value{Text: r.Resp.ServerVersion, Sub: r.Resp.Type}
	}
	return v, nil
}

func (n *navidrome) scan(ctx context.Context) tile.Values {
	r, err := n.get(ctx, "getScanStatus")
	if err != nil {
		return nil
	}
	s := r.Resp.ScanStatus
	sv := tile.Value{Text: "Terminé", Level: "ok", Sub: "dernier : " + tile.Ago(s.LastScan)}
	if s.LastScan.IsZero() {
		sv.Sub = ""
	}
	if s.Scanning {
		sv = tile.Value{Text: "En cours", Level: "warn", Sub: tile.Num(s.Count) + " fichiers vus"}
	}
	return tile.Values{"scan": sv, "tracks": {Text: tile.Num(s.Count), Sub: tile.Num(s.FolderCount) + " dossiers"}}
}

func (n *navidrome) slow(ctx context.Context) tile.Values {
	r, err := n.get(ctx, "getArtists")
	if err != nil {
		return nil
	}
	artists, albums := 0, 0.0
	for _, i := range r.Resp.Artists.Index {
		artists += len(i.Artist)
		for _, a := range i.Artist {
			albums += a.AlbumCount
		}
	}
	return tile.Values{"artists": {Text: tile.Num(float64(artists))}, "albums": {Text: tile.Num(albums)}}
}
