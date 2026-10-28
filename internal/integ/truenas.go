package integ

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// TrueNAS SCALE : API JSON-RPC 2.0 sur websocket (/api/current). Le temps réel
// vient de l'abonnement reporting.realtime (≈ 2 s) ; pools, alertes,
// snapshots et réplications sont relus périodiquement.
type trueNAS struct {
	url, key string
	hc       *http.Client

	cpu, rx, tx *tile.Ring

	mu     sync.Mutex
	cur    *rpc     // connexion authentifiée en cours (nil hors session)
	pools  []string // noms relevés au dernier passage lent
	alerts int      // alertes non acquittées au dernier passage lent
}

func (n *trueNAS) Actions() []tile.Action {
	n.mu.Lock()
	pools, alerts := slices.Clone(n.pools), n.alerts
	n.mu.Unlock()
	var a []tile.Action
	if len(pools) > 0 {
		a = append(a, tile.Action{ID: "scrub", Label: "Lancer un scrub", Confirm: "Lancer un scrub sur ce pool ? Il charge les disques pendant plusieurs heures.", Choices: pools})
	}
	if alerts > 0 {
		a = append(a, tile.Action{ID: "dismiss", Label: "Acquitter les alertes", Confirm: "Acquitter toutes les alertes en cours ?"})
	}
	return a
}

// Do : pool.scrub.run(nom, 0) (seuil 0 jour : le scrub part même si le
// dernier est récent) ; alert.dismiss(uuid) pour chaque alerte non acquittée.
func (n *trueNAS) Do(ctx context.Context, id, arg string) (string, error) {
	n.mu.Lock()
	r := n.cur
	known := slices.Contains(n.pools, arg)
	n.mu.Unlock()
	if r == nil {
		return "", errors.New("TrueNAS non connecté : réessayer dans un instant")
	}
	switch id {
	case "scrub":
		if !known {
			return "", errors.New("pool inconnu")
		}
		if err := r.call(ctx, "pool.scrub.run", nil, arg, 0); err != nil {
			return "", err
		}
		return "Scrub lancé sur " + arg, nil
	case "dismiss":
		var alerts []struct {
			UUID      string `json:"uuid"`
			Dismissed bool   `json:"dismissed"`
		}
		if err := r.call(ctx, "alert.list", &alerts); err != nil {
			return "", err
		}
		done := 0
		for _, a := range alerts {
			if a.Dismissed || a.UUID == "" {
				continue
			}
			if err := r.call(ctx, "alert.dismiss", nil, a.UUID); err != nil {
				return "", fmt.Errorf("%d alerte(s) acquittée(s), puis : %w", done, err)
			}
			done++
		}
		n.mu.Lock()
		n.alerts = 0
		n.mu.Unlock()
		if done == 0 {
			return "Aucune alerte à acquitter", nil
		}
		return fmt.Sprintf("%d alerte(s) acquittée(s)", done), nil
	}
	return "", fmt.Errorf("action inconnue : %s", id)
}

func newTrueNAS(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(t.URL, "/")
	u = strings.Replace(strings.Replace(u, "https://", "wss://", 1), "http://", "ws://", 1) + "/api/current"
	return &trueNAS{url: u, key: key, hc: httpClient(t.InsecureTLS, false), cpu: tile.NewRing(60), rx: tile.NewRing(60), tx: tile.NewRing(60)}, nil
}

func (n *trueNAS) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cpu", "CPU", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("pools", "Pools", tile.Bars, tile.OnTile),
		tile.F("net_rx", "Réseau ↓", tile.Spark, tile.OnTile),
		tile.F("net_tx", "Réseau ↑", tile.Spark, tile.InDetail),
		tile.F("alerts_n", "Alertes", tile.Stat, tile.OnTile),
		tile.F("cpu_hist", "CPU (2 min)", tile.Spark, tile.InDetail),
		tile.F("cputemp", "Température CPU", tile.Stat, tile.InDetail),
		tile.F("arc", "Cache ARC", tile.Stat, tile.InDetail),
		tile.F("disk_io", "Disques lecture / écriture", tile.Stat, tile.InDetail),
		tile.F("load", "Charge", tile.Stat, tile.InDetail),
		tile.F("uptime", "En marche depuis", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
		tile.F("alerts", "Liste des alertes", tile.Table, tile.InDetail),
		tile.F("scrub", "Pools : état et dernier scrub", tile.Table, tile.InDetail),
		tile.F("temps", "Températures des disques", tile.Table, tile.InDetail),
		tile.F("snapshots", "Snapshots périodiques", tile.Table, tile.InDetail),
		tile.F("replication", "Réplications", tile.Table, tile.InDetail),
	}
}

