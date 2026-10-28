package integ

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// system : métriques de l'hôte Linux lues dans /proc et /sys, sans agent.
// Dans un conteneur, on monte /proc de l'hôte (options.proc) : le réseau se
// lit alors dans <proc>/1/net/dev, l'espace réseau du processus 1 de l'hôte.
// CPU et réseau toutes les 3 s, le reste toutes les 30 s.
type system struct {
	proc, sys string
	disks     []sysDisk
	now       func() time.Time
	pending   map[string]chan statfsRes // Statfs en cours par point de montage

	mu       sync.Mutex
	cpuTotal int64
	cpuIdle  int64
	cpuN     int
	net      map[string][2]uint64 // octets reçus, envoyés
	netAt    time.Time
	cpu      *tile.Ring
	down, up *tile.Ring
}

type sysDisk struct{ label, path string }

func newSystem(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	s := &system{proc: "/proc", sys: "/sys", now: time.Now, cpu: tile.NewRing(100), down: tile.NewRing(100), up: tile.NewRing(100)}
	if p := t.Options["proc"]; p != "" {
		s.proc = strings.TrimRight(p, "/")
	}
	if p := t.Options["sys"]; p != "" {
		s.sys = strings.TrimRight(p, "/")
	}
	disks := t.Options["disks"]
	if disks == "" {
		disks = "/"
	}
	for _, d := range strings.Split(disks, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		label, path, ok := strings.Cut(d, "=")
		if !ok {
			path = label
		}
		label, path = strings.TrimSpace(label), strings.TrimSpace(path)
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("options.disks : %q n'est pas un chemin absolu", path)
		}
		s.disks = append(s.disks, sysDisk{label, path})
	}
	return s, nil
}

func (s *system) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cpu", "CPU", tile.Spark, tile.OnTile),
		tile.F("ram", "Mémoire", tile.Meter, tile.OnTile),
		tile.F("disks", "Disques", tile.Bars, tile.OnTile),
		tile.F("net_down", "Réseau réception", tile.Spark, tile.OnTile),
		tile.F("net_up", "Réseau envoi", tile.Spark, tile.OnTile),
		tile.F("load", "Charge", tile.Stat, tile.InDetail),
		tile.F("temp", "Température max", tile.Stat, tile.InDetail),
		tile.F("swap", "Swap", tile.Meter, tile.InDetail),
		tile.F("uptime", "En marche depuis", tile.Stat, tile.InDetail),
		tile.F("host", "Hôte", tile.Stat, tile.InDetail),
		tile.F("kernel", "Noyau", tile.Stat, tile.InDetail),
		tile.F("temps", "Capteurs de température", tile.Table, tile.InDetail),
		tile.F("interfaces", "Interfaces réseau", tile.Table, tile.InDetail),
	}
}

func (s *system) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 3*time.Second, func() {
		v, err := s.fast()
		if ctx.Err() == nil && (len(v) > 0 || err != nil) {
			e[0](v, err)
		}
	})
	every(ctx, 30*time.Second, func() {
		if v := s.slow(); ctx.Err() == nil && len(v) > 0 {
			e[1](v, nil)
		}
	})
}

func (s *system) read(rel string) (string, error) {
	b, err := os.ReadFile(filepath.Join(s.proc, rel))
	return string(b), err
}

// fast : CPU (delta de /proc/stat) et débits (delta de net/dev). /proc/stat
// illisible = options.proc fausse : la tuile passe en erreur.
func (s *system) fast() (tile.Values, error) {
	stat, err := s.read("stat")
	if err != nil {
		return nil, fmt.Errorf("lecture de %s/stat impossible (options.proc ?)", s.proc)
	}
	netdev, nerr := s.read("1/net/dev")
	if nerr != nil {
		netdev, nerr = s.read("net/dev")
	}
	now := s.now()
	v := tile.Values{}

	s.mu.Lock()
	defer s.mu.Unlock()
	if total, idle, n, ok := sysCPU(stat); ok {
		dt, di := total-s.cpuTotal, idle-s.cpuIdle
		first := s.cpuTotal == 0
		s.cpuTotal, s.cpuIdle, s.cpuN = total, idle, n
		// iowait peut reculer légèrement : idle borné, pas de point si le total recule.
		if !first && dt > 0 {
			di = max(0, min(di, dt))
			pct := float64(dt-di) / float64(dt) * 100
			v["cpu"] = tile.Value{Text: tile.Pct(pct), Level: tile.Level(pct, 80, 95), Points: s.cpu.Push(pct), Sub: fmt.Sprintf("%d cœurs", n)}
		}
	}
	if nerr == nil {
		s.netRates(netdev, now, v)
	}
	return v, nil
}

