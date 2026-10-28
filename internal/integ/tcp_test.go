package integ

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestTCP(t *testing.T) {
	open, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	go func() {
		for {
			c, err := open.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	// Port libéré juste après l'écoute : connexion refusée.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().String()
	l.Close()

	src, err := newTCP(config.Tile{Options: map[string]string{"targets": " web=" + open.Addr().String() + ", ssh=" + closed + ",, " + open.Addr().String()}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := src.(*tcpCheck).check(context.Background())
	if d := netNeed(t, v, "down"); d.Text != "1" || d.Level != "crit" || d.Sub != "ssh" {
		t.Errorf("down = %+v", d)
	}
	if u := netNeed(t, v, "up"); u.Text != "2 / 3" || u.Level != "crit" {
		t.Errorf("up = %+v", u)
	}
	if l := netNeed(t, v, "latency"); !strings.HasSuffix(l.Text, " ms") || !strings.Contains(l.Sub, "max") {
		t.Errorf("latency = %+v", l)
	}
	rows := netNeed(t, v, "targets").Rows
	if len(rows) != 3 || rows[0][0].Text != "ssh" || rows[0][2].Text != "refusé" || rows[0][2].Level != "crit" || rows[1][2].Text != "ouvert" || rows[2][0].Text != open.Addr().String() {
		t.Errorf("targets = %v", rows)
	}

	for _, bad := range []string{"", " , ", "web=hote", "web=:80", "web=h:0", "web=h:99999", "web=h:http"} {
		if _, err := newTCP(config.Tile{Options: map[string]string{"targets": bad}}, nil); err == nil {
			t.Errorf("cibles %q acceptées", bad)
		}
	}
	if _, err := newTCP(config.Tile{Options: map[string]string{"targets": "v6=[::1]:22"}}, nil); err != nil {
		t.Errorf("IPv6 refusée : %v", err)
	}
	if len((&tcpCheck{}).Fields()) == 0 {
		t.Error("aucun champ")
	}
}
