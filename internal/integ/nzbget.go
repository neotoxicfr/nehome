package integ

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// NZBGet : JSON-RPC sur /jsonrpc, basic auth (ControlUsername/ControlPassword).
// État et file toutes les 3 s, historique (potentiellement long) chaque minute.
type nzbget struct {
	url, user, pass string
	hc              *http.Client
	id              int

	down  *tile.Ring
	hist  tile.Values
	histT time.Time
}

type nzbStatus struct {
	RemainingMB  float64 `json:"RemainingSizeMB"`
	DownloadedMB float64 `json:"DownloadedSizeMB"`
	FreeMB       float64 `json:"FreeDiskSpaceMB"`
	Rate         float64 `json:"DownloadRate"`
	Limit        float64 `json:"DownloadLimit"`
	Uptime       int64   `json:"UpTimeSec"`
	Paused       bool    `json:"DownloadPaused"`
	PostPaused   bool    `json:"PostPaused"`
	ScanPaused   bool    `json:"ScanPaused"`
	Threads      int     `json:"ThreadCount"`
	ParJobs      int     `json:"ParJobCount"`
	PostJobs     int     `json:"PostJobCount"`
	CacheMB      float64 `json:"ArticleCacheMB"`
}

type nzbGroup struct {
	Name     string  `json:"NZBName"`
	Status   string  `json:"Status"`
	FileMB   float64 `json:"FileSizeMB"`
	RemainMB float64 `json:"RemainingSizeMB"`
	Category string  `json:"Category"`
	Health   float64 `json:"Health"` // 0 à 1000
}

func newNZBGet(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &nzbget{url: strings.TrimRight(t.URL, "/") + "/jsonrpc", hc: httpClient(t.InsecureTLS, false), down: tile.NewRing(60)}
	if _, ok := t.Secrets["user"]; ok {
		var err error
		if n.user, err = secret("user"); err != nil {
			return nil, err
		}
		if n.pass, err = secret("pass"); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func (n *nzbget) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("queue", "En file", tile.Stat, tile.OnTile),
		tile.F("left", "Reste à télécharger", tile.Stat, tile.OnTile),
		tile.F("status", "État", tile.Stat, tile.OnTile),
		tile.F("free", "Espace libre", tile.Stat, tile.InDetail),
		tile.F("limit", "Limite de débit", tile.Stat, tile.InDetail),
		tile.F("session", "Téléchargé depuis le démarrage", tile.Stat, tile.InDetail),
		tile.F("post", "Post-traitements", tile.Stat, tile.InDetail),
		tile.F("threads", "Fils actifs", tile.Stat, tile.InDetail),
		tile.F("cache", "Cache d'articles", tile.Stat, tile.InDetail),
		tile.F("uptime", "En marche depuis", tile.Stat, tile.InDetail),
		tile.F("failed", "Échecs dans l'historique", tile.Stat, tile.InDetail),
		tile.F("dl", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("history", "Historique récent", tile.Table, tile.InDetail),
	}
}

func (n *nzbget) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 3*time.Second, func() {
		var st nzbStatus
		if err := n.rpc(ctx, "status", nil, &st); err != nil {
			if ctx.Err() == nil {
				emit(nil, err)
			}
			return
		}
		var groups []nzbGroup
		gerr := n.rpc(ctx, "listgroups", []any{0}, &groups)
		if time.Since(n.histT) > time.Minute {
			n.histT = time.Now()
			n.refreshHistory(ctx)
		}
		v := n.values(st, groups, gerr == nil)
		for k, x := range n.hist {
			v[k] = x
		}
		emit(v, nil)
	})
}