// sysCPU : total et inactif (idle + iowait) en ticks, nombre de cœurs.
func sysCPU(stat string) (total, idle int64, n int, ok bool) {
	for _, line := range strings.Split(stat, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			n++
			continue
		}
		// user nice system idle iowait irq softirq steal ; guest est déjà dans user.
		for i := 1; i < len(f) && i <= 8; i++ {
			x, err := strconv.ParseInt(f[i], 10, 64)
			if err != nil {
				return 0, 0, 0, false
			}
			total += x
			if i == 4 || i == 5 {
				idle += x
			}
		}
		ok = len(f) >= 5
	}
	return total, idle, n, ok
}

func sysIgnoredIface(name string) bool {
	if name == "lo" {
		return true
	}
	for _, p := range []string{"veth", "docker", "br-"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// netRates calcule les débits par interface. Une interface nouvelle ou dont un
// compteur recule (redémarrage, débordement) est ignorée pour ce tour.
func (s *system) netRates(netdev string, now time.Time, v tile.Values) {
	cur := map[string][2]uint64{}
	for _, line := range strings.Split(netdev, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if sysIgnoredIface(name) || len(f) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(f[0], 10, 64)
		tx, err2 := strconv.ParseUint(f[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		cur[name] = [2]uint64{rx, tx}
	}
	prev, prevAt := s.net, s.netAt
	s.net, s.netAt = cur, now
	dt := now.Sub(prevAt).Seconds()
	if prevAt.IsZero() || dt <= 0 {
		return
	}
	type rate struct {
		name   string
		rx, tx float64
		c      [2]uint64
	}
	var rates []rate
	var sumRx, sumTx float64
	for name, c := range cur {
		p, ok := prev[name]
		if !ok || c[0] < p[0] || c[1] < p[1] {
			continue
		}
		r := rate{name, float64(c[0]-p[0]) / dt, float64(c[1]-p[1]) / dt, c}
		rates = append(rates, r)
		sumRx += r.rx
		sumTx += r.tx
	}
	if len(rates) == 0 {
		return
	}
	v["net_down"] = tile.Value{Text: tile.Rate(sumRx), Points: s.down.Push(sumRx)}
	v["net_up"] = tile.Value{Text: tile.Rate(sumTx), Points: s.up.Push(sumTx)}
	sort.Slice(rates, func(i, j int) bool {
		a, b := rates[i].rx+rates[i].tx, rates[j].rx+rates[j].tx
		if a != b {
			return a > b
		}
		return rates[i].name < rates[j].name
	})
	rows := [][]tile.Cell{}
	for _, r := range rates[:min(len(rates), 15)] {
		rows = append(rows, []tile.Cell{{Text: r.name}, {Text: tile.Rate(r.rx)}, {Text: tile.Rate(r.tx)}, {Text: tile.Bytes(float64(r.c[0]))}, {Text: tile.Bytes(float64(r.c[1]))}})
	}
	v["interfaces"] = tile.Value{Head: []string{"Interface", "Réception", "Envoi", "Reçu", "Envoyé"}, Rows: rows}
}

// slow : mémoire, charge, durée de marche, températures, disques, hôte.
// Chaque lecture est facultative.
func (s *system) slow() tile.Values {
	v := tile.Values{}
	if mi, err := s.read("meminfo"); err == nil {
		m := sysMeminfo(mi)
		if tot := m["MemTotal"]; tot > 0 {
			avail, ok := m["MemAvailable"]
			if !ok { // noyaux antérieurs à 3.14
				avail = m["MemFree"] + m["Buffers"] + m["Cached"] + m["SReclaimable"]
			}
			used := max(0, tot-avail)
			pct := used / tot * 100
			v["ram"] = tile.Value{Pct: pct, Text: tile.Bytes(used) + " / " + tile.Bytes(tot), Level: tile.Level(pct, 85, 95)}
		}
		if tot := m["SwapTotal"]; tot > 0 {
			used := max(0, tot-m["SwapFree"])
			pct := used / tot * 100
			v["swap"] = tile.Value{Pct: pct, Text: tile.Bytes(used) + " / " + tile.Bytes(tot), Level: tile.Level(pct, 50, 80)}
		}
	}
	if la, err := s.read("loadavg"); err == nil {
		if f := strings.Fields(la); len(f) >= 3 {
			l1, e1 := strconv.ParseFloat(f[0], 64)
			l5, e5 := strconv.ParseFloat(f[1], 64)
			l15, e15 := strconv.ParseFloat(f[2], 64)
			if e1 == nil && e5 == nil && e15 == nil {
				lv := tile.Value{Text: tile.Num(l1) + " · " + tile.Num(l5) + " · " + tile.Num(l15), Sub: "1, 5 et 15 min"}
				s.mu.Lock()
				n := s.cpuN
				s.mu.Unlock()
				if n > 0 {
					lv.Level = tile.Level(l1/float64(n), 1, 2)
					lv.Sub += fmt.Sprintf(" · %d cœurs", n)
				}
				v["load"] = lv
			}
		}
	}
	if up, err := s.read("uptime"); err == nil {
		if f := strings.Fields(up); len(f) > 0 {
			if sec, err := strconv.ParseFloat(f[0], 64); err == nil {
				v["uptime"] = tile.Value{Text: tile.Dur(time.Duration(sec * float64(time.Second)))}
			}
		}
	}
	if h, err := s.read("sys/kernel/hostname"); err == nil && strings.TrimSpace(h) != "" {
		v["host"] = tile.Value{Text: strings.TrimSpace(h)}
	}
	if k, err := s.read("sys/kernel/osrelease"); err == nil && strings.TrimSpace(k) != "" {
		v["kernel"] = tile.Value{Text: strings.TrimSpace(k)}
	}
	s.temps(v)
	s.diskBars(v)
	return v
}

// sysMeminfo : valeurs de /proc/meminfo en octets.
func sysMeminfo(s string) map[string]float64 {
	m := map[string]float64{}
	for _, line := range strings.Split(s, "\n") {
		k, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		x, err := strconv.ParseFloat(f[0], 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			x *= 1024
		}
		m[k] = x
	}
	return m
}

type sysSensor struct {
	label      string
	c          float64
	warn, crit float64
}

// sysMilli lit un fichier en millidegrés ; ok=false si absent ou illisible.
func sysMilli(path string) (float64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return x / 1000, err == nil
}

func sysLine(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

// temps lit hwmon ; thermal_zone seulement si hwmon ne donne rien (les deux
// décrivent souvent les mêmes capteurs).
func (s *system) temps(v tile.Values) {
	var list []sysSensor
	dirs, _ := filepath.Glob(filepath.Join(s.sys, "class/hwmon/hwmon*"))
	names := map[string]int{}
	for _, d := range dirs {
		names[sysLine(filepath.Join(d, "name"))]++
	}
	for _, d := range dirs {
		name := sysLine(filepath.Join(d, "name"))
		if name == "" {
			name = filepath.Base(d)
		}
		if names[name] > 1 { // plusieurs disques ou nvme : on précise le périphérique (sda, nvme0)
			dev := filepath.Base(d)
			if blk, _ := filepath.Glob(filepath.Join(d, "device/block/*")); len(blk) > 0 {
				dev = filepath.Base(blk[0])
			} else if real, err := filepath.EvalSymlinks(filepath.Join(d, "device")); err == nil {
				dev = filepath.Base(real)
			}
			name += " " + dev
		}
		inputs, _ := filepath.Glob(filepath.Join(d, "temp*_input"))
		for _, in := range inputs {
			c, ok := sysMilli(in)
			if !ok {
				continue
			}
			base := strings.TrimSuffix(in, "_input")
			label := sysLine(base + "_label")
			if label == "" {
				label = strings.TrimPrefix(filepath.Base(base), "temp")
				if len(inputs) == 1 {
					label = ""
				}
			}
			sn := sysSensor{label: strings.TrimSpace(name + " " + label), c: c, warn: 80, crit: 90}
			if x, ok := sysMilli(base + "_crit"); ok && x > 0 {
				sn.crit, sn.warn = x, x-10
			}
			if x, ok := sysMilli(base + "_max"); ok && x > 0 && x < sn.crit {
				sn.warn = x
			}
			list = append(list, sn)
		}
	}
	if len(list) == 0 {
		zones, _ := filepath.Glob(filepath.Join(s.sys, "class/thermal/thermal_zone*"))
		for _, z := range zones {
			if c, ok := sysMilli(filepath.Join(z, "temp")); ok {
				label := sysLine(filepath.Join(z, "type"))
				if label == "" {
					label = filepath.Base(z)
				}
				list = append(list, sysSensor{label: label, c: c, warn: 80, crit: 90})
			}
		}
	}
	// Capteurs non branchés : valeurs nulles, négatives ou absurdes.
	ok := list[:0]
	for _, sn := range list {
		if sn.c > 0 && sn.c < 200 {
			ok = append(ok, sn)
		}
	}
	if len(ok) == 0 {
		return
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].c > ok[j].c })
	deg := func(c float64) string { return tile.Num(c) + " °C" }
	hot := ok[0]
	v["temp"] = tile.Value{Text: deg(hot.c), Sub: hot.label, Level: tile.Level(hot.c, hot.warn, hot.crit)}
	rows := [][]tile.Cell{}
	for _, sn := range ok[:min(len(ok), 15)] {
		rows = append(rows, []tile.Cell{{Text: sn.label}, {Text: deg(sn.c), Level: tile.Level(sn.c, sn.warn, sn.crit)}})
	}
	v["temps"] = tile.Value{Head: []string{"Capteur", "Température"}, Rows: rows}
}

var (
	statfs        = syscall.Statfs // remplaçable en test
	statfsTimeout = 2 * time.Second
)

type statfsRes struct {
	st  syscall.Statfs_t
	err error
}

// statfs appelle Statfs avec un délai : un montage NFS/SMB bloqué ne doit pas
// figer la boucle. Un appel encore en cours pour ce point est attendu, pas relancé.
// Appelé par la seule boucle lente, donc pending n'a pas besoin de verrou.
func (s *system) statfs(path string) (st syscall.Statfs_t, err error, late bool) {
	ch := s.pending[path]
	if ch == nil {
		ch = make(chan statfsRes, 1)
		go func() {
			var r statfsRes
			r.err = statfs(path, &r.st)
			ch <- r
		}()
		if s.pending == nil {
			s.pending = map[string]chan statfsRes{}
		}
		s.pending[path] = ch
	}
	select {
	case r := <-ch:
		delete(s.pending, path)
		return r.st, r.err, false
	case <-time.After(statfsTimeout):
		return st, nil, true
	}
}

// diskBars : occupation des points de montage, comme df (part réservée à root
// exclue du disponible).
func (s *system) diskBars(v tile.Values) {
	var bars []tile.Bar
	for _, d := range s.disks {
		st, err, late := s.statfs(d.path)
		if late {
			bars = append(bars, tile.Bar{Label: d.label, Text: "bloqué", Level: "crit"})
			continue
		}
		if err != nil {
			bars = append(bars, tile.Bar{Label: d.label, Text: "inaccessible", Level: "crit"})
			continue
		}
		bs := float64(st.Bsize)
		total := float64(st.Blocks) * bs
		used := float64(st.Blocks-st.Bfree) * bs
		avail := float64(st.Bavail) * bs
		if total <= 0 || used+avail <= 0 {
			continue
		}
		pct := used / (used + avail) * 100
		bars = append(bars, tile.Bar{Label: d.label, Text: tile.Bytes(used) + " / " + tile.Bytes(total), Pct: pct, Level: tile.Level(pct, 85, 95)})
	}
	if len(bars) > 0 {
		v["disks"] = tile.Value{Bars: bars}
	}
}
