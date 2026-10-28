package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// --- aides partagées par les intégrations « infrastructure » (préfixe inf) ---

// infNum accepte un nombre JSON ou une chaîne numérique (OpenMediaVault, OPNsense,
// DSM renvoient souvent les tailles en chaînes). Une valeur illisible vaut 0.
type infNum float64

func (n *infNum) UnmarshalJSON(b []byte) error {
	f, _ := strconv.ParseFloat(strings.Trim(string(b), `"`), 64)
	*n = infNum(f)
	return nil
}

// infPost envoie un corps (JSON ou formulaire) et décode la réponse.
func infPost(ctx context.Context, hc *http.Client, url, ctype string, body []byte, out any, set func(*http.Request)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	if set != nil {
		set(req)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return statusError{resp.StatusCode, redact(url)}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

func infPct(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}

// infLvl : niveau d'une jauge d'occupation (70 % / 90 %).
func infLvl(p float64) string { return tile.Level(p, 70, 90) }

func infRows(r [][]tile.Cell) [][]tile.Cell {
	if len(r) > 15 {
		return r[:15]
	}
	return r
}

func infBytes(f float64) string { return tile.Bytes(f) }

// --- Proxmox VE ---

// Proxmox VE : API REST /api2/json, jeton d'API en lecture seule
// (rôle PVEAuditor). Une seule requête /cluster/resources donne nœuds, VM,
// LXC et stockages.
type proxmox struct {
	url, token string
	hc         *http.Client
}

func newProxmox(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	tok, err := secret("token")
	if err != nil {
		return nil, err
	}
	return &proxmox{url: strings.TrimRight(t.URL, "/") + "/api2/json", token: strings.TrimSpace(tok), hc: httpClient(t.InsecureTLS, false)}, nil
}

func (p *proxmox) Fields() []tile.Field {
	return []tile.Field{
		tile.F("guests", "VM et conteneurs en marche", tile.Stat, tile.OnTile),
		tile.F("cpu", "Processeur (cluster)", tile.Meter, tile.OnTile),
		tile.F("ram", "Mémoire (cluster)", tile.Meter, tile.OnTile),
		tile.F("failed", "Tâches en échec", tile.Stat, tile.OnTile),
		tile.F("nodes", "Nœuds", tile.Table, tile.InDetail),
		tile.F("vms", "VM et conteneurs", tile.Table, tile.InDetail),
		tile.F("storages", "Stockages", tile.Bars, tile.InDetail),
		tile.F("failedtasks", "Tâches récentes en échec", tile.Table, tile.InDetail),
	}
}

func (p *proxmox) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, p.hc, p.url+path, out, func(r *http.Request) {
		r.Header.Set("Authorization", "PVEAPIToken="+p.token)
	})
}

