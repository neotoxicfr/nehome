package integ

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Tuile « link » : un lien (href) avec, au choix, l'état d'un conteneur
// (options.container, lu sur le socket Docker en lecture) et/ou une sonde HTTP
// (options.health_url). Sans option, la tuile n'est qu'un lien.
// options.post_url ajoute un bouton qui envoie un POST sans corps à cette URL
// (réveil d'un PC, webhook…), libellé options.post_label.
type link struct {
	api       *dockerAPI
	container string
	healthURL string
	postURL   string
	postLabel string
	hc        *http.Client
	protected bool // option protect (« nehome » par défaut) : pas de redémarrage
}

func newLink(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	l := &link{container: t.Options["container"], healthURL: t.Options["health_url"], postURL: t.Options["post_url"], postLabel: t.Options["post_label"]}
	if l.postURL != "" {
		u, err := url.Parse(l.postURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("options.post_url invalide")
		}
		if l.postLabel == "" {
			l.postLabel = "Déclencher"
		}
	}
	if l.container != "" {
		l.api = newDockerAPI(t.Options["docker_url"])
		l.protected = dkProtected(t.Options["protect"])[l.container]
	}
	if l.healthURL != "" {
		u, err := url.Parse(l.healthURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("options.health_url invalide")
		}
		l.hc = httpClient(t.InsecureTLS, false)
		l.hc.Timeout = 10 * time.Second
		// Une redirection vers un autre hôte (Authelia, par exemple) prouve que
		// l'appli répond : on s'arrête sur la réponse 3xx au lieu de la suivre.
		l.hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != via[0].URL.Host || len(via) > 5 {
				return http.ErrUseLastResponse
			}
			return nil
		}
	}
	return l, nil
}

func (l *link) Fields() []tile.Field {
	return []tile.Field{
		tile.F("state", "État", tile.Stat, tile.OnTile),
		tile.F("health", "Santé HTTP", tile.Stat, tile.OnTile),
		tile.F("uptime", "En marche depuis", tile.Stat, tile.OnTile),
		tile.F("image", "Image", tile.Stat, tile.InDetail),
		tile.F("restarts", "Redémarrages", tile.Stat, tile.InDetail),
	}
}

func (l *link) Actions() []tile.Action {
	var a []tile.Action
	if l.postURL != "" {
		a = append(a, tile.Action{ID: "post", Label: l.postLabel, Default: tile.OnTile})
	}
	if l.container != "" && !l.protected {
		a = append(a, tile.Action{ID: "restart", Label: "Redémarrer le conteneur", Confirm: "Redémarrer ce conteneur ?"})
	}
	return a
}

func (l *link) Do(ctx context.Context, id, _ string) (string, error) {
	if id == "post" && l.postURL != "" {
		hc := httpClient(false, false)
		hc.Timeout = 20 * time.Second
		if err := callAction(ctx, hc, http.MethodPost, l.postURL, nil, nil); err != nil {
			return "", err
		}
		return l.postLabel + " : fait", nil
	}
	if id != "restart" || l.container == "" || l.protected {
		return "", fmt.Errorf("action indisponible : %s", id)
	}
	if err := l.api.restart(ctx, l.container); err != nil {
		return "", err
	}
	return l.container + " redémarré", nil
}

func (l *link) Run(ctx context.Context, emit func(tile.Values, error)) {
	if l.container == "" && l.healthURL == "" {
		<-ctx.Done()
		return
	}
	every(ctx, 30*time.Second, func() {
		v, err := l.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (l *link) poll(ctx context.Context) (tile.Values, error) {
	v := tile.Values{}
	var err error
	if l.container != "" {
		in, e := l.api.inspect(ctx, l.container)
		var se statusError
		switch {
		case e == nil:
			txt, lvl := ctrState(in)
			v["state"] = tile.Value{Text: txt, Level: lvl}
			if in.State.Running {
				v["uptime"] = tile.Value{Text: tile.Dur(time.Since(in.State.StartedAt))}
			}
			v["image"] = tile.Value{Text: in.Config.Image}
			rl := ""
			if in.RestartCount >= 3 {
				rl = "warn"
			}
			v["restarts"] = tile.Value{Text: fmt.Sprint(in.RestartCount), Level: rl}
		case asStatus(e, &se) && se.Code == http.StatusNotFound:
			v["state"] = tile.Value{Text: "introuvable", Level: "crit"}
		default:
			err = e
		}
	}
	if l.healthURL != "" {
		v["health"] = l.probe(ctx)
	}
	if err != nil && l.healthURL == "" {
		return nil, err // rien d'autre à afficher
	}
	return v, nil
}

func asStatus(err error, out *statusError) bool {
	se, ok := err.(statusError)
	*out = se
	return ok
}

// probe : un GET, code HTTP et latence. Réseau ou 5xx : crit ; 4xx : warn
// (l'appli répond mais refuse la sonde) ; lenteur > 1,5 s : warn.
func (l *link) probe(ctx context.Context) tile.Value {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.healthURL, nil)
	if err != nil {
		return tile.Value{Text: "URL invalide", Level: "crit"}
	}
	t0 := time.Now()
	resp, err := l.hc.Do(req)
	if err != nil {
		return tile.Value{Text: "injoignable", Sub: cleanErr(err).Error(), Level: "crit"}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	ms := time.Since(t0).Milliseconds()
	lvl := "ok"
	switch {
	case resp.StatusCode >= 500:
		lvl = "crit"
	case resp.StatusCode >= 400, ms > 1500:
		lvl = "warn"
	}
	return tile.Value{Text: fmt.Sprintf("HTTP %d", resp.StatusCode), Sub: fmt.Sprintf("%d ms", ms), Level: lvl}
}
