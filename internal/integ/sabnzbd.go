package integ

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// SABnzbd : API HTTP /api?mode=… . La clé ne passe que dans la requête (le
// service n'accepte rien d'autre), donc toute erreur est expurgée par redact/cleanErr.
// Trois rythmes : file (3 s), historique (1 min), statistiques serveurs (5 min).
type sabnzbd struct {
	url, key string
	hc       *http.Client

	down  *tile.Ring
	hist  tile.Values
	histT time.Time
	stats tile.Values
	statT time.Time
}

type sabQueue struct {
	Status    string `json:"status"`
	Paused    bool   `json:"paused"`
	Speed     string `json:"kbpersec"` // ko/s (binaires), en chaîne
	Limit     string `json:"speedlimit_abs"`
	Left      string `json:"mbleft"`
	Size      string `json:"mb"`
	TimeLeft  string `json:"timeleft"`
	Disk1     string `json:"diskspace1"` // Go libres, dossier temporaire
	Disk2     string `json:"diskspace2"` // Go libres, dossier des terminés
	Warnings  string `json:"have_warnings"`
	Version   string `json:"version"`
	NoOfSlots int    `json:"noofslots_total"`
	Slots     []struct {
		Name     string `json:"filename"`
		Status   string `json:"status"`
		Percent  string `json:"percentage"`
		Left     string `json:"mbleft"`
		TimeLeft string `json:"timeleft"`
		Cat      string `json:"cat"`
	} `json:"slots"`
}

func newSABnzbd(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &sabnzbd{url: strings.TrimRight(t.URL, "/"), key: key, hc: httpClient(t.InsecureTLS, false), down: tile.NewRing(60)}, nil
}

func (s *sabnzbd) Fields() []tile.Field {
	return []tile.Field{
		tile.F("down", "Réception", tile.Spark, tile.OnTile),
		tile.F("queue", "En file", tile.Stat, tile.OnTile),
		tile.F("left", "Reste à télécharger", tile.Stat, tile.OnTile),
		tile.F("status", "État", tile.Stat, tile.OnTile),
		tile.F("free", "Espace libre", tile.Stat, tile.InDetail),
		tile.F("limit", "Limite de débit", tile.Stat, tile.InDetail),
		tile.F("warnings", "Avertissements", tile.Stat, tile.InDetail),
		tile.F("failed", "Échecs récents", tile.Stat, tile.InDetail),
		tile.F("day", "Aujourd'hui", tile.Stat, tile.InDetail),
		tile.F("week", "Cette semaine", tile.Stat, tile.InDetail),
		tile.F("month", "Ce mois", tile.Stat, tile.InDetail),
		tile.F("total", "Total depuis l'origine", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("dl", "Téléchargements en cours", tile.Table, tile.InDetail),
		tile.F("history", "Historique récent", tile.Table, tile.InDetail),
		tile.F("servers", "Serveurs Usenet", tile.Table, tile.InDetail),
	}
}

func (s *sabnzbd) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 3*time.Second, func() {
		var q struct {
			Queue sabQueue `json:"queue"`
		}
		if err := s.get(ctx, "queue", nil, &q); err != nil {
			if ctx.Err() == nil {
				emit(nil, err)
			}
			return
		}
		// Les requêtes secondaires en échec gardent les valeurs précédentes.
		if time.Since(s.histT) > time.Minute {
			s.histT = time.Now()
			s.refreshHistory(ctx)
		}
		if time.Since(s.statT) > 5*time.Minute {
			s.statT = time.Now()
			s.refreshStats(ctx)
		}
		v := s.queueValues(q.Queue)
		for _, m := range []tile.Values{s.hist, s.stats} {
			for k, x := range m {
				v[k] = x
			}
		}
		emit(v, nil)
	})
}

// sabError : SABnzbd répond 200 avec {"status": false, "error": "..."} sur une clé fausse.
type sabError struct {
	Status any    `json:"status"`
	Error  string `json:"error"`
}

func (s *sabnzbd) get(ctx context.Context, mode string, extra url.Values, out any) error {
	q := url.Values{"mode": {mode}, "output": {"json"}, "apikey": {s.key}}
	for k, v := range extra {
		q[k] = v
	}
	u := s.url + "/api?" + q.Encode()
	var e sabError
	if err := getJSON(ctx, s.hc, u, &sabEnvelope{out: out, err: &e}, nil); err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("SABnzbd : %s", e.Error)
	}
	return nil
}

// sabEnvelope décode la même réponse dans la structure voulue et dans sabError.
type sabEnvelope struct {
	out any
	err *sabError
}

func (e *sabEnvelope) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, e.err); err != nil {
		return err
	}
	if e.err.Error != "" {
		return nil
	}
	return json.Unmarshal(b, e.out)
}

