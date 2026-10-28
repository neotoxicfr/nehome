package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Gitea et Forgejo (même API /api/v1). Jeton d'accès personnel (rôle token),
// en-tête « Authorization: token … ». Les totaux viennent de l'en-tête
// X-Total-Count. Utilisateurs et organisations passent par /admin : sans jeton
// administrateur, ces deux champs restent simplement absents. Relevé toutes
// les minutes.
type gitea struct {
	url, token string
	hc         *http.Client
}

func newGitea(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g := &gitea{url: strings.TrimRight(t.URL, "/") + "/api/v1", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if g.token, err = secret("token"); err != nil {
		return nil, err
	}
	return g, nil
}

// Forgejo expose la même API : même source, autre nom de type.
func newForgejo(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	return newGitea(t, secret)
}

func (g *gitea) Fields() []tile.Field {
	return []tile.Field{
		tile.F("repos", "Dépôts", tile.Stat, tile.OnTile),
		tile.F("notifications", "Notifications non lues", tile.Stat, tile.OnTile),
		tile.F("users", "Utilisateurs", tile.Stat, tile.OnTile),
		tile.F("orgs", "Organisations", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("recent", "Dépôts récemment modifiés", tile.Table, tile.InDetail),
	}
}

func (g *gitea) set(r *http.Request) { r.Header.Set("Authorization", "token "+g.token) }

// total renvoie X-Total-Count d'une liste (limit=1 : on ne veut que le compte).
func (g *gitea) total(ctx context.Context, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url+path, nil)
	if err != nil {
		return 0, err
	}
	g.set(req)
	resp, err := g.hc.Do(req)
	if err != nil {
		return 0, cleanErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, statusError{resp.StatusCode, redact(g.url + path)}
	}
	return strconv.Atoi(resp.Header.Get("X-Total-Count"))
}

func (g *gitea) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := g.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (g *gitea) poll(ctx context.Context) (tile.Values, error) {
	var repos struct {
		Data []struct {
			FullName   string    `json:"full_name"`
			Stars      int       `json:"stars_count"`
			OpenIssues int       `json:"open_issues_count"`
			Updated    time.Time `json:"updated_at"`
			Private    bool      `json:"private"`
		}
	}
	// Requête principale : liste triée, le total est dans l'en-tête.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url+"/repos/search?sort=updated&order=desc&limit=10", nil)
	if err != nil {
		return nil, err
	}
	g.set(req)
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError{resp.StatusCode, redact(req.URL.String())}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&repos); err != nil {
		return nil, err
	}
	n, _ := strconv.Atoi(resp.Header.Get("X-Total-Count"))
	if n == 0 {
		n = len(repos.Data)
	}
	v := tile.Values{"repos": {Text: fmt.Sprint(n)}}
	var rows [][]tile.Cell
	for _, r := range repos.Data {
		name := r.FullName
		if r.Private {
			name += " (privé)"
		}
		rows = append(rows, []tile.Cell{{Text: name}, {Text: fmt.Sprintf("%d", r.Stars)}, {Text: fmt.Sprintf("%d", r.OpenIssues)}, {Text: tile.Ago(r.Updated)}})
	}
	v["recent"] = tile.Value{Head: []string{"Dépôt", "Étoiles", "Tickets ouverts", "Modifié"}, Rows: rows}

	// Le reste est facultatif : une route en erreur (droits) n'efface rien.
	var ver struct{ Version string }
	if getJSON(ctx, g.hc, g.url+"/version", &ver, g.set) == nil && ver.Version != "" {
		v["version"] = tile.Value{Text: ver.Version}
	}
	var nt struct{ New int }
	if getJSON(ctx, g.hc, g.url+"/notifications/new", &nt, g.set) == nil {
		lvl := ""
		if nt.New > 0 {
			lvl = "warn"
		}
		v["notifications"] = tile.Value{Text: fmt.Sprint(nt.New), Level: lvl}
	}
	if c, err := g.total(ctx, "/admin/users?limit=1"); err == nil {
		v["users"] = tile.Value{Text: fmt.Sprint(c)}
	}
	if c, err := g.total(ctx, "/admin/orgs?limit=1"); err == nil {
		v["orgs"] = tile.Value{Text: fmt.Sprint(c)}
	}
	return v, nil
}
