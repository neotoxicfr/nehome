package integ

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// prometheus : (a) requêtes PromQL instantanées sur un serveur Prometheus
// (/api/v1/query) ; (b) avec scrape: true, lecture directe d'un endpoint
// /metrics au format texte. Formats et seuils : ceux de customapi.
type prometheus struct {
	genBase
	base   string
	scrape bool
	sels   []promSel // mode scrape, dans l'ordre de specs
}

type promSel struct {
	name   string
	labels map[string]string
}

func newPrometheus(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g, err := newGenBase(t, secret, false)
	if err != nil {
		return nil, err
	}
	p := &prometheus{genBase: *g, base: strings.TrimRight(t.URL, "/")}
	if s := t.Options["scrape"]; s != "" {
		if p.scrape, err = strconv.ParseBool(s); err != nil {
			return nil, fmt.Errorf("options.scrape : %q n'est pas un booléen", s)
		}
	}
	if p.scrape {
		for _, f := range p.specs {
			sel, err := promSelector(f.Path)
			if err != nil {
				return nil, fmt.Errorf("fields.%s : %v", f.ID, err)
			}
			p.sels = append(p.sels, sel)
		}
	}
	return p, nil
}

func (p *prometheus) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, p.interval, func() {
		var v tile.Values
		var err error
		if p.scrape {
			v, err = p.pollScrape(ctx)
		} else {
			v, err = p.pollQuery(ctx)
		}
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

// promQueryError : la requête a été comprise mais n'a pas de réponse utilisable ;
// elle ne concerne que son champ.
type promQueryError struct{ msg string }

func (e promQueryError) Error() string { return e.msg }

func (p *prometheus) pollQuery(ctx context.Context) (tile.Values, error) {
	v := tile.Values{}
	var down error
	failed := 0
	for _, f := range p.specs {
		x, err := p.query(ctx, f.Path)
		if err != nil {
			if !errors.As(err, new(promQueryError)) {
				down = err
				failed++
			}
			v[f.ID] = genErr(err)
			continue
		}
		v[f.ID] = genValue(f, x)
	}
	// Toutes les requêtes en échec réseau ou HTTP : le serveur est en panne.
	if failed == len(p.specs) {
		return nil, down
	}
	return v, nil
}

// query renvoie la valeur (chaîne) du premier résultat d'une requête
// instantanée : scalaire, chaîne ou premier élément d'un vecteur.
func (p *prometheus) query(ctx context.Context, q string) (any, error) {
	u := p.base + "/api/v1/query?query=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	p.set(req)
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	var r struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string          `json:"resultType"`
			Result     json.RawMessage `json:"result"`
		} `json:"data"`
	}
	// Une requête invalide répond 400/422 avec un corps JSON explicite.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&r); err != nil || r.Status == "" {
		if resp.StatusCode != http.StatusOK {
			return nil, statusError{resp.StatusCode, redact(u)}
		}
		return nil, fmt.Errorf("%s : réponse non conforme à l'API Prometheus", redact(u))
	}
	if r.Status != "success" {
		return nil, promQueryError{"Prometheus : " + genClip(r.Error, 160)}
	}
	var pair []any
	switch r.Data.ResultType {
	case "scalar", "string":
		if json.Unmarshal(r.Data.Result, &pair) != nil {
			return nil, promQueryError{"résultat illisible"}
		}
	case "vector":
		var vec []struct {
			Value []any `json:"value"`
		}
		if json.Unmarshal(r.Data.Result, &vec) != nil {
			return nil, promQueryError{"résultat illisible"}
		}
		if len(vec) == 0 {
			return nil, promQueryError{"aucun résultat"}
		}
		pair = vec[0].Value
	default:
		return nil, promQueryError{fmt.Sprintf("résultat %q non géré : requête instantanée attendue", r.Data.ResultType)}
	}
	if len(pair) != 2 {
		return nil, promQueryError{"résultat illisible"}
	}
	return pair[1], nil
}

