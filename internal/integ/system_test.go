package integ

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formats de /proc et /sys : proc(5) et Documentation/hwmon/sysfs-interface
// du noyau Linux. Valeurs factices.

func sysWrite(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const sysNetHead = "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n"

func sysNetLine(name string, rx, tx int) string {
	return "  " + name + ": " + strconv.Itoa(rx) + " 10 0 0 0 0 0 0 " + strconv.Itoa(tx) + " 10 0 0 0 0 0 0\n"
}

func TestSystemFast(t *testing.T) {
	proc := t.TempDir()
	stat := func(user, idle, iowait int) string {
		return "cpu  " + strconv.Itoa(user) + " 0 100 " + strconv.Itoa(idle) + " " + strconv.Itoa(iowait) + " 0 0 0 0 0\ncpu0 1 0 0 1 0 0 0 0 0 0\ncpu1 1 0 0 1 0 0 0 0 0 0\nintr 1\n"
	}
	sysWrite(t, proc, map[string]string{
		"stat":      stat(1000, 5000, 100),
		"1/net/dev": sysNetHead + sysNetLine("lo", 50, 50) + sysNetLine("eth0", 10000, 2000) + sysNetLine("eth1", 900000, 900000) + sysNetLine("veth12ab", 1, 1) + sysNetLine("docker0", 1, 1) + sysNetLine("br-1234", 1, 1),
		"net/dev":   sysNetHead + sysNetLine("eth0", 1, 1), // espace réseau du conteneur : doit être ignoré
	})
	src, err := newSystem(config.Tile{Options: map[string]string{"proc": proc + "/"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := src.(*system)
	at := time.Unix(1000, 0)
	s.now = func() time.Time { return at }

	v, err := s.fast()
	if err != nil || len(v) != 0 {
		t.Fatalf("premier tour : %v %v", v, err)
	}

	// +100 ticks utiles, +25 inactifs (dont iowait) => 75 % ; eth0 +3000/+300 o en 3 s ;
	// eth1 recule (redémarrage) et wg0 apparaît : tous deux ignorés ce tour-ci.
	at = at.Add(3 * time.Second)
	sysWrite(t, proc, map[string]string{
		"stat":      stat(1075, 5020, 105),
		"1/net/dev": sysNetHead + sysNetLine("lo", 99999, 99999) + sysNetLine("eth0", 13000, 2300) + sysNetLine("eth1", 10, 10) + sysNetLine("wg0", 500, 500),
	})
	v, err = s.fast()
	if err != nil {
		t.Fatal(err)
	}
	if c := netNeed(t, v, "cpu"); c.Text != "75 %" || c.Level != "ok" || len(c.Points) != 1 || c.Sub != "2 cœurs" {
		t.Errorf("cpu = %+v", c)
	}
	if d := netNeed(t, v, "net_down"); d.Text != "1 ko/s" {
		t.Errorf("net_down = %+v", d)
	}
	if u := netNeed(t, v, "net_up"); u.Text != "100 o/s" {
		t.Errorf("net_up = %+v", u)
	}
	if r := netNeed(t, v, "interfaces").Rows; len(r) != 1 || r[0][0].Text != "eth0" {
		t.Errorf("interfaces = %v", r)
	}

	// Compteurs CPU qui reculent (nouvelle lecture après redémarrage) : pas de point.
	at = at.Add(3 * time.Second)
	sysWrite(t, proc, map[string]string{"stat": stat(10, 10, 0)})
	if v, err = s.fast(); err != nil || v["cpu"].Text != "" {
		t.Errorf("recul CPU : %+v %v", v["cpu"], err)
	}
	// wg0 et eth1 ont désormais une référence.
	if r := v["interfaces"].Rows; len(r) != 3 {
		t.Errorf("interfaces après apparition = %v", r)
	}

	// options.proc fausse : la tuile passe en erreur.
	bad, _ := newSystem(config.Tile{Options: map[string]string{"proc": filepath.Join(proc, "absent")}}, nil)
	if _, err := bad.(*system).fast(); err == nil {
		t.Error("erreur attendue sans /proc/stat")
	}
}

func TestSystemNetFallback(t *testing.T) {
	proc := t.TempDir()
	sysWrite(t, proc, map[string]string{"stat": "cpu  1 0 0 1 0 0 0 0\n", "net/dev": sysNetHead + sysNetLine("ens3", 0, 0)})
	src, _ := newSystem(config.Tile{Options: map[string]string{"proc": proc}}, nil)
	s := src.(*system)
	s.fast()
	if _, ok := s.net["ens3"]; !ok {
		t.Errorf("net/dev de repli non lu : %v", s.net)
	}
}

func TestSystemSlow(t *testing.T) {
	proc, sys, disk := t.TempDir(), t.TempDir(), t.TempDir()
	sysWrite(t, proc, map[string]string{
		"stat":                 "cpu  1 0 0 1 0 0 0 0\ncpu0 1 0 0 1\ncpu1 1 0 0 1\ncpu2 1 0 0 1\ncpu3 1 0 0 1\n",
		"meminfo":              "MemTotal:       16000000 kB\nMemFree:         1000000 kB\nMemAvailable:    4000000 kB\nBuffers:          100000 kB\nSwapTotal:       2000000 kB\nSwapFree:        1500000 kB\n",
		"loadavg":              "4.50 2.00 1.25 3/812 12345\n",
		"uptime":               "93784.21 300000.00\n",
		"sys/kernel/hostname":  "nas\n",
		"sys/kernel/osrelease": "6.12.105-production+truenas\n",
	})
	sysWrite(t, sys, map[string]string{
		"class/hwmon/hwmon0/name":          "coretemp\n",
		"class/hwmon/hwmon0/temp1_input":   "52000\n",
		"class/hwmon/hwmon0/temp1_label":   "Package id 0\n",
		"class/hwmon/hwmon0/temp1_crit":    "100000\n",
		"class/hwmon/hwmon0/temp2_input":   "48000\n",
		"class/hwmon/hwmon1/name":          "nvme\n",
		"class/hwmon/hwmon1/temp1_input":   "71850\n",
		"class/hwmon/hwmon1/temp1_label":   "Composite\n",
		"class/hwmon/hwmon1/temp1_max":     "70850\n",
		"class/hwmon/hwmon1/temp1_crit":    "84850\n",
		"class/hwmon/hwmon2/name":          "nvme\n",
		"class/hwmon/hwmon2/temp1_input":   "40000\n",
		"class/hwmon/hwmon3/name":          "bidon\n",
		"class/hwmon/hwmon3/temp1_input":   "-273000\n",
		"class/thermal/thermal_zone0/type": "acpitz\n",
		"class/thermal/thermal_zone0/temp": "99000\n",
	})
	src, err := newSystem(config.Tile{Options: map[string]string{"proc": proc, "sys": sys, "disks": "Données=" + disk + ", /chemin/absent"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := src.(*system)
	s.fast() // nombre de cœurs pour la charge
	v := s.slow()

	if r := netNeed(t, v, "ram"); r.Pct != 75 || r.Text != "12,3 Go / 16,4 Go" {
		t.Errorf("ram = %+v", r)
	}
	if w := netNeed(t, v, "swap"); w.Pct != 25 || w.Level != "ok" {
		t.Errorf("swap = %+v", w)
	}
	if l := netNeed(t, v, "load"); l.Text != "4,5 · 2 · 1,2" || l.Level != "warn" || !strings.Contains(l.Sub, "4 cœurs") {
		t.Errorf("load = %+v", l)
	}
	if u := netNeed(t, v, "uptime"); u.Text != "1 j 2 h" {
		t.Errorf("uptime = %+v", u)
	}
	if netNeed(t, v, "host").Text != "nas" || netNeed(t, v, "kernel").Text != "6.12.105-production+truenas" {
		t.Errorf("host/kernel = %+v %+v", v["host"], v["kernel"])
	}
	// nvme au-dessus de son seuil max (70,85 °C) : warn ; thermal_zone ignorée.
	if tp := netNeed(t, v, "temp"); tp.Text != "71,8 °C" || tp.Level != "warn" || tp.Sub != "nvme hwmon1 Composite" {
		t.Errorf("temp = %+v", tp)
	}
	rows := netNeed(t, v, "temps").Rows
	if len(rows) != 4 || rows[3][0].Text != "nvme hwmon2" || rows[2][0].Text != "coretemp 2" {
		t.Errorf("temps = %v", rows)
	}
	bars := netNeed(t, v, "disks").Bars
	if len(bars) != 2 || bars[0].Label != "Données" || bars[0].Pct <= 0 || bars[0].Pct > 100 || bars[1].Label != "/chemin/absent" || bars[1].Level != "crit" {
		t.Errorf("disks = %+v", bars)
	}
}

func TestSystemFallbacks(t *testing.T) {
	proc, sys := t.TempDir(), t.TempDir()
	// Noyau ancien sans MemAvailable, pas de swap ; pas de hwmon : thermal_zone.
	sysWrite(t, proc, map[string]string{
		"meminfo": "MemTotal: 1000 kB\nMemFree: 100 kB\nBuffers: 100 kB\nCached: 200 kB\nSReclaimable: 100 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
	})
	sysWrite(t, sys, map[string]string{
		"class/thermal/thermal_zone0/type": "cpu-thermal\n",
		"class/thermal/thermal_zone0/temp": "45678\n",
	})
	src, _ := newSystem(config.Tile{Options: map[string]string{"proc": proc, "sys": sys}}, nil)
	v := src.(*system).slow()
	if r := netNeed(t, v, "ram"); r.Pct != 50 {
		t.Errorf("ram sans MemAvailable = %+v", r)
	}
	if _, ok := v["swap"]; ok {
		t.Error("swap absent attendu")
	}
	if tp := netNeed(t, v, "temp"); tp.Text != "45,7 °C" || tp.Sub != "cpu-thermal" || tp.Level != "ok" {
		t.Errorf("temp = %+v", tp)
	}
	if _, ok := v["load"]; ok {
		t.Error("load absent attendu")
	}
	if _, err := newSystem(config.Tile{Options: map[string]string{"disks": "relatif"}}, nil); err == nil {
		t.Error("chemin relatif refusé attendu")
	}
	if len((&system{}).Fields()) == 0 {
		t.Error("aucun champ")
	}
	var _ tile.Source = &system{}
}

// Un montage bloqué affiche « bloqué » sans figer la boucle ni empiler une
// goroutine par passage.
func TestSystemStatfsBloque(t *testing.T) {
	old, oldT := statfs, statfsTimeout
	defer func() { statfs, statfsTimeout = old, oldT }()
	statfsTimeout = 20 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	statfs = func(string, *syscall.Statfs_t) error { calls.Add(1); <-release; return nil }
	s := &system{disks: []sysDisk{{label: "nfs", path: "/mnt/nfs"}}}
	for range 3 {
		v := tile.Values{}
		s.diskBars(v)
		if b := v["disks"].Bars; len(b) != 1 || b[0].Text != "bloqué" || b[0].Level != "crit" {
			t.Fatalf("bars = %+v", b)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("Statfs lancé %d fois, 1 attendu", n)
	}
}
