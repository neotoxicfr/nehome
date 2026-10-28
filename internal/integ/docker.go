package integ

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Docker Engine (GET seulement) via un proxy de socket en lecture
// (http://socket-proxy:2375, réseau socket-r). Le client est partagé avec la
// tuile « link » (link.go) mais chaque tuile a le sien : aucun état global.
type dockerAPI struct {
	base string
	hc   *http.Client
}

const dockerDefaultURL = "http://socket-proxy:2375"

func newDockerAPI(base string) *dockerAPI {
	if base == "" {
		base = dockerDefaultURL
	}
	return &dockerAPI{base: strings.TrimRight(base, "/"), hc: httpClient(false, false)}
}

func (d *dockerAPI) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, d.hc, d.base+path, out, nil)
}

// restart : POST /containers/{nom}/restart?t=10 (10 s d'arrêt propre avant
// SIGKILL). Seule écriture de l'API : le proxy doit l'autoriser.
func (d *dockerAPI) restart(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	u := d.base + "/containers/" + url.PathEscape(name) + "/restart?t=10"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	hc := *d.hc
	hc.Timeout = 0 // borné par le contexte : arrêt (10 s) + démarrage
	resp, err := hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("conteneur %q introuvable", name)
	case http.StatusForbidden, http.StatusMethodNotAllowed, http.StatusUnauthorized:
		return errors.New("le proxy Docker refuse le redémarrage (POST /containers/…/restart non autorisé)")
	}
	return statusError{resp.StatusCode, redact(u)}
}

// dkProtected : conteneurs qu'on ne propose jamais de redémarrer (option
// protect, « nehome » par défaut : le tableau ne se coupe pas lui-même).
func dkProtected(opt string) map[string]bool {
	if strings.TrimSpace(opt) == "" {
		opt = "nehome"
	}
	m := map[string]bool{}
	for _, n := range strings.Split(opt, ",") {
		if n = strings.TrimSpace(n); n != "" {
			m[n] = true
		}
	}
	return m
}

// Sous-ensembles utiles des réponses de l'API.
type dkContainer struct {
	ID, Image, State, Status string
	Names                    []string
}

type dkInspect struct {
	RestartCount int
	Config       struct{ Image string }
	State        struct {
		Status                string
		Running, Restarting   bool
		ExitCode              int
		StartedAt, FinishedAt time.Time
		Health                *struct{ Status string }
	}
}