func (n *nzbget) rpc(ctx context.Context, method string, params []any, out any) error {
	n.id++
	if params == nil {
		params = []any{}
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_, err := dlPost(ctx, n.hc, n.url, map[string]any{"method": method, "params": params, "id": n.id}, &env, func(r *http.Request) {
		if n.user != "" {
			r.SetBasicAuth(n.user, n.pass)
		}
	})
	if err != nil {
		return err
	}
	if env.Error != nil {
		return fmt.Errorf("NZBGet : %s", env.Error.Message)
	}
	return json.Unmarshal(env.Result, out)
}

const nzbMiB = 1 << 20

func (n *nzbget) values(st nzbStatus, groups []nzbGroup, haveGroups bool) tile.Values {
	state, lvl := "en téléchargement", "ok"
	switch {
	case st.Paused:
		state, lvl = "en pause", "warn"
	case st.Rate == 0 && st.RemainingMB == 0:
		state, lvl = "inactif", ""
	}
	left := tile.Value{Text: tile.Bytes(st.RemainingMB * nzbMiB)}
	if st.Rate > 0 && st.RemainingMB > 0 {
		left.Sub = tile.Dur(time.Duration(st.RemainingMB*nzbMiB/st.Rate) * time.Second)
	}
	limit := "∞"
	if st.Limit > 0 {
		limit = tile.Rate(st.Limit)
	}
	v := tile.Values{
		"down":    {Text: tile.Rate(st.Rate), Points: n.down.Push(st.Rate)},
		"left":    left,
		"status":  {Text: state, Level: lvl},
		"free":    {Text: tile.Bytes(st.FreeMB * nzbMiB)},
		"limit":   {Text: limit},
		"session": {Text: tile.Bytes(st.DownloadedMB * nzbMiB)},
		"post":    {Text: fmt.Sprint(st.PostJobs), Sub: fmt.Sprintf("dont %d réparations", st.ParJobs)},
		"threads": {Text: fmt.Sprint(st.Threads)},
		"cache":   {Text: tile.Bytes(st.CacheMB * nzbMiB)},
		"uptime":  {Text: tile.Dur(time.Duration(st.Uptime) * time.Second)},
	}
	if haveGroups {
		slices.SortFunc(groups, func(a, b nzbGroup) int {
			return cmp.Compare(b.FileMB-b.RemainMB, a.FileMB-a.RemainMB)
		})
		var rows [][]tile.Cell
		for _, g := range groups[:min(15, len(groups))] {
			pct := 0.0
			if g.FileMB > 0 {
				pct = (g.FileMB - g.RemainMB) / g.FileMB * 100
			}
			cell := tile.Cell{Text: strings.ToLower(strings.ReplaceAll(g.Status, "_", " "))}
			if g.Health > 0 && g.Health < 1000 {
				cell.Level = "warn"
			}
			rows = append(rows, []tile.Cell{{Text: g.Name}, {Text: tile.Pct(pct)}, {Text: tile.Bytes(g.RemainMB * nzbMiB)}, cell})
		}
		v["queue"] = tile.Value{Text: fmt.Sprint(len(groups)), Sub: fmt.Sprintf("%d en pause", countPaused(groups))}
		v["dl"] = tile.Value{Head: []string{"Nom", "Avancement", "Reste", "État"}, Rows: rows, Text: "file vide"}
	}
	return v
}

func countPaused(g []nzbGroup) (c int) {
	for _, x := range g {
		if x.Status == "PAUSED" {
			c++
		}
	}
	return
}

func (n *nzbget) refreshHistory(ctx context.Context) {
	var h []struct {
		Name   string  `json:"Name"`
		Status string  `json:"Status"` // « SUCCESS/ALL », « FAILURE/UNPACK »…
		SizeMB float64 `json:"FileSizeMB"`
		Time   int64   `json:"HistoryTime"`
	}
	if n.rpc(ctx, "history", []any{false}, &h) != nil {
		return
	}
	slices.SortFunc(h, func(a, b struct {
		Name   string  `json:"Name"`
		Status string  `json:"Status"`
		SizeMB float64 `json:"FileSizeMB"`
		Time   int64   `json:"HistoryTime"`
	}) int {
		return cmp.Compare(b.Time, a.Time)
	})
	failed := 0
	var rows [][]tile.Cell
	for i, e := range h {
		kind, detail, _ := strings.Cut(e.Status, "/")
		lvl, txt := "", strings.ToLower(e.Status)
		switch kind {
		case "SUCCESS":
			lvl, txt = "ok", "terminé"
		case "WARNING":
			lvl, txt = "warn", "avertissement : "+strings.ToLower(detail)
		case "FAILURE":
			lvl, txt = "crit", "échec : "+strings.ToLower(detail)
			failed++
		case "DELETED":
			txt = "supprimé"
		}
		if i < 15 {
			rows = append(rows, []tile.Cell{{Text: e.Name}, {Text: txt, Level: lvl}, {Text: tile.Bytes(e.SizeMB * nzbMiB)}, {Text: tile.Ago(time.Unix(e.Time, 0))}})
		}
	}
	flvl := ""
	if failed > 0 {
		flvl = "warn"
	}
	n.hist = tile.Values{
		"history": {Head: []string{"Nom", "État", "Taille", "Il y a"}, Rows: rows, Text: "historique vide"},
		"failed":  {Text: fmt.Sprint(failed), Sub: fmt.Sprintf("sur %d entrées", len(h)), Level: flvl},
	}
}
