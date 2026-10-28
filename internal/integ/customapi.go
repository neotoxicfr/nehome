package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// customapi : tuile générique sur n'importe quelle API JSON, lue en GET. Les
// champs sont déclarés dans config.yaml (fields:). Ce fichier porte aussi ce
// qui est commun avec prometheus.go : options, authentification, mise en forme.
type customAPI struct {
	genBase
	url string
	// Bouton facultatif : POST sans corps sur options.post_url, avec le secret
	// post_key en Bearer (clé distincte de celle de lecture).
	postURL, postLabel, postKey string
}

// genBase : réglages partagés par customapi et prometheus.
type genBase struct {
	hc       *http.Client
	set      func(*http.Request)
	interval time.Duration
	specs    []config.FieldMap
	fields   []tile.Field
}

func newCustomAPI(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	g, err := newGenBase(t, secret, true)
	if err != nil {
		return nil, err
	}
	c := &customAPI{genBase: *g, url: t.URL, postURL: t.Options["post_url"], postLabel: t.Options["post_label"]}
	if c.postURL != "" {
		// Même schéma et même hôte:port que l'URL lue : la clé post_key (et le
		// contrôle de changement de destination fait sur l'URL dans les
		// réglages) ne peut pas partir ailleurs.
		u, err := url.Parse(c.postURL)
		base, berr := url.Parse(t.URL)
		if err != nil || berr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("options.post_url invalide")
		}
		if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil {
			return nil, errors.New("options.post_url doit viser le même hôte que url")
		}
		if c.postLabel == "" {
			c.postLabel = "Déclencher"
		}
		if _, ok := t.Secrets["post_key"]; ok {
			if c.postKey, err = secret("post_key"); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

func (c *customAPI) Actions() []tile.Action {
	if c.postURL == "" {
		return nil
	}
	return []tile.Action{{ID: "post", Label: c.postLabel}}
}

func (c *customAPI) Do(ctx context.Context, id, _ string) (string, error) {
	if id != "post" || c.postURL == "" {
		return "", errUnknownAction
	}
	err := callAction(ctx, c.hc, http.MethodPost, c.postURL, nil, func(r *http.Request) {
		if c.postKey != "" {
			r.Header.Set("Authorization", "Bearer "+c.postKey)
		}
	})
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusTooManyRequests {
		return "", errors.New("refusé : trop tôt après le précédent, réessaie dans quelques minutes")
	}
	if err != nil {
		return "", err
	}
	return c.postLabel + " : fait", nil
}

// Fields dépend de la configuration : nil sur la valeur zéro.
func (g *genBase) Fields() []tile.Field { return g.fields }

var genFormats = map[string]bool{"text": true, "number": true, "bytes": true, "rate": true, "percent": true, "duration": true, "date": true}

// newGenBase valide la tuile : url, méthode, intervalle, en-têtes, secrets
// facultatifs et champs déclarés. emptyPath autorise un chemin vide (racine).
func newGenBase(t config.Tile, secret func(string) (string, error), emptyPath bool) (*genBase, error) {
	if strings.TrimSpace(t.URL) == "" {
		return nil, errors.New("url manquante")
	}
	if m := t.Options["method"]; m != "" && !strings.EqualFold(m, http.MethodGet) {
		return nil, fmt.Errorf("options.method : %q refusé, seul GET est permis (lecture seule)", m)
	}
	g := &genBase{hc: httpClient(t.InsecureTLS, false), interval: time.Minute}
	if s := t.Options["interval"]; s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("options.interval : %q n'est pas une durée (ex. 30s, 5m)", s)
		}
		if d < 5*time.Second {
			return nil, fmt.Errorf("options.interval : %s trop court (5s minimum)", s)
		}
		g.interval = d
	}

	headers := map[string]string{}
	for k, v := range t.Options {
		if name, ok := strings.CutPrefix(k, "header."); ok {
			if name == "" {
				return nil, errors.New("options : nom d'en-tête vide (header.<Nom>)")
			}
			headers[name] = v
		}
	}
	var token, key, user, pass string
	keyHeader := t.Options["key_header"]
	if keyHeader == "" {
		keyHeader = "X-Api-Key"
	}
	for _, r := range []struct {
		role string
		dst  *string
	}{{"token", &token}, {"key", &key}, {"user", &user}, {"pass", &pass}} {
		if _, ok := t.Secrets[r.role]; !ok {
			continue
		}
		v, err := secret(r.role)
		if err != nil {
			return nil, err
		}
		*r.dst = v
	}
	_, hasUser := t.Secrets["user"]
	_, hasPass := t.Secrets["pass"]
	if hasUser != hasPass {
		return nil, errors.New("secrets : user et pass vont ensemble (authentification basique)")
	}
	g.set = func(r *http.Request) {
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			r.Header.Set(keyHeader, key)
		}
		if hasUser {
			r.SetBasicAuth(user, pass)
		}
	}

	if len(t.Fields) == 0 {
		return nil, errors.New("aucun champ déclaré (fields:)")
	}
	seen := map[string]bool{}
	for i, f := range t.Fields {
		if f.ID == "" {
			return nil, fmt.Errorf("fields[%d] : id manquant", i)
		}
		if seen[f.ID] {
			return nil, fmt.Errorf("fields : id %q en double", f.ID)
		}
		seen[f.ID] = true
		if f.Format == "" {
			f.Format = "text"
		}
		if !genFormats[f.Format] {
			return nil, fmt.Errorf("fields.%s : format %q inconnu (text, number, bytes, rate, percent, duration, date)", f.ID, f.Format)
		}
		if f.Path == "" && !emptyPath {
			return nil, fmt.Errorf("fields.%s : path manquant", f.ID)
		}
		place := tile.OnTile
		switch f.Placement {
		case "", "tile":
		case "detail":
			place = tile.InDetail
		case "hidden":
			place = tile.Hidden
		default:
			return nil, fmt.Errorf("fields.%s : placement %q inconnu (tile, detail)", f.ID, f.Placement)
		}
		if f.Label == "" {
			f.Label = f.ID
		}
		kind := tile.Stat
		if f.Format == "percent" {
			kind = tile.Meter
		}
		g.specs = append(g.specs, f)
		g.fields = append(g.fields, tile.F(f.ID, f.Label, kind, place))
	}
	return g, nil
}