// rpc : connexion JSON-RPC, réponses aiguillées par id, événements à part.
type rpc struct {
	c       *websocket.Conn
	mu      sync.Mutex
	next    int
	pending map[int]chan rpcMsg
}

type rpcMsg struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
		Data    struct {
			Reason string `json:"reason"`
		} `json:"data"`
	} `json:"error"`
}

func (r *rpc) call(ctx context.Context, method string, out any, params ...any) error {
	if params == nil {
		params = []any{}
	}
	r.mu.Lock()
	r.next++
	id := r.next
	ch := make(chan rpcMsg, 1)
	r.pending[id] = ch
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, id); r.mu.Unlock() }()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err := r.c.Write(ctx, websocket.MessageText, b); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return fmt.Errorf("%s : %w", method, ctx.Err())
	case m := <-ch:
		if m.Error != nil {
			msg := m.Error.Data.Reason
			if msg == "" {
				msg = m.Error.Message
			}
			return fmt.Errorf("%s : %s", method, msg)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(m.Result, out)
	}
}

func (n *trueNAS) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2) // 0 : temps réel, 1 : session et données lentes
	wait := 5 * time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := n.session(ctx, e[0], e[1])
		if ctx.Err() != nil {
			return
		}
		e[1](nil, err)
		if time.Since(start) > time.Minute {
			wait = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, time.Minute)
	}
}

func (n *trueNAS) session(ctx context.Context, rt, emit func(tile.Values, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c, _, err := websocket.Dial(ctx, n.url, &websocket.DialOptions{HTTPClient: n.hc})
	if err != nil {
		return fmt.Errorf("connexion à TrueNAS : %w", err)
	}
	defer c.CloseNow()
	c.SetReadLimit(32 << 20)
	r := &rpc{c: c, pending: map[int]chan rpcMsg{}}

	readErr := make(chan error, 1)
	go func() {
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			var m rpcMsg
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			if m.ID != nil {
				r.mu.Lock()
				ch := r.pending[*m.ID]
				r.mu.Unlock()
				if ch != nil {
					ch <- m
				}
				continue
			}
			if m.Method == "collection_update" {
				if v := n.realtime(m.Params); v != nil {
					rt(v, nil)
				}
			}
		}
	}()

	var ok bool
	if err := r.call(ctx, "auth.login_with_api_key", &ok, n.key); err != nil {
		return err
	}
	if !ok {
		return errors.New("TrueNAS refuse la clé API")
	}
	n.mu.Lock()
	n.cur = r
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		if n.cur == r {
			n.cur = nil
		}
		n.mu.Unlock()
	}()
	if err := r.call(ctx, "core.subscribe", nil, "reporting.realtime"); err != nil {
		return err
	}

	slow := func() {
		v, err := n.slow(ctx, r)
		if ctx.Err() != nil {
			return
		}
		emit(v, err)
	}
	slow()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return fmt.Errorf("TrueNAS : connexion perdue (%w)", err)
		case <-tick.C:
			go slow()
		}
	}
}

type tnRealtime struct {
	Collection string `json:"collection"`
	Fields     struct {
		Memory struct {
			ARC   float64 `json:"arc_size"`
			Total float64 `json:"physical_memory_total"`
			Avail float64 `json:"physical_memory_available"`
		} `json:"memory"`
		CPU   map[string]struct{ Usage, Temp float64 } `json:"cpu"`
		Disks struct {
			Read  float64 `json:"read_bytes"`
			Write float64 `json:"write_bytes"`
		} `json:"disks"`
		Interfaces map[string]struct {
			Rx float64 `json:"received_bytes_rate"`
			Tx float64 `json:"sent_bytes_rate"`
		} `json:"interfaces"`
	} `json:"fields"`
}