func (p *proxmox) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 5*time.Minute, func() {
		if v := p.tasks(ctx); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, 30*time.Second, func() {
		v, err := p.resources(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

type pveRes struct {
	Type    string  `json:"type"`
	Node    string  `json:"node"`
	Name    string  `json:"name"`
	Storage string  `json:"storage"`
	Shared  int     `json:"shared"`
	VMID    int     `json:"vmid"`
	Status  string  `json:"status"`
	CPU     float64 `json:"cpu"`
	MaxCPU  float64 `json:"maxcpu"`
	Mem     float64 `json:"mem"`
	MaxMem  float64 `json:"maxmem"`
	Disk    float64 `json:"disk"`
	MaxDisk float64 `json:"maxdisk"`
	Uptime  float64 `json:"uptime"`
	Tmpl    int     `json:"template"`
}

func (p *proxmox) resources(ctx context.Context) (tile.Values, error) {
	var r struct {
		Data []pveRes `json:"data"`
	}
	if err := p.get(ctx, "/cluster/resources", &r); err != nil {
		return nil, err
	}
	var nodes, vms [][]tile.Cell
	var bars []tile.Bar
	var cpuUsed, cpuMax, mem, memMax float64
	run, total, nvm, nct, nodeUp, nodeAll := 0, 0, 0, 0, 0, 0
	seen := map[string]bool{}
	var local []int // barres de stockages non partagés
	for _, x := range r.Data {
		switch x.Type {
		case "node":
			nodeAll++
			lv := "ok"
			up := "hors ligne"
			if x.Status == "online" {
				nodeUp++
				cpuUsed += x.CPU * x.MaxCPU
				cpuMax += x.MaxCPU
				mem += x.Mem
				memMax += x.MaxMem
				up = tile.Dur(time.Duration(x.Uptime) * time.Second)
			} else {
				lv = "crit"
			}
			nodes = append(nodes, []tile.Cell{{Text: x.Node, Level: lv}, {Text: tile.Pct(x.CPU * 100), Level: infLvl(x.CPU * 100)},
				{Text: tile.Pct(infPct(x.Mem, x.MaxMem)), Level: infLvl(infPct(x.Mem, x.MaxMem))},
				{Text: tile.Pct(infPct(x.Disk, x.MaxDisk)), Level: infLvl(infPct(x.Disk, x.MaxDisk))}, {Text: up}})
		case "qemu", "lxc":
			if x.Tmpl == 1 {
				continue
			}
			total++
			lv := ""
			if x.Status == "running" {
				run++
				if x.Type == "qemu" {
					nvm++
				} else {
					nct++
				}
				lv = "ok"
			}
			kind := "VM"
			if x.Type == "lxc" {
				kind = "LXC"
			}
			cpu, ram := "", ""
			if x.Status == "running" {
				cpu = tile.Pct(x.CPU * 100)
				ram = infBytes(x.Mem) + " / " + infBytes(x.MaxMem)
			}
			vms = append(vms, []tile.Cell{{Text: strconv.Itoa(x.VMID)}, {Text: x.Name, Level: lv}, {Text: kind}, {Text: x.Node}, {Text: x.Status}, {Text: cpu}, {Text: ram}})
		case "storage":
			// un stockage partagé apparaît une fois par nœud ; les autres sont propres à leur nœud
			key := x.Node + "/" + x.Storage
			if x.Shared == 1 {
				key = x.Storage
			}
			if x.MaxDisk <= 0 || x.Status != "available" || seen[key] {
				continue
			}
			seen[key] = true
			pc := infPct(x.Disk, x.MaxDisk)
			label := x.Storage
			if x.Shared != 1 {
				local = append(local, len(bars))
				label += " (" + x.Node + ")"
			}
			bars = append(bars, tile.Bar{Label: label, Pct: pc, Level: infLvl(pc), Text: infBytes(x.Disk) + " / " + infBytes(x.MaxDisk)})
		}
	}
	if nodeAll < 2 { // un seul nœud : le nom du nœud n'apporte rien
		for _, i := range local {
			bars[i].Label = bars[i].Label[:strings.LastIndex(bars[i].Label, " (")]
		}
	}
	sort.SliceStable(vms, func(i, j int) bool { return vms[i][4].Text == "running" && vms[j][4].Text != "running" })
	cpu := infPct(cpuUsed, cpuMax)
	ramP := infPct(mem, memMax)
	v := tile.Values{
		"guests": {Text: fmt.Sprintf("%d / %d", run, total), Sub: fmt.Sprintf("%d VM · %d LXC en marche", nvm, nct), Level: map[bool]string{true: "ok", false: "warn"}[run == total]},
		"nodes":  {Head: []string{"Nœud", "CPU", "RAM", "Disque", "Uptime"}, Rows: infRows(nodes)},
		"vms":    {Head: []string{"ID", "Nom", "Type", "Nœud", "État", "CPU", "RAM"}, Rows: infRows(vms)},
	}
	if nodeUp > 0 {
		v["cpu"] = tile.Value{Pct: cpu, Text: tile.Pct(cpu), Level: infLvl(cpu), Sub: fmt.Sprintf("%d/%d nœuds en ligne", nodeUp, nodeAll)}
		v["ram"] = tile.Value{Pct: ramP, Text: tile.Pct(ramP), Level: infLvl(ramP), Sub: infBytes(mem) + " / " + infBytes(memMax)}
	}
	if len(bars) > 0 {
		v["storages"] = tile.Value{Bars: bars}
	}
	return v, nil
}

// tasks : tâches récentes (/cluster/tasks) dont l'état n'est pas « OK ».
func (p *proxmox) tasks(ctx context.Context) tile.Values {
	var r struct {
		Data []struct {
			Node   string  `json:"node"`
			Type   string  `json:"type"`
			User   string  `json:"user"`
			ID     string  `json:"id"`
			Status string  `json:"status"`
			End    float64 `json:"endtime"`
		} `json:"data"`
	}
	if p.get(ctx, "/cluster/tasks", &r) != nil {
		return nil
	}
	tab := tile.Value{Head: []string{"Tâche", "Nœud", "Erreur", "Quand"}}
	n := 0
	for _, x := range r.Data {
		// status vide = en cours ; « OK » et « WARNINGS: n » ne sont pas des échecs
		if x.Status == "" || x.Status == "OK" || strings.HasPrefix(x.Status, "WARNINGS") {
			continue
		}
		n++
		if len(tab.Rows) < 15 {
			name := x.Type
			if x.ID != "" {
				name += " " + x.ID
			}
			tab.Rows = append(tab.Rows, []tile.Cell{{Text: name}, {Text: x.Node}, {Text: x.Status, Level: "crit"}, {Text: tile.Ago(time.Unix(int64(x.End), 0))}})
		}
	}
	f := tile.Value{Text: "aucune", Level: "ok"}
	if n > 0 {
		f = tile.Value{Text: strconv.Itoa(n), Level: "crit", Sub: "tâches récentes"}
	}
	return tile.Values{"failed": f, "failedtasks": tab}
}
