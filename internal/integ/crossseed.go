package integ

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// cross-seed n'expose en lecture que /api/ping (sans clé) : l'historique des
// recherches et des injections n'est que dans sa base SQLite (pas d'API). La
// tuile ne dit donc que « joignable ou non ». Avec la clé (facultative), elle
// propose de lancer un job en avance (POST /api/job) ; /api/webhook et
// /api/announce ne sont jamais appelés.
type crossSeed struct {
	url, key string
	hc       *http.Client
}

func newCrossSeed(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	c := &crossSeed{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	if _, ok := t.Secrets["key"]; ok {
		var err error
		if c.key, err = secret("key"); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *crossSeed) Actions() []tile.Action {
	if c.key == "" {
		return nil
	}
	return []tile.Action{
		{ID: "rss", Label: "Lancer le RSS"},
		{ID: "search", Label: "Lancer une recherche", Confirm: "Lancer la recherche maintenant ? Elle interroge chaque indexeur (fair use)."},
	}
}

// Do : POST /api/job {"name": "rss" | "search"} ; 409 si le job tourne déjà,
// 404 s'il est désactivé dans la config de cross-seed.
func (c *crossSeed) Do(ctx context.Context, id, _ string) (string, error) {
	if c.key == "" || (id != "rss" && id != "search") {
		return "", errUnknownAction
	}
	err := callAction(ctx, c.hc, http.MethodPost, c.url+"/api/job", map[string]string{"name": id},
		func(r *http.Request) { r.Header.Set("X-Api-Key", c.key) })
	var se statusError
	switch {
	case errors.As(err, &se) && se.Code == http.StatusConflict:
		return "", errors.New("cross-seed : job déjà en cours")
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		return "", errors.New("cross-seed : job désactivé dans sa configuration")
	case err != nil:
		return "", err
	}
	return "Job " + id + " lancé", nil
}

func (c *crossSeed) Fields() []tile.Field {
	return []tile.Field{tile.F("state", "État", tile.Stat, tile.OnTile)}
}

func (c *crossSeed) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		start := time.Now()
		err := c.ping(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			emit(nil, err)
			return
		}
		emit(tile.Values{"state": {Text: "En ligne", Level: "ok", Sub: fmt.Sprintf("%d ms", time.Since(start).Milliseconds())}}, nil)
	})
}

func (c *crossSeed) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		return statusError{resp.StatusCode, c.url + "/api/ping"}
	}
	return nil
}