func (n *trueNAS) realtime(raw json.RawMessage) tile.Values {
	var e tnRealtime
	if json.Unmarshal(raw, &e) != nil || e.Collection != "reporting.realtime" {
		return nil
	}
	f := e.Fields
	v := tile.Values{}
	if c, ok := f.CPU["cpu"]; ok {
		v["cpu"] = tile.Value{Pct: c.Usage, Text: tile.Pct(c.Usage), Level: tile.Level(c.Usage, 75, 90)}
		v["cpu_hist"] = tile.Value{Points: n.cpu.Push(c.Usage), Text: tile.Pct(c.Usage)}
		v["cputemp"] = tile.Value{Text: tile.Num(c.Temp) + " °C", Level: tile.Level(c.Temp, 70, 85)}
	}
	if m := f.Memory; m.Total > 0 {
		apps := max(m.Total-m.Avail-m.ARC, 0)
		v["ram"] = tile.Value{Pct: apps / m.Total * 100, Text: tile.Bytes(apps) + " / " + tile.Bytes(m.Total), Sub: "ARC " + tile.Bytes(m.ARC)}
		v["arc"] = tile.Value{Text: tile.Bytes(m.ARC), Sub: tile.Bytes(m.Avail) + " disponibles"}
	}
	var rx, tx float64
	for _, i := range f.Interfaces {
		rx += i.Rx
		tx += i.Tx
	}
	v["net_rx"] = tile.Value{Points: n.rx.Push(rx), Text: tile.Rate(rx)}
	v["net_tx"] = tile.Value{Points: n.tx.Push(tx), Text: tile.Rate(tx)}
	v["disk_io"] = tile.Value{Text: tile.Rate(f.Disks.Read) + " / " + tile.Rate(f.Disks.Write)}
	return v
}

type tnDate struct {
	Date int64 `json:"$date"`
}

func (d tnDate) Time() time.Time {
	if d.Date == 0 {
		return time.Time{}
	}
	return time.UnixMilli(d.Date)
}

type tnState struct {
	State        string `json:"state"`
	Datetime     tnDate `json:"datetime"`
	LastSnapshot string `json:"last_snapshot"`
	Error        string `json:"error"`
}

type tnSnapTask struct {
	Dataset string  `json:"dataset"`
	Enabled bool    `json:"enabled"`
	State   tnState `json:"state"`
}

func stateCell(s tnState) tile.Cell {
	lvl := "ok"
	switch s.State {
	case "FINISHED", "SUCCESS":
	case "RUNNING", "PENDING", "HOLD":
		lvl = ""
	default:
		lvl = "crit"
	}
	return tile.Cell{Text: strings.ToLower(s.State), Level: lvl}
}

func dateText(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("02/01 15:04")
}

