package integ

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// dlPost envoie un corps JSON en POST et décode la réponse. Les en-têtes
// sont rendus même en cas d'erreur HTTP (Transmission y renvoie son jeton de
// session sur un 409).
func dlPost(ctx context.Context, c *http.Client, url string, body, out any, set func(*http.Request)) (http.Header, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if set != nil {
		set(req)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return resp.Header, statusError{resp.StatusCode, redact(url)}
	}
	return resp.Header, json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// Transmission : RPC JSON sur /transmission/rpc. Le jeton X-Transmission-Session-Id
// est exigé à chaque appel ; le démon le renvoie dans un 409 quand il manque ou expire.
type transmission struct {
	url, user, pass string
	hc              *http.Client
	sid             string

	down, up *tile.Ring
	slowAt   time.Time
	slow     tile.Values // champs rafraîchis lentement (version, limites, espace libre)
}

type trTorrent struct {
	Name         string  `json:"name"`
	Status       int     `json:"status"`
	PercentDone  float64 `json:"percentDone"`
	RateDownload int64   `json:"rateDownload"`
	RateUpload   int64   `json:"rateUpload"`
	ETA          int64   `json:"eta"`
	TotalSize    int64   `json:"totalSize"`
	Error        int     `json:"error"`
	UploadRatio  float64 `json:"uploadRatio"`
}

type trStats struct {
	Active     int     `json:"activeTorrentCount"`
	Paused     int     `json:"pausedTorrentCount"`
	Total      int     `json:"torrentCount"`
	Down       float64 `json:"downloadSpeed"`
	Up         float64 `json:"uploadSpeed"`
	Cumulative struct {
		Down float64 `json:"downloadedBytes"`
		Up   float64 `json:"uploadedBytes"`
	} `json:"cumulative-stats"`
}

func newTransmission(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	u := strings.TrimRight(t.URL, "/")
	if !strings.HasSuffix(u, "/rpc") {
		u += "/transmission/rpc"
	}
	s := &transmission{url: u, hc: httpClient(t.InsecureTLS, false), down: tile.NewRing(60), up: tile.NewRing(60)}
	// Authentification facultative (rpc-authentication-required).
	if _, ok := t.Secrets["user"]; ok {
		var err error
		if s.user, err = secret("user"); err != nil {
			return nil, err
		}
		if s.pass, err = secret("pass"); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *transmission) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("up", "Envoi", tile.Spark, tile.OnTile),
		tile.F("leech", "En téléchargement", tile.Stat, tile.OnTile),
		tile.F("seed", "En partage", tile.Stat, tile.OnTile),
		tile.F("ratio", "Ratio global", tile.Stat, tile.InDetail),
		tile.F("alt", "Limite alternative", tile.Stat, tile.InDetail),
		tile.F("limits", "Limites", tile.Stat, tile.InDetail),
		tile.F("free", "Espace libre", tile.Stat, tile.InDetail),
		tile.F("total", "Total depuis l'origine", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("errors", "En erreur", tile.Stat, tile.InDetail),
		tile.F("checking", "En vérification", tile.Stat, tile.InDetail),
		tile.F("dl", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("topup", "Envois les plus actifs", tile.Table, tile.InDetail),
	}
}

func (s *transmission) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 3*time.Second, func() {
		v, err := s.poll(ctx)
		if err != nil {
			if ctx.Err() == nil {
				emit(nil, err)
			}
			return
		}
		emit(v, nil)
	})
}

