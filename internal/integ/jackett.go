package integ

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Jackett : connexion par mot de passe admin (cookie « Jackett »), puis liste
// des indexeurs configurés. Le sondage ne teste rien (chaque test interroge un
// tracker, fair use) : seule la dernière erreur connue est lue ; le test n'est
// qu'une action manuelle.
type jackett struct {
	url, pass string
	hc        *http.Client

	mu  sync.Mutex
	ids map[string]string // nom → id des indexeurs, pour l'action « test »
	bad []string          // noms des indexeurs en erreur
}

func newJackett(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	pass, err := secret("pass")
	if err != nil {
		return nil, err
	}
	j := &jackett{url: strings.TrimRight(t.URL, "/"), pass: pass, hc: httpClient(t.InsecureTLS, true)}
	// Session expirée = 302 vers /UI/Login : on la détecte au lieu de la suivre.
	j.hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return j, nil
}

func (j *jackett) Fields() []tile.Field {
	return []tile.Field{
		tile.F("errors", "Indexeurs en erreur", tile.Stat, tile.OnTile),
		tile.F("count", "Indexeurs", tile.Stat, tile.OnTile),
		tile.F("list", "Indexeurs configurés", tile.Table, tile.InDetail),
	}
}

func (j *jackett) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := j.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (j *jackett) login(ctx context.Context) error {
	j.hc.Jar, _ = cookiejar.New(nil) // un cookie périmé ferait passer un échec de login pour un succès
	// Le premier GET pose le cookie de test que le formulaire exige.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, j.url+"/UI/Login", nil)
	resp, err := j.hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	resp.Body.Close()
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, j.url+"/UI/Dashboard", strings.NewReader(url.Values{"password": {j.pass}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resp, err = j.hc.Do(req); err != nil {
		return cleanErr(err)
	}
	resp.Body.Close()
	u, _ := url.Parse(j.url)
	for _, c := range j.hc.Jar.Cookies(u) {
		if c.Name == "Jackett" {
			return nil
		}
	}
	return errors.New("Jackett refuse le mot de passe")
}

type jkIndexer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Language  string `json:"language"`
	LastError string `json:"last_error"`
}

func (j *jackett) poll(ctx context.Context) (tile.Values, error) {
	var ix []jkIndexer
	const path = "/api/v2.0/indexers?configured=true"
	err := getJSON(ctx, j.hc, j.url+path, &ix, nil)
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusFound {
		if err = j.login(ctx); err != nil {
			return nil, err
		}
		err = getJSON(ctx, j.hc, j.url+path, &ix, nil)
	}
	if err != nil {
		return nil, err
	}
	bad := 0
	ids, failed := map[string]string{}, []string{}
	tbl := tile.Value{Head: []string{"Indexeur", "Type", "Langue", "État"}}
	for _, i := range ix {
		ids[i.Name] = i.ID
		st := tile.Cell{Text: "OK", Level: "ok"}
		if i.LastError != "" {
			bad++
			failed = append(failed, i.Name)
			msg := []rune(i.LastError)
			if len(msg) > 80 {
				msg = append(msg[:80], '…')
			}
			st = tile.Cell{Text: string(msg), Level: "crit"}
		}
		tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: i.Name}, {Text: jkType(i.Type)}, {Text: i.Language}, st})
	}
	j.mu.Lock()
	j.ids, j.bad = ids, failed
	j.mu.Unlock()
	ev := tile.Value{Text: tile.Num(float64(bad)), Level: "ok", Sub: "sur " + fmt.Sprint(len(ix))}
	if bad > 0 {
		ev.Level = "crit"
	}
	return tile.Values{"errors": ev, "count": {Text: tile.Num(float64(len(ix)))}, "list": tbl}, nil
}

func jkType(t string) string {
	switch t {
	case "private":
		return "privé"
	case "semi-private":
		return "semi-privé"
	case "public":
		return "public"
	}
	return t
}

func (j *jackett) Actions() []tile.Action {
	j.mu.Lock()
	defer j.mu.Unlock()
	var a []tile.Action
	if len(j.bad) > 0 {
		a = append(a, tile.Action{ID: "testbad", Label: "Retester les indexeurs en erreur"})
	}
	if len(j.ids) > 0 {
		names := make([]string, 0, len(j.ids))
		for n := range j.ids {
			names = append(names, n)
		}
		slices.Sort(names)
		a = append(a, tile.Action{ID: "test", Label: "Tester un indexeur", Choices: names})
	}
	return a
}

// Do : POST /api/v2.0/indexers/{id}/test (bouton « Test » de Jackett), une
// requête par indexeur, l'un après l'autre.
func (j *jackett) Do(ctx context.Context, id, arg string) (string, error) {
	j.mu.Lock()
	var names []string
	switch id {
	case "testbad":
		names = slices.Clone(j.bad)
	case "test":
		if _, ok := j.ids[arg]; ok {
			names = []string{arg}
		}
	}
	ids := make([]string, len(names))
	for k, n := range names {
		ids[k] = j.ids[n]
	}
	j.mu.Unlock()
	if len(names) == 0 {
		return "", errUnknownAction
	}
	var ko []string
	for k, ix := range ids {
		if err := j.test(ctx, ix); err != nil {
			ko = append(ko, names[k])
		}
	}
	if len(ko) > 0 {
		return "", fmt.Errorf("toujours en erreur : %s", strings.Join(ko, ", "))
	}
	return fmt.Sprintf("%d indexeur(s) OK", len(ids)), nil
}

func (j *jackett) test(ctx context.Context, id string) error {
	u := j.url + "/api/v2.0/indexers/" + url.PathEscape(id) + "/test"
	err := callAction(ctx, j.hc, http.MethodPost, u, nil, nil)
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusFound {
		if err = j.login(ctx); err != nil {
			return err
		}
		err = callAction(ctx, j.hc, http.MethodPost, u, nil, nil)
	}
	return err
}
