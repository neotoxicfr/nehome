package integ

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Cleanuparr : strikes (téléchargements bloqués ou imports ratés), retraits,
// tâches planifiées. Clé API dans l'en-tête X-Api-Key (son login exige la 2FA).
type cleanuparr struct {
	url, key string
	hc       *http.Client

	mu   sync.Mutex
	jobs []string // types des tâches planifiées, choix de l'action « run »
}

func newCleanuparr(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	c := &cleanuparr{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false)}
	var err error
	if c.key, err = secret("key"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *cleanuparr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("struck", "Téléchargements en strike", tile.Stat, tile.OnTile),
		tile.F("removals", "Retirés", tile.Stat, tile.OnTile),
		tile.F("next", "Prochain nettoyage", tile.Stat, tile.OnTile),
		tile.F("clients", "Clients de téléchargement", tile.Stat, tile.OnTile),
		tile.F("strikes_total", "Strikes au total", tile.Stat, tile.InDetail),
		tile.F("searches", "Recherches de remplacement", tile.Stat, tile.InDetail),
		tile.F("jobs_failed", "Tâches en échec", tile.Stat, tile.InDetail),
		tile.F("strikes", "Strikes en cours", tile.Table, tile.InDetail),
		tile.F("events", "Derniers événements", tile.Table, tile.InDetail),
		tile.F("jobs", "Tâches planifiées", tile.Table, tile.InDetail),
	}
}

func (c *cleanuparr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, c.hc, c.url+path, out, func(r *http.Request) { r.Header.Set("X-Api-Key", c.key) })
}

func (c *cleanuparr) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		if v := c.slow(ctx); ctx.Err() == nil && v != nil {
			e[1](v, nil)
		}
	})
	every(ctx, time.Minute, func() {
		v, err := c.fast(ctx)
		if ctx.Err() == nil {
			e[0](v, err)
		}
	})
}

type cuStats struct {
	Strikes struct {
		Total int `json:"total"`
	} `json:"strikes"`
	Removals struct {
		Total    int            `json:"total"`
		ByReason map[string]int `json:"byReason"`
	} `json:"removals"`
	Searches struct {
		Total   int `json:"total"`
		Grabbed int `json:"grabbed"`
		Failed  int `json:"failed"`
	} `json:"searches"`
	Jobs struct {
		Failed int `json:"failed"`
		ByType map[string]struct {
			NextRunAt *time.Time `json:"nextRunAt"`
		} `json:"byType"`
	} `json:"jobs"`
}

type cuStrike struct {
	Title         string         `json:"title"`
	TotalStrikes  int            `json:"totalStrikes"`
	StrikesByType map[string]int `json:"strikesByType"`
	Latest        time.Time      `json:"latestStrikeAt"`
	Marked        bool           `json:"isMarkedForRemoval"`
	Removed       bool           `json:"isRemoved"`
}

func (c *cleanuparr) fast(ctx context.Context) (tile.Values, error) {
	var st cuStats
	if err := c.get(ctx, "/api/v2/stats", &st); err != nil {
		return nil, err
	}
	v := tile.Values{}
	v["strikes_total"] = tile.Value{Text: fmt.Sprint(st.Strikes.Total)}
	rem := tile.Value{Text: fmt.Sprint(st.Removals.Total)}
	var reasons []string
	for r, n := range st.Removals.ByReason {
		reasons = append(reasons, fmt.Sprintf("%s %d", r, n))
	}
	slices.Sort(reasons)
	rem.Sub = strings.Join(reasons, " · ")
	v["removals"] = rem
	v["searches"] = tile.Value{Text: fmt.Sprint(st.Searches.Total), Sub: fmt.Sprintf("%d trouvée(s), %d échec(s)", st.Searches.Grabbed, st.Searches.Failed)}
	lvl := "ok"
	if st.Jobs.Failed > 0 {
		lvl = "warn"
	}
	v["jobs_failed"] = tile.Value{Text: fmt.Sprint(st.Jobs.Failed), Level: lvl}
	if j, ok := st.Jobs.ByType["QueueCleaner"]; ok && j.NextRunAt != nil {
		v["next"] = tile.Value{Text: j.NextRunAt.Local().Format("15:04"), Sub: "dans " + tile.Dur(time.Until(*j.NextRunAt))}
	}

	var sk struct {
		Items []cuStrike `json:"items"`
	}
	if c.get(ctx, "/api/strikes?pageSize=50", &sk) == nil {
		var active []cuStrike
		for _, s := range sk.Items {
			if !s.Removed {
				active = append(active, s)
			}
		}
		slices.SortFunc(active, func(a, b cuStrike) int { return b.Latest.Compare(a.Latest) })
		struck := tile.Value{Text: fmt.Sprint(len(active)), Level: "ok"}
		marked := 0
		var rows [][]tile.Cell
		for _, s := range active {
			if s.Marked {
				marked++
			}
			var types []string
			for t, n := range s.StrikesByType {
				types = append(types, fmt.Sprintf("%s %d", t, n))
			}
			slices.Sort(types)
			lvl := ""
			if s.Marked {
				lvl = "crit"
			}
			rows = append(rows, []tile.Cell{{Text: s.Title}, {Text: strings.Join(types, ", "), Level: lvl}, {Text: dateText(s.Latest)}})
		}
		if len(active) > 0 {
			struck.Level = "warn"
		}
		if marked > 0 {
			struck.Sub, struck.Level = fmt.Sprintf("%d à retirer", marked), "crit"
		}
		v["struck"] = struck
		v["strikes"] = tile.Value{Head: []string{"Téléchargement", "Strikes", "Dernier"}, Rows: rows[:min(15, len(rows))], Text: "aucun"}
	}

	var health map[string]struct {
		Healthy bool   `json:"isHealthy"`
		Name    string `json:"clientName"`
		Err     string `json:"errorMessage"`
	}
	if c.get(ctx, "/api/health", &health) == nil {
		ok, names := 0, []string{}
		for _, h := range health {
			if h.Healthy {
				ok++
			} else {
				names = append(names, h.Name)
			}
		}
		hv := tile.Value{Text: fmt.Sprintf("%d / %d", ok, len(health)), Level: "ok"}
		if len(names) > 0 {
			slices.Sort(names)
			hv.Sub, hv.Level = "en panne : "+strings.Join(names, ", "), "crit"
		}
		v["clients"] = hv
	}
	return v, nil
}

