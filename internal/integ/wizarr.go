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

// Wizarr (API v1, clé en en-tête X-API-Key ; la clé se crée dans Wizarr, page
// Clés API). Schéma tiré du swagger du conteneur, non éprouvé avec une vraie
// clé. Relevé toutes les minutes. Le code d'une invitation donne accès au
// serveur : il n'est jamais affiché.
type wizarr struct {
	url, key string
	hc       *http.Client
}

func newWizarr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	w := &wizarr{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if w.key, err = secret("key"); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *wizarr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("users", "Utilisateurs", tile.Stat, tile.OnTile),
		tile.F("pending", "Invitations en attente", tile.Stat, tile.OnTile),
		tile.F("expired", "Invitations expirées", tile.Stat, tile.InDetail),
		tile.F("invites", "Invitations", tile.Stat, tile.InDetail),
		tile.F("activity", "Activité (7 j)", tile.Stat, tile.InDetail),
		tile.F("user_list", "Comptes (expiration la plus proche)", tile.Table, tile.InDetail),
		tile.F("invite_list", "Invitations récentes", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (w *wizarr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, w.hc, w.url+path, out, func(r *http.Request) { r.Header.Set("X-API-Key", w.key) })
}

func (w *wizarr) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := w.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// wizarrTime lit les dates de l'API (ISO avec ou sans fuseau) ; zéro si illisible.
func wizarrTime(s string) time.Time {
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02 15:04:05.999999", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func wizarrWhen(s string, past bool) string {
	t := wizarrTime(s)
	switch {
	case s == "":
		return "jamais"
	case t.IsZero():
		return s
	case past:
		return tile.Ago(t)
	case time.Until(t) < 0:
		return "expiré"
	}
	return "dans " + tile.Dur(time.Until(t))
}

func (w *wizarr) poll(ctx context.Context) (tile.Values, error) {
	var st struct{ Users, Invites, Pending, Expired int }
	if err := w.get(ctx, "/status", &st); err != nil {
		return nil, err
	}
	v := tile.Values{
		"users":   {Text: fmt.Sprint(st.Users)},
		"pending": {Text: fmt.Sprint(st.Pending)},
		"expired": {Text: fmt.Sprint(st.Expired)},
		"invites": {Text: fmt.Sprint(st.Invites), Sub: fmt.Sprintf("%d en attente, %d expirées", st.Pending, st.Expired)},
	}

	// Le reste est facultatif : une route en erreur n'efface pas la tuile.
	var ver struct{ Version string }
	if w.get(ctx, "/version", &ver) == nil {
		v["version"] = tile.Value{Text: ver.Version}
	}

	var as struct {
		TotalSessions         int     `json:"total_sessions"`
		UniqueUsers           int     `json:"unique_users"`
		TotalWatchTimeSeconds float64 `json:"total_watch_time_seconds"`
	}
	if w.get(ctx, "/activity/stats?days=7", &as) == nil {
		v["activity"] = tile.Value{Text: fmt.Sprintf("%d sessions", as.TotalSessions), Sub: fmt.Sprintf("%d comptes, %s", as.UniqueUsers, tile.Dur(time.Duration(as.TotalWatchTimeSeconds)*time.Second))}
	}

	var ul struct {
		Users []struct {
			Username, Server, Expires string
			CreatedAt                 string `json:"created_at"`
		}
	}
	if w.get(ctx, "/users", &ul) == nil {
		us := ul.Users
		sort.SliceStable(us, func(i, j int) bool { // expirations d'abord (les plus proches), puis comptes sans expiration
			a, b := wizarrTime(us[i].Expires), wizarrTime(us[j].Expires)
			if a.IsZero() != b.IsZero() {
				return !a.IsZero()
			}
			return a.Before(b)
		})
		var rows [][]tile.Cell
		soon := 0
		for _, u := range us {
			if t := wizarrTime(u.Expires); !t.IsZero() && time.Until(t) < 7*24*time.Hour {
				soon++
			}
			if len(rows) < 15 {
				lvl := ""
				if t := wizarrTime(u.Expires); !t.IsZero() && time.Until(t) < 7*24*time.Hour {
					lvl = "warn"
				}
				exp := "jamais"
				if u.Expires != "" {
					exp = wizarrWhen(u.Expires, false)
				}
				rows = append(rows, []tile.Cell{{Text: u.Username}, {Text: u.Server}, {Text: exp, Level: lvl}, {Text: wizarrWhen(u.CreatedAt, true)}})
			}
		}
		v["user_list"] = tile.Value{Head: []string{"Compte", "Serveur", "Expire", "Créé"}, Rows: rows}
		if soon > 0 {
			u := v["users"]
			u.Sub = fmt.Sprintf("%d expirent sous 7 j", soon)
			v["users"] = u
		}
	}

	var il struct {
		Invitations []struct {
			Status, Created string
			UsedBy          any
			DisplayName     string `json:"display_name"`
		}
	}
	if w.get(ctx, "/invitations", &il) == nil {
		var rows [][]tile.Cell
		for _, i := range il.Invitations {
			if len(rows) == 15 {
				break
			}
			used, _ := i.UsedBy.(string) // l'API peut renvoyer autre chose qu'un nom : on l'ignore
			rows = append(rows, []tile.Cell{{Text: i.DisplayName}, {Text: i.Status}, {Text: wizarrWhen(i.Created, true)}, {Text: used}})
		}
		v["invite_list"] = tile.Value{Head: []string{"Nom", "Statut", "Créée", "Utilisée par"}, Rows: rows}
	}
	return v, nil
}