// call appelle une méthode RPC ; rejoue une fois avec le nouveau jeton sur un 409.
func (s *transmission) call(ctx context.Context, method string, args map[string]any, out any) error {
	body := map[string]any{"method": method}
	if args != nil {
		body["arguments"] = args
	}
	var env struct {
		Result    string          `json:"result"`
		Arguments json.RawMessage `json:"arguments"`
	}
	for try := 0; ; try++ {
		hdr, err := dlPost(ctx, s.hc, s.url, body, &env, func(r *http.Request) {
			if s.sid != "" {
				r.Header.Set("X-Transmission-Session-Id", s.sid)
			}
			if s.user != "" {
				r.SetBasicAuth(s.user, s.pass)
			}
		})
		var se statusError
		if errors.As(err, &se) && se.Code == http.StatusConflict && try == 0 && hdr.Get("X-Transmission-Session-Id") != "" {
			s.sid = hdr.Get("X-Transmission-Session-Id")
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	if env.Result != "success" {
		return fmt.Errorf("Transmission : %s", env.Result)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Arguments, out)
}

// refreshSlow : version, limites et espace libre bougent peu (toutes les minutes).
// Un échec garde les anciennes valeurs.
func (s *transmission) refreshSlow(ctx context.Context) {
	if time.Since(s.slowAt) < time.Minute {
		return
	}
	s.slowAt = time.Now()
	var g struct {
		Version  string  `json:"version"`
		Dir      string  `json:"download-dir"`
		Alt      bool    `json:"alt-speed-enabled"`
		LimDown  bool    `json:"speed-limit-down-enabled"`
		LimUp    bool    `json:"speed-limit-up-enabled"`
		DownKBps float64 `json:"speed-limit-down"`
		UpKBps   float64 `json:"speed-limit-up"`
		AltDown  float64 `json:"alt-speed-down"`
		AltUp    float64 `json:"alt-speed-up"`
	}
	if err := s.call(ctx, "session-get", nil, &g); err != nil {
		return
	}
	v := tile.Values{}
	if g.Version != "" {
		v["version"] = tile.Value{Text: g.Version}
	}
	lim := func(on bool, kb float64) string {
		if !on {
			return "∞"
		}
		return tile.Rate(kb * 1000) // Transmission compte en ko/s décimaux
	}
	altTxt, altLvl := "non", ""
	if g.Alt {
		altTxt, altLvl = "oui", "warn"
		v["limits"] = tile.Value{Text: "↓ " + tile.Rate(g.AltDown*1000) + " · ↑ " + tile.Rate(g.AltUp*1000)}
	} else {
		v["limits"] = tile.Value{Text: "↓ " + lim(g.LimDown, g.DownKBps) + " · ↑ " + lim(g.LimUp, g.UpKBps)}
	}
	v["alt"] = tile.Value{Text: altTxt, Level: altLvl}
	if g.Dir != "" {
		var fs struct {
			Free int64 `json:"size-bytes"`
		}
		if s.call(ctx, "free-space", map[string]any{"path": g.Dir}, &fs) == nil && fs.Free >= 0 {
			v["free"] = tile.Value{Text: tile.Bytes(float64(fs.Free))}
		}
	}
	s.slow = v
}

func (s *transmission) poll(ctx context.Context) (tile.Values, error) {
	var st trStats
	if err := s.call(ctx, "session-stats", nil, &st); err != nil {
		return nil, err
	}
	var tl struct {
		Torrents []trTorrent `json:"torrents"`
	}
	fields := []string{"name", "status", "percentDone", "rateDownload", "rateUpload", "eta", "totalSize", "error", "uploadRatio"}
	if err := s.call(ctx, "torrent-get", map[string]any{"fields": fields}, &tl); err != nil {
		return nil, err
	}
	s.refreshSlow(ctx)

	var leech, seed, checking, errs int
	var dl, up []trTorrent
	for _, t := range tl.Torrents {
		switch t.Status {
		case 3, 4: // 3 = en file pour le téléchargement, 4 = en cours
			leech++
			dl = append(dl, t)
		case 5, 6:
			seed++
		case 1, 2:
			checking++
		}
		if t.Error != 0 {
			errs++
		}
		if t.RateUpload > 0 {
			up = append(up, t)
		}
	}
	slices.SortFunc(dl, func(a, b trTorrent) int { return cmp.Compare(b.PercentDone, a.PercentDone) })
	var dlRows [][]tile.Cell
	for _, t := range dl[:min(15, len(dl))] {
		eta := "∞"
		if t.ETA > 0 && t.ETA < 8640000 {
			eta = tile.Dur(time.Duration(t.ETA) * time.Second)
		}
		dlRows = append(dlRows, []tile.Cell{{Text: t.Name}, {Text: tile.Pct(t.PercentDone * 100)}, {Text: tile.Rate(float64(t.RateDownload))}, {Text: eta}})
	}
	slices.SortFunc(up, func(a, b trTorrent) int { return cmp.Compare(b.RateUpload, a.RateUpload) })
	var upRows [][]tile.Cell
	for _, t := range up[:min(15, len(up))] {
		upRows = append(upRows, []tile.Cell{{Text: t.Name}, {Text: tile.Num(t.UploadRatio)}, {Text: tile.Rate(float64(t.RateUpload))}})
	}
	errLvl := ""
	if errs > 0 {
		errLvl = "warn"
	}
	v := tile.Values{
		"down":     {Text: tile.Rate(st.Down), Points: s.down.Push(st.Down)},
		"up":       {Text: tile.Rate(st.Up), Points: s.up.Push(st.Up)},
		"leech":    {Text: fmt.Sprint(leech), Sub: fmt.Sprintf("%d en pause", st.Paused)},
		"seed":     {Text: fmt.Sprint(seed), Sub: fmt.Sprintf("%d au total", len(tl.Torrents))},
		"total":    {Text: "↓ " + tile.Bytes(st.Cumulative.Down) + " · ↑ " + tile.Bytes(st.Cumulative.Up)},
		"errors":   {Text: fmt.Sprint(errs), Level: errLvl},
		"checking": {Text: fmt.Sprint(checking)},
		"dl":       {Head: []string{"Nom", "Avancement", "Débit", "Reste"}, Rows: dlRows, Text: "aucun"},
		"topup":    {Head: []string{"Nom", "Ratio", "Débit"}, Rows: upRows, Text: "aucun envoi"},
	}
	if st.Cumulative.Down > 0 {
		v["ratio"] = tile.Value{Text: tile.Num(st.Cumulative.Up / st.Cumulative.Down)}
	}
	for k, x := range s.slow {
		v[k] = x
	}
	return v, nil
}