type cuJob struct {
	Name   string     `json:"name"`
	Type   string     `json:"jobType"`
	Status string     `json:"status"`
	Prev   *time.Time `json:"previousRunTime"`
	Next   *time.Time `json:"nextRunTime"`
}

func (c *cleanuparr) slow(ctx context.Context) tile.Values {
	v := tile.Values{}
	var ev struct {
		Items []struct {
			Time     time.Time `json:"timestamp"`
			Type     string    `json:"eventType"`
			Message  string    `json:"message"`
			Severity string    `json:"severity"`
		} `json:"items"`
	}
	if c.get(ctx, "/api/events?pageSize=15", &ev) == nil {
		var rows [][]tile.Cell
		for _, e := range ev.Items {
			lvl := ""
			if e.Severity == "Important" || e.Severity == "Warning" {
				lvl = "warn"
			}
			if e.Severity == "Error" {
				lvl = "crit"
			}
			rows = append(rows, []tile.Cell{{Text: dateText(e.Time)}, {Text: e.Type, Level: lvl}, {Text: e.Message}})
		}
		v["events"] = tile.Value{Head: []string{"Date", "Type", "Message"}, Rows: rows, Text: "aucun"}
	}
	var jobs []cuJob
	if c.get(ctx, "/api/jobs", &jobs) == nil {
		slices.SortFunc(jobs, func(a, b cuJob) int { return cmp.Compare(a.Name, b.Name) })
		t := func(p *time.Time) string {
			if p == nil {
				return "—"
			}
			return dateText(*p)
		}
		var rows [][]tile.Cell
		var types []string
		for _, j := range jobs {
			if j.Status == "Not Scheduled" {
				continue
			}
			if j.Type != "" && j.Type != "Seeker" { // Seeker refuse le déclenchement manuel
				types = append(types, j.Type)
			}
			rows = append(rows, []tile.Cell{{Text: j.Name}, {Text: t(j.Prev)}, {Text: t(j.Next)}})
		}
		c.mu.Lock()
		c.jobs = types
		c.mu.Unlock()
		v["jobs"] = tile.Value{Head: []string{"Tâche", "Dernière", "Prochaine"}, Rows: rows, Text: "aucune"}
	}
	return v
}

func (c *cleanuparr) Actions() []tile.Action {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.jobs) == 0 {
		return nil
	}
	return []tile.Action{{ID: "run", Label: "Lancer une tâche maintenant", Confirm: "Lancer cette tâche hors planning ? Elle applique ses règles (strikes, retraits) tout de suite.", Choices: slices.Clone(c.jobs)}}
}

// Do : POST /api/jobs/{type}/trigger (exécution unique, le planning ne change pas).
func (c *cleanuparr) Do(ctx context.Context, id, job string) (string, error) {
	c.mu.Lock()
	ok := id == "run" && slices.Contains(c.jobs, job)
	c.mu.Unlock()
	if !ok {
		return "", errUnknownAction
	}
	err := callAction(ctx, c.hc, http.MethodPost, c.url+"/api/jobs/"+job+"/trigger", nil, func(r *http.Request) { r.Header.Set("X-Api-Key", c.key) })
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusBadRequest {
		return "", errors.New("Cleanuparr refuse : tâche absente ou non configurée")
	}
	if err != nil {
		return "", err
	}
	return job + " lancée", nil
}