type dkCPU struct {
	CPUUsage struct {
		Total  uint64   `json:"total_usage"`
		Percpu []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	System uint64 `json:"system_cpu_usage"`
	Online int    `json:"online_cpus"`
}

type dkStats struct {
	CPU    dkCPU `json:"cpu_stats"`
	Pre    dkCPU `json:"precpu_stats"`
	Memory struct {
		Usage, Limit uint64
		Stats        map[string]uint64
	} `json:"memory_stats"`
}

func (d *dockerAPI) inspect(ctx context.Context, name string) (*dkInspect, error) {
	var in dkInspect
	if err := d.get(ctx, "/containers/"+url.PathEscape(name)+"/json", &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// cpuPct : part de cœur utilisée (100 = un cœur) entre les deux échantillons
// que renvoie stream=false (précédent à ~1 s).
func (s dkStats) cpuPct() float64 {
	dc := float64(s.CPU.CPUUsage.Total) - float64(s.Pre.CPUUsage.Total)
	ds := float64(s.CPU.System) - float64(s.Pre.System)
	if dc < 0 || ds <= 0 {
		return 0
	}
	n := s.CPU.Online
	if n == 0 {
		n = len(s.CPU.CPUUsage.Percpu)
	}
	if n == 0 {
		n = 1
	}
	return dc / ds * float64(n) * 100
}

// mem : usage sans le cache de pages (inactive_file en cgroup v2, cache en v1),
// comme `docker stats`.
func (s dkStats) mem() float64 {
	u := float64(s.Memory.Usage)
	c := s.Memory.Stats["inactive_file"]
	if c == 0 {
		c = s.Memory.Stats["cache"]
	}
	if f := float64(c); f < u {
		u -= f
	}
	return u
}

// ctrState traduit l'état d'un conteneur inspecté en texte français + niveau.
func ctrState(in *dkInspect) (string, string) {
	st := in.State
	h := ""
	if st.Health != nil {
		h = st.Health.Status
	}
	switch {
	case st.Restarting:
		return "redémarre en boucle", "crit"
	case st.Status == "created":
		return "Created (jamais démarré)", "crit"
	case st.Status == "paused":
		return "en pause", "warn"
	case st.Status == "exited" || st.Status == "dead":
		if st.ExitCode != 0 {
			return fmt.Sprintf("arrêté (code %d)", st.ExitCode), "crit"
		}
		return "arrêté", "warn"
	case h == "unhealthy":
		return "unhealthy", "crit"
	case h == "starting":
		return "démarrage", "warn"
	case h == "healthy":
		return "sain", "ok"
	case st.Running:
		return "en marche", "ok"
	}
	return st.Status, "warn"
}

type docker struct {
	api       *dockerAPI
	updateLog string
	protect   map[string]bool

	mu    sync.Mutex
	names []string // conteneurs redémarrables, relevés au dernier passage
}

func newDocker(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	return &docker{api: newDockerAPI(t.URL), updateLog: t.Options["update_log"], protect: dkProtected(t.Options["protect"])}, nil
}

func (d *docker) Actions() []tile.Action {
	d.mu.Lock()
	names := slices.Clone(d.names)
	d.mu.Unlock()
	if len(names) == 0 {
		return nil
	}
	return []tile.Action{{ID: "restart", Label: "Redémarrer un conteneur", Confirm: "Redémarrer ce conteneur ?", Choices: names}}
}

func (d *docker) Do(ctx context.Context, id, arg string) (string, error) {
	if id != "restart" {
		return "", fmt.Errorf("action inconnue : %s", id)
	}
	if arg == "" || d.protect[arg] {
		return "", errors.New("ce conteneur ne peut pas être redémarré d'ici")
	}
	if err := d.api.restart(ctx, arg); err != nil {
		return "", err
	}
	return arg + " redémarré", nil
}

func (d *docker) Fields() []tile.Field {
	return []tile.Field{
		tile.F("running", "Conteneurs en marche", tile.Stat, tile.OnTile),
		tile.F("healthy", "Sains", tile.Stat, tile.OnTile),
		tile.F("trouble", "Problèmes", tile.Stat, tile.OnTile),
		tile.F("update", "Mise à jour nocturne", tile.Stat, tile.OnTile),
		tile.F("total", "Total", tile.Stat, tile.InDetail),
		tile.F("unhealthy", "Unhealthy", tile.Stat, tile.InDetail),
		tile.F("stopped", "Arrêtés", tile.Stat, tile.InDetail),
		tile.F("created", "Created (jamais démarrés)", tile.Stat, tile.InDetail),
		tile.F("images", "Images", tile.Stat, tile.InDetail),
		tile.F("usage", "Ressources des conteneurs", tile.Stat, tile.InDetail),
		tile.F("problems", "Conteneurs à surveiller", tile.Table, tile.InDetail),
		tile.F("cpu", "CPU par conteneur", tile.Table, tile.InDetail),
		tile.F("ram", "Mémoire par conteneur", tile.Table, tile.InDetail),
		tile.F("update_detail", "Dernière mise à jour nocturne", tile.Table, tile.InDetail),
	}
}

func (d *docker) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	if d.updateLog != "" {
		go every(ctx, 5*time.Minute, func() {
			if v := d.updateValues(); ctx.Err() == nil {
				e[0](v, nil)
			}
		})
	}
	every(ctx, time.Minute, func() {
		v, err := d.snapshot(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

type dkRow struct {
	c        dkContainer
	name     string
	in       *dkInspect
	cpu, mem float64
	limit    uint64
	stats    bool
}

type dkProblem struct {
	sev   int
	cells []tile.Cell
}

func (d *docker) snapshot(ctx context.Context) (tile.Values, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	var list []dkContainer
	if err := d.api.get(ctx, "/containers/json?all=1", &list); err != nil {
		return nil, err
	}
	rows := make([]dkRow, len(list))
	sem := make(chan struct{}, 8) // borne la concurrence : chaque stats dure ~1 s
	var wg sync.WaitGroup
	for i, c := range list {
		rows[i] = dkRow{c: c, name: c.ID[:min(12, len(c.ID))]}
		if len(c.Names) > 0 {
			rows[i].name = strings.TrimPrefix(c.Names[0], "/")
		}
		wg.Add(1)
		go func(r *dkRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r.in, _ = d.api.inspect(ctx, r.c.ID)
			if r.c.State != "running" {
				return
			}
			var s dkStats
			if d.api.get(ctx, "/containers/"+r.c.ID+"/stats?stream=false", &s) == nil {
				r.cpu, r.mem, r.limit, r.stats = s.cpuPct(), s.mem(), s.Memory.Limit, true
			}
		}(&rows[i])
	}
	wg.Wait()

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if !d.protect[r.name] {
			names = append(names, r.name)
		}
	}
	sort.Strings(names)
	d.mu.Lock()
	d.names = names
	d.mu.Unlock()

	var running, healthy, unhealthy, stopped, created int
	var probs []dkProblem
	for _, r := range rows {
		h := healthOf(r)
		switch r.c.State {
		case "running", "restarting", "paused":
			running++
		case "created":
			created++
		default:
			stopped++
		}
		if h == "healthy" {
			healthy++
		}
		if h == "unhealthy" {
			unhealthy++
		}
		if p, ok := problemOf(r, h); ok {
			probs = append(probs, p)
		}
	}
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].sev < probs[j].sev })
	prows := make([][]tile.Cell, 0, 15)
	lvl := "ok"
	for _, p := range probs {
		if len(prows) < 15 {
			prows = append(prows, p.cells)
		}
		if p.sev <= 2 {
			lvl = "crit"
		} else if lvl == "ok" {
			lvl = "warn"
		}
	}

	v := tile.Values{
		"total":     {Text: fmt.Sprint(len(rows))},
		"running":   {Text: fmt.Sprintf("%d / %d", running, len(rows)), Sub: "en marche"},
		"healthy":   {Text: fmt.Sprint(healthy), Sub: "sains", Level: levelIf(unhealthy > 0, "crit", "ok")},
		"unhealthy": {Text: fmt.Sprint(unhealthy), Level: levelIf(unhealthy > 0, "crit", "ok")},
		"stopped":   {Text: fmt.Sprint(stopped), Level: levelIf(stopped > 0, "warn", "ok")},
		"created":   {Text: fmt.Sprint(created), Level: levelIf(created > 0, "crit", "ok")},
		"trouble":   {Text: fmt.Sprint(len(probs)), Sub: "à surveiller", Level: lvl},
		"problems":  {Head: []string{"Conteneur", "État", "Santé", "Redémarrages", "Depuis"}, Rows: prows, Text: "aucun problème"},
	}
	if len(probs) == 0 {
		v["trouble"] = tile.Value{Text: "0", Sub: "tout va bien", Level: "ok"}
	}

	// CPU et mémoire : top 15, les conteneurs sans mesure sont ignorés.
	var cpu, ram []dkRow
	var sumCPU, sumMem float64
	for _, r := range rows {
		if r.stats {
			cpu, ram = append(cpu, r), append(ram, r)
			sumCPU += r.cpu
			sumMem += r.mem
		}
	}
	sort.SliceStable(cpu, func(i, j int) bool { return cpu[i].cpu > cpu[j].cpu })
	sort.SliceStable(ram, func(i, j int) bool { return ram[i].mem > ram[j].mem })
	var hostMem float64
	var info struct{ MemTotal float64 }
	if d.api.get(ctx, "/info", &info) == nil {
		hostMem = info.MemTotal
	}
	var crow, mrow [][]tile.Cell
	for _, r := range cpu[:min(15, len(cpu))] {
		crow = append(crow, []tile.Cell{{Text: r.name}, {Text: tile.Pct(r.cpu), Level: tile.Level(r.cpu, 100, 300)}})
	}
	for _, r := range ram[:min(15, len(ram))] {
		lim, l := "sans limite", ""
		if f := float64(r.limit); r.limit > 0 && (hostMem == 0 || f < hostMem) {
			lim, l = tile.Bytes(f), tile.Level(r.mem/f*100, 80, 95)
			if l == "ok" {
				l = ""
			}
		}
		mrow = append(mrow, []tile.Cell{{Text: r.name}, {Text: tile.Bytes(r.mem), Level: l}, {Text: lim}})
	}
	v["cpu"] = tile.Value{Head: []string{"Conteneur", "CPU (100 % = un cœur)"}, Rows: crow}
	v["ram"] = tile.Value{Head: []string{"Conteneur", "Mémoire", "Limite"}, Rows: mrow}
	v["usage"] = tile.Value{Text: tile.Pct(sumCPU) + " CPU", Sub: tile.Bytes(sumMem) + " de mémoire"}

	// Facultatif : un proxy qui refuse /images n'efface pas la tuile.
	var imgs []struct{ ID string }
	if d.api.get(ctx, "/images/json", &imgs) == nil {
		v["images"] = tile.Value{Text: fmt.Sprint(len(imgs))}
	}
	return v, nil
}

func levelIf(c bool, yes, no string) string {
	if c {
		return yes
	}
	return no
}

// healthOf : « healthy », « unhealthy », « starting » ou « » (pas de sonde).
func healthOf(r dkRow) string {
	if r.in != nil && r.in.State.Health != nil {
		return r.in.State.Health.Status
	}
	switch {
	case strings.Contains(r.c.Status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(r.c.Status, "(healthy)"):
		return "healthy"
	}
	return ""
}

// problemOf classe un conteneur à surveiller : 0 Created, 1 en boucle ou
// unhealthy, 2 arrêté en erreur, 3 arrêté proprement ou en pause, 4 trop de
// redémarrages.
func problemOf(r dkRow, h string) (dkProblem, bool) {
	var txt, lvl string
	sev := -1
	rc := 0
	if r.in != nil {
		rc = r.in.RestartCount
		txt, lvl = ctrState(r.in)
	} else { // inspect en échec : on se rabat sur la liste
		txt, lvl = r.c.State, levelIf(r.c.State == "running", "ok", "warn")
	}
	switch {
	case r.c.State == "created":
		sev = 0
	case r.c.State == "restarting" || h == "unhealthy":
		sev = 1
	case r.c.State == "exited" || r.c.State == "dead":
		sev = levelSev(lvl)
	case r.c.State == "paused":
		sev = 3
	case rc >= 3:
		sev, txt, lvl = 4, "en marche", "warn"
	}
	if sev < 0 {
		return dkProblem{}, false
	}
	since := ""
	if r.in != nil {
		if r.in.State.Running {
			since = tile.Dur(time.Since(r.in.State.StartedAt))
		} else {
			since = tile.Ago(r.in.State.FinishedAt)
		}
	}
	hl := ""
	if h == "unhealthy" {
		hl = "crit"
	}
	rcl := ""
	if rc >= 3 {
		rcl = "warn"
	}
	if h == "" {
		h = "—"
	}
	return dkProblem{sev, []tile.Cell{{Text: r.name}, {Text: txt, Level: lvl}, {Text: h, Level: hl}, {Text: fmt.Sprint(rc), Level: rcl}, {Text: since}}}, true
}

func levelSev(lvl string) int {
	if lvl == "crit" {
		return 2
	}
	return 3
}

// ---- Journal de la mise à jour nocturne (update.sh) ----

var (
	updLineRe  = regexp.MustCompile(`^\[(\d{4})-(\d\d)-(\d\d) (\d\d:\d\d):(\d\d)\] (.*)$`)
	updRecrRe  = regexp.MustCompile(`^\s*Container (\S+)\s+Recreated\s*$`)
	updErrRe   = regexp.MustCompile(`(?i)echoue|failed|sante ko|souffrance|non envoye|erreur`)
	updStackRe = regexp.MustCompile(`Stacks traites : (\d+)`)
)

type updSummary struct {
	start, end   time.Time
	date         string // « 27/10 à 02:30 », lu tel quel dans le journal
	done         bool
	stacks       string
	recreated    []string
	errs         []string
	hubUpdated   bool
	hasStartLine bool
}

// parseUpdate lit un update.log. Les horodatages sont ceux du NAS, lus sans
// fuseau : seule leur différence compte (durée).
func parseUpdate(text string) updSummary {
	var s updSummary
	seen := map[string]bool{}
	for _, ln := range strings.Split(text, "\n") {
		if m := updRecrRe.FindStringSubmatch(ln); m != nil {
			if !seen[m[1]] {
				seen[m[1]] = true
				s.recreated = append(s.recreated, m[1])
			}
			continue
		}
		m := updLineRe.FindStringSubmatch(ln)
		if m == nil {
			if strings.Contains(ln, "Error response from daemon") && len(s.errs) < 8 {
				s.errs = append(s.errs, strings.TrimSpace(ln))
			}
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05", m[1]+"-"+m[2]+"-"+m[3]+" "+m[4]+":"+m[5])
		msg := m[6]
		switch {
		case strings.Contains(msg, "=== DEBUT MISE A JOUR"):
			s.start, s.hasStartLine = t, true
			s.date = m[3] + "/" + m[2] + " à " + m[4]
		case strings.Contains(msg, "=== FIN ==="):
			s.end, s.done = t, true
		case updStackRe.MatchString(msg):
			s.stacks = updStackRe.FindStringSubmatch(msg)[1]
		case strings.Contains(msg, "hub crowdsec mis a jour"):
			s.hubUpdated = true
		case updErrRe.MatchString(msg) && !strings.Contains(msg, "aucun unhealthy"):
			if len(s.errs) < 8 {
				s.errs = append(s.errs, msg)
			}
		}
	}
	return s
}

func (d *docker) updateValues() tile.Values {
	b, err := os.ReadFile(d.updateLog)
	if err != nil {
		return tile.Values{"update": {Text: "journal illisible", Level: "warn"}}
	}
	s := parseUpdate(string(b))
	if !s.hasStartLine {
		return tile.Values{"update": {Text: "journal vide", Level: "warn"}}
	}
	lvl, sub := "ok", fmt.Sprintf("%d conteneur(s) recréé(s)", len(s.recreated))
	switch {
	case len(s.errs) > 0:
		lvl, sub = "crit", fmt.Sprintf("%d erreur(s), %s", len(s.errs), sub)
	case !s.done:
		lvl, sub = "warn", "en cours ou interrompue, "+sub
	}
	rows := [][]tile.Cell{{{Text: "Début"}, {Text: s.date}}}
	if s.done {
		rows = append(rows, []tile.Cell{{Text: "Durée"}, {Text: tile.Dur(s.end.Sub(s.start))}})
	} else {
		rows = append(rows, []tile.Cell{{Text: "Fin"}, {Text: "non atteinte", Level: "warn"}})
	}
	if s.stacks != "" {
		rows = append(rows, []tile.Cell{{Text: "Piles traitées"}, {Text: s.stacks}})
	}
	recr := "aucun"
	if len(s.recreated) > 0 {
		recr = strings.Join(s.recreated[:min(12, len(s.recreated))], ", ")
		if len(s.recreated) > 12 {
			recr += fmt.Sprintf(" (+%d)", len(s.recreated)-12)
		}
	}
	rows = append(rows, []tile.Cell{{Text: "Recréés"}, {Text: recr}})
	if s.hubUpdated {
		rows = append(rows, []tile.Cell{{Text: "Hub CrowdSec"}, {Text: "mis à jour"}})
	}
	for _, e := range s.errs {
		if len(rows) >= 15 {
			break
		}
		rows = append(rows, []tile.Cell{{Text: "Erreur"}, {Text: e, Level: "crit"}})
	}
	return tile.Values{
		"update":        {Text: s.date, Sub: sub, Level: lvl},
		"update_detail": {Head: []string{"Élément", "Détail"}, Rows: rows},
	}
}