func (p *prometheus) pollScrape(ctx context.Context) (tile.Values, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base, nil)
	if err != nil {
		return nil, err
	}
	p.set(req)
	// Format texte explicite : certains clients servent du protobuf sinon.
	req.Header.Set("Accept", "text/plain;version=0.0.4;q=0.9,*/*;q=0.1")
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, statusError{resp.StatusCode, redact(p.base)}
	}
	want := map[string]bool{}
	for _, s := range p.sels {
		want[s.name] = true
	}
	sum := make([]float64, len(p.sels))
	hits := make([]int, len(p.sels))
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 32<<20))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		name, labels, val, ok := promSample(sc.Text(), want)
		if !ok {
			continue
		}
		for i, s := range p.sels {
			if s.name == name && promMatch(s.labels, labels) {
				sum[i] += val
				hits[i]++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s : lecture interrompue (%v)", redact(p.base), err)
	}
	v := tile.Values{}
	for i, f := range p.specs {
		if hits[i] == 0 {
			v[f.ID] = genErr(fmt.Errorf("série %s introuvable", genClip(f.Path, 80)))
			continue
		}
		v[f.ID] = genValue(f, sum[i])
	}
	return v, nil
}

var promNameRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// parseSelector lit « nom » ou « nom{label="v",autre="w"} » (égalité seule).
func promSelector(s string) (promSel, error) {
	s = strings.TrimSpace(s)
	name, rest, hasLabels := strings.Cut(s, "{")
	name = strings.TrimSpace(name)
	if !promNameRe.MatchString(name) {
		return promSel{}, fmt.Errorf("nom de série %q invalide", name)
	}
	sel := promSel{name: name}
	if !hasLabels {
		return sel, nil
	}
	labels, n, err := promLabels(rest)
	if err != nil {
		return promSel{}, fmt.Errorf("sélecteur %q : %v", s, err)
	}
	if strings.TrimSpace(rest[n:]) != "" {
		return promSel{}, fmt.Errorf("sélecteur %q : texte après }", s)
	}
	sel.labels = labels
	return sel, nil
}

// parseLabels lit « a="x",b="y"} » (accolade ouvrante déjà consommée) et
// renvoie la position qui suit l'accolade fermante.
func promLabels(s string) (map[string]string, int, error) {
	out := map[string]string{}
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil, 0, errors.New("accolade fermante manquante")
		}
		if s[i] == '}' {
			return out, i + 1, nil
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			return nil, 0, errors.New("= manquant")
		}
		key := strings.TrimSpace(s[i : i+eq])
		if key == "" || strings.ContainsAny(key, `!~"`) {
			return nil, 0, fmt.Errorf("étiquette %q invalide (égalité seule)", key)
		}
		i += eq + 1
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i >= len(s) || s[i] != '"' {
			return nil, 0, errors.New("valeur d'étiquette sans guillemets")
		}
		i++
		var b strings.Builder
		closed := false
		for i < len(s) {
			c := s[i]
			i++
			if c == '\\' && i < len(s) {
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				default:
					b.WriteByte(s[i])
				}
				i++
				continue
			}
			if c == '"' {
				closed = true
				break
			}
			b.WriteByte(c)
		}
		if !closed {
			return nil, 0, errors.New("guillemet fermant manquant")
		}
		out[key] = b.String()
	}
}

// parseSample lit une ligne d'exposition « nom{étiquettes} valeur [horodatage] »
// si son nom est demandé.
func promSample(line string, want map[string]bool) (string, map[string]string, float64, bool) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '#' {
		return "", nil, 0, false
	}
	end := strings.IndexAny(line, "{ \t")
	if end < 0 {
		return "", nil, 0, false
	}
	name := line[:end]
	if !want[name] {
		return "", nil, 0, false
	}
	rest := line[end:]
	var labels map[string]string
	if rest[0] == '{' {
		l, n, err := promLabels(rest[1:])
		if err != nil {
			return "", nil, 0, false
		}
		labels, rest = l, rest[1+n:]
	}
	f := strings.Fields(rest)
	if len(f) == 0 {
		return "", nil, 0, false
	}
	val, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return "", nil, 0, false
	}
	return name, labels, val, true
}

func promMatch(want, got map[string]string) bool {
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