func (c *customAPI) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, c.interval, func() {
		v, err := c.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (c *customAPI) poll(ctx context.Context) (tile.Values, error) {
	var raw json.RawMessage
	if err := getJSON(ctx, c.hc, c.url, &raw, c.set); err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%s : réponse non JSON", redact(c.url))
		}
		return nil, err
	}
	// UseNumber garde les grands entiers intacts (identifiants, compteurs).
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s : réponse non JSON", redact(c.url))
	}
	v := tile.Values{}
	for _, f := range c.specs {
		x, err := genLookup(doc, f.Path)
		if err != nil {
			v[f.ID] = genErr(err)
			continue
		}
		v[f.ID] = genValue(f, x)
	}
	return v, nil
}

// lookup suit un chemin pointé : « data.items.0.count », « list.-1 » (dernier
// élément), « data.items.# » (longueur d'un tableau ou d'un objet).
func genLookup(doc any, path string) (any, error) {
	if path == "" || path == "." {
		return doc, nil
	}
	parts := strings.Split(path, ".")
	cur := doc
	for i, p := range parts {
		at := strings.Join(parts[:i], ".")
		if at == "" {
			at = "la racine"
		}
		if p == "#" {
			if i != len(parts)-1 {
				return nil, errors.New("# doit terminer le chemin")
			}
			switch x := cur.(type) {
			case []any:
				return json.Number(strconv.Itoa(len(x))), nil
			case map[string]any:
				return json.Number(strconv.Itoa(len(x))), nil
			case nil:
				return json.Number("0"), nil // null tient lieu de liste vide
			}
			return nil, fmt.Errorf("%s n'est ni un tableau ni un objet", at)
		}
		switch x := cur.(type) {
		case map[string]any:
			v, ok := x[p]
			if !ok {
				return nil, fmt.Errorf("clé %q absente de %s", p, at)
			}
			cur = v
		case []any:
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("%s est un tableau : %q n'est pas un indice", at, p)
			}
			if n < 0 {
				n += len(x)
			}
			if n < 0 || n >= len(x) {
				return nil, fmt.Errorf("indice %s hors de %s (%d éléments)", p, at, len(x))
			}
			cur = x[n]
		case nil:
			return nil, fmt.Errorf("%s vaut null", at)
		default:
			return nil, fmt.Errorf("%s n'est pas un objet", at)
		}
	}
	return cur, nil
}

// fieldErr : un champ en échec s'affiche sans casser les autres.
func genErr(err error) tile.Value {
	return tile.Value{Text: "—", Sub: err.Error(), Level: "warn"}
}

// toFloat accepte nombres JSON, nombres en chaîne et booléens (1/0).
func genFloat(x any) (float64, bool) {
	var f float64
	var err error
	switch v := x.(type) {
	case json.Number:
		f, err = v.Float64()
	case float64:
		f = v
	case string:
		f, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	case bool:
		if v {
			f = 1
		}
	default:
		return 0, false
	}
	return f, err == nil
}