func sabNum(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

var sabStatus = map[string]string{"Downloading": "en téléchargement", "Paused": "en pause", "Idle": "inactif", "Fetching": "récupération", "Checking": "vérification"}

func (s *sabnzbd) queueValues(q sabQueue) tile.Values {
	rate := sabNum(q.Speed) * 1024
	st := sabStatus[q.Status]
	if st == "" {
		st = strings.ToLower(q.Status)
	}
	lvl := "ok"
	if q.Paused {
		st, lvl = "en pause", "warn"
	}
	var rows [][]tile.Cell
	for _, sl := range q.Slots[:min(15, len(q.Slots))] {
		rows = append(rows, []tile.Cell{{Text: sl.Name}, {Text: tile.Pct(sabNum(sl.Percent))}, {Text: tile.Bytes(sabNum(sl.Left) * 1048576)}, {Text: sl.TimeLeft}})
	}
	v := tile.Values{
		"down":   {Text: tile.Rate(rate), Points: s.down.Push(rate)},
		"queue":  {Text: fmt.Sprint(q.NoOfSlots), Sub: tile.Bytes(sabNum(q.Size)*1048576) + " au total"},
		"left":   {Text: tile.Bytes(sabNum(q.Left) * 1048576), Sub: q.TimeLeft},
		"status": {Text: st, Level: lvl},
		"dl":     {Head: []string{"Nom", "Avancement", "Reste", "Durée"}, Rows: rows, Text: "file vide"},
	}
	if q.Disk1 != "" {
		sub := ""
		if q.Disk2 != "" && q.Disk2 != q.Disk1 {
			sub = "terminés : " + tile.Bytes(sabNum(q.Disk2)*(1<<30))
		}
		v["free"] = tile.Value{Text: tile.Bytes(sabNum(q.Disk1) * (1 << 30)), Sub: sub}
	}
	if q.Limit != "" {
		v["limit"] = tile.Value{Text: tile.Rate(sabNum(q.Limit))}
	}
	if n := sabNum(q.Warnings); n > 0 {
		v["warnings"] = tile.Value{Text: fmt.Sprint(int(n)), Level: "warn"}
	}
	if q.Version != "" {
		v["version"] = tile.Value{Text: q.Version}
	}
	return v
}

func (s *sabnzbd) refreshHistory(ctx context.Context) {
	var h struct {
		History struct {
			Slots []struct {
				Name   string  `json:"name"`
				Status string  `json:"status"`
				Bytes  float64 `json:"bytes"`
				Done   int64   `json:"completed"`
				Fail   string  `json:"fail_message"`
			} `json:"slots"`
		} `json:"history"`
	}
	if s.get(ctx, "history", url.Values{"limit": {"15"}}, &h) != nil {
		return
	}
	var rows [][]tile.Cell
	failed := 0
	for _, e := range h.History.Slots {
		lvl, txt := "", strings.ToLower(e.Status)
		switch e.Status {
		case "Completed":
			lvl, txt = "ok", "terminé"
		case "Failed":
			lvl, txt = "crit", "échec"
			failed++
		}
		rows = append(rows, []tile.Cell{{Text: e.Name}, {Text: txt, Level: lvl}, {Text: tile.Bytes(e.Bytes)}, {Text: tile.Ago(time.Unix(e.Done, 0))}})
	}
	lvl := ""
	if failed > 0 {
		lvl = "warn"
	}
	s.hist = tile.Values{
		"history": {Head: []string{"Nom", "État", "Taille", "Il y a"}, Rows: rows, Text: "historique vide"},
		"failed":  {Text: fmt.Sprint(failed), Sub: fmt.Sprintf("sur les %d derniers", len(rows)), Level: lvl},
	}
}

func (s *sabnzbd) refreshStats(ctx context.Context) {
	var st struct {
		Total   float64 `json:"total"`
		Month   float64 `json:"month"`
		Week    float64 `json:"week"`
		Day     float64 `json:"day"`
		Servers map[string]struct {
			Total float64 `json:"total"`
			Day   float64 `json:"day"`
		} `json:"servers"`
	}
	if s.get(ctx, "server_stats", nil, &st) != nil {
		return
	}
	var rows [][]tile.Cell
	for name, sv := range st.Servers {
		rows = append(rows, []tile.Cell{{Text: name}, {Text: tile.Bytes(sv.Day)}, {Text: tile.Bytes(sv.Total)}})
	}
	slices.SortFunc(rows, func(a, b []tile.Cell) int { return cmp.Compare(a[0].Text, b[0].Text) })
	s.stats = tile.Values{
		"day":     {Text: tile.Bytes(st.Day)},
		"week":    {Text: tile.Bytes(st.Week)},
		"month":   {Text: tile.Bytes(st.Month)},
		"total":   {Text: tile.Bytes(st.Total)},
		"servers": {Head: []string{"Serveur", "Aujourd'hui", "Total"}, Rows: rows[:min(15, len(rows))]},
	}
}