func (n *trueNAS) slow(ctx context.Context, r *rpc) (tile.Values, error) {
	v := tile.Values{}

	var info struct {
		Version string    `json:"version"`
		Load    []float64 `json:"loadavg"`
		Uptime  float64   `json:"uptime_seconds"`
		Cores   int       `json:"cores"`
	}
	if err := r.call(ctx, "system.info", &info); err != nil {
		return nil, err
	}
	v["version"] = tile.Value{Text: info.Version}
	v["uptime"] = tile.Value{Text: tile.Dur(time.Duration(info.Uptime * float64(time.Second)))}
	if len(info.Load) == 3 {
		v["load"] = tile.Value{Text: tile.Num(info.Load[0]) + " · " + tile.Num(info.Load[1]) + " · " + tile.Num(info.Load[2]), Sub: fmt.Sprintf("%d cœurs", info.Cores)}
	}

	var pools []struct {
		Name      string  `json:"name"`
		Status    string  `json:"status"`
		Healthy   bool    `json:"healthy"`
		Size      float64 `json:"size"`
		Allocated float64 `json:"allocated"`
		Free      float64 `json:"free"`
		Scan      *struct {
			Function string  `json:"function"`
			State    string  `json:"state"`
			End      tnDate  `json:"end_time"`
			Errors   int     `json:"errors"`
			Pct      float64 `json:"percentage"`
		} `json:"scan"`
	}
	if err := r.call(ctx, "pool.query", &pools); err == nil {
		var bars []tile.Bar
		var rows [][]tile.Cell
		for _, p := range pools {
			pct := 0.0
			if p.Size > 0 {
				pct = p.Allocated / p.Size * 100
			}
			bars = append(bars, tile.Bar{Label: p.Name, Pct: pct, Text: tile.Pct(pct) + " · " + tile.Bytes(p.Free) + " libres", Level: tile.Level(pct, 80, 90)})
			st := tile.Cell{Text: strings.ToLower(p.Status), Level: "ok"}
			if !p.Healthy {
				st.Level = "crit"
			}
			scrub, scrubLvl := "jamais", "warn"
			if s := p.Scan; s != nil && s.Function == "SCRUB" {
				switch s.State {
				case "SCANNING":
					scrub, scrubLvl = "en cours, "+tile.Pct(s.Pct), ""
				default:
					scrub, scrubLvl = dateText(s.End.Time())+fmt.Sprintf(", %d erreur(s)", s.Errors), "ok"
					if s.Errors > 0 {
						scrubLvl = "crit"
					} else if time.Since(s.End.Time()) > 40*24*time.Hour {
						scrubLvl = "warn"
					}
				}
			}
			rows = append(rows, []tile.Cell{{Text: p.Name}, st, {Text: tile.Bytes(p.Allocated) + " / " + tile.Bytes(p.Size)}, {Text: scrub, Level: scrubLvl}})
		}
		names := make([]string, len(pools))
		for i, p := range pools {
			names[i] = p.Name
		}
		n.mu.Lock()
		n.pools = names
		n.mu.Unlock()
		v["pools"] = tile.Value{Bars: bars}
		v["scrub"] = tile.Value{Head: []string{"Pool", "État", "Utilisé", "Dernier scrub"}, Rows: rows}
	}

	var alerts []struct {
		Level     string `json:"level"`
		Formatted string `json:"formatted"`
		Dismissed bool   `json:"dismissed"`
		Datetime  tnDate `json:"datetime"`
	}
	if err := r.call(ctx, "alert.list", &alerts); err == nil {
		var rows [][]tile.Cell
		worst := ""
		for _, a := range alerts {
			if a.Dismissed {
				continue
			}
			lvl := "warn"
			switch a.Level {
			case "CRITICAL", "ALERT", "EMERGENCY", "ERROR":
				lvl, worst = "crit", "crit"
			case "INFO", "NOTICE":
				lvl = ""
			default:
				if worst == "" {
					worst = "warn"
				}
			}
			rows = append(rows, []tile.Cell{{Text: dateText(a.Datetime.Time())}, {Text: strings.ToLower(a.Level), Level: lvl}, {Text: a.Formatted}})
		}
		if worst == "" {
			worst = "ok"
		}
		n.mu.Lock()
		n.alerts = len(rows)
		n.mu.Unlock()
		v["alerts_n"] = tile.Value{Text: fmt.Sprint(len(rows)), Level: worst}
		v["alerts"] = tile.Value{Head: []string{"Date", "Niveau", "Message"}, Rows: rows, Text: "aucune alerte"}
	}

	var temps map[string]*float64
	if err := r.call(ctx, "disk.temperatures", &temps); err == nil {
		names := make([]string, 0, len(temps))
		for k := range temps {
			names = append(names, k)
		}
		slices.Sort(names)
		var rows [][]tile.Cell
		for _, d := range names {
			if t := temps[d]; t != nil {
				rows = append(rows, []tile.Cell{{Text: d}, {Text: tile.Num(*t) + " °C", Level: tile.Level(*t, 45, 55)}})
			} else {
				rows = append(rows, []tile.Cell{{Text: d}, {Text: "—"}})
			}
		}
		v["temps"] = tile.Value{Head: []string{"Disque", "Température"}, Rows: rows}
	}

	var snaps []tnSnapTask
	if err := r.call(ctx, "pool.snapshottask.query", &snaps); err == nil {
		slices.SortFunc(snaps, func(a, b tnSnapTask) int { return cmp.Compare(a.Dataset, b.Dataset) })
		var rows [][]tile.Cell
		for _, s := range snaps {
			if !s.Enabled {
				continue
			}
			last := s.State.LastSnapshot
			if _, after, ok := strings.Cut(last, "@"); ok {
				last = after
			}
			rows = append(rows, []tile.Cell{{Text: s.Dataset}, stateCell(s.State), {Text: dateText(s.State.Datetime.Time())}, {Text: last}})
		}
		v["snapshots"] = tile.Value{Head: []string{"Dataset", "État", "Date", "Dernier"}, Rows: rows}
	}

	var reps []struct {
		Name    string  `json:"name"`
		Enabled bool    `json:"enabled"`
		State   tnState `json:"state"`
	}
	if err := r.call(ctx, "replication.query", &reps); err == nil {
		var rows [][]tile.Cell
		for _, p := range reps {
			if !p.Enabled {
				continue
			}
			c := stateCell(p.State)
			if p.State.Error != "" {
				c.Text += " : " + p.State.Error
			}
			rows = append(rows, []tile.Cell{{Text: p.Name}, c, {Text: dateText(p.State.Datetime.Time())}})
		}
		v["replication"] = tile.Value{Head: []string{"Tâche", "État", "Date"}, Rows: rows}
	}
	return v, nil
}