// textOf : rendu brut d'une valeur pour le format text.
func genText(x any) string {
	switch v := x.(type) {
	case string:
		return genClip(v, 200)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "oui"
		}
		return "non"
	}
	b, _ := json.Marshal(x)
	return genClip(string(b), 200)
}

func genClip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// genNum : entier groupé par milliers (espace fine insécable) à partir de
// 100, une décimale à la française en dessous (comme tile.Num).
func genNum(f float64) string {
	if f != math.Trunc(f) && math.Abs(f) < 100 || math.Abs(f) >= 1e18 {
		return tile.Num(f)
	}
	s := strconv.FormatInt(int64(math.Abs(math.Round(f))), 10)
	var b strings.Builder
	if f < 0 && s != "0" {
		b.WriteByte('-')
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString("\u202f")
		}
		b.WriteRune(c)
	}
	return b.String()
}

// fieldLevel applique les seuils ; warn > crit inverse le sens (plus bas = pire).
func genLevel(f config.FieldMap, n float64) string {
	if f.Warn == nil && f.Crit == nil {
		return ""
	}
	w, c := math.Inf(1), math.Inf(1)
	if f.Crit != nil {
		c = *f.Crit
	}
	if f.Warn != nil {
		w = *f.Warn
	} else {
		w = c
	}
	if f.Warn != nil && f.Crit != nil && w > c {
		return tile.Level(-n, -w, -c)
	}
	return tile.Level(n, w, c)
}

// fmtValue met en forme une valeur brute selon le format du champ.
func genValue(f config.FieldMap, x any) tile.Value {
	if x == nil {
		return tile.Value{Text: "—", Sub: "valeur nulle"}
	}
	format := f.Format
	if format == "" {
		format = "text"
	}
	if format == "date" {
		return genDate(f, x)
	}
	n, isNum := genFloat(x)
	if isNum && (math.IsNaN(n) || math.IsInf(n, 0)) {
		if format == "text" {
			return tile.Value{Text: genText(x)}
		}
		return genErr(errors.New("valeur non finie"))
	}
	if format == "text" {
		v := tile.Value{Text: genText(x)}
		if _, isStr := x.(string); isNum && !isStr {
			v.Level = genLevel(f, n)
		}
		return v
	}
	if !isNum {
		return genErr(fmt.Errorf("valeur non numérique : %s", genClip(genText(x), 60)))
	}
	v := tile.Value{Level: genLevel(f, n)}
	switch format {
	case "number":
		v.Text = genNum(n)
	case "bytes":
		v.Text = tile.Bytes(n)
	case "rate":
		v.Text = tile.Rate(n)
	case "percent":
		v.Text = tile.Pct(n)
		v.Pct = math.Max(0, math.Min(100, n))
	case "duration":
		d := time.Duration(math.Abs(n) * float64(time.Second))
		v.Text = tile.Dur(d)
		if n < 0 {
			v.Text = "-" + v.Text
		}
	}
	return v
}

// fmtDate lit RFC 3339 (ou date seule) ou une époque Unix (s, ms, µs, ns
// selon l'ordre de grandeur). Les seuils portent sur l'âge en secondes.
func genDate(f config.FieldMap, x any) tile.Value {
	var t time.Time
	if s, ok := x.(string); ok {
		s = strings.TrimSpace(s)
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
			if p, err := time.ParseInLocation(layout, s, time.Local); err == nil {
				t = p
				break
			}
		}
	}
	if t.IsZero() {
		n, ok := genFloat(x)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return genErr(fmt.Errorf("date illisible : %s", genClip(genText(x), 60)))
		}
		if n == 0 {
			return tile.Value{Text: "—", Sub: "date vide"}
		}
		switch a := math.Abs(n); {
		case a >= 1e17:
			t = time.Unix(0, int64(n))
		case a >= 1e14:
			t = time.UnixMicro(int64(n))
		case a >= 1e11:
			t = time.UnixMilli(int64(n))
		default:
			sec, frac := math.Modf(n)
			t = time.Unix(int64(sec), int64(frac*1e9))
		}
	}
	v := tile.Value{Text: t.Local().Format("02/01/2006 15:04")}
	age := time.Since(t)
	v.Sub = tile.Ago(t)
	if age < 0 {
		v.Sub = "dans " + tile.Dur(-age)
	}
	v.Level = genLevel(f, age.Seconds())
	return v
}
