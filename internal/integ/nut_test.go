package integ

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponse réelle d'upsd 2.8.0 (CyberPower CP900EPFCLCD, usbhid-ups), allégée,
// numéro de série retiré.
const nutReply = `BEGIN LIST VAR ups
VAR ups battery.charge "100"
VAR ups battery.charge.low "5"
VAR ups battery.charge.warning "20"
VAR ups battery.runtime "1890"
VAR ups battery.runtime.low "600"
VAR ups battery.voltage "13.1"
VAR ups device.mfr "CPS"
VAR ups device.model "CP900EPFCLCD "
VAR ups input.voltage "240.0"
VAR ups output.voltage "240.0"
VAR ups ups.load "20"
VAR ups ups.realpower.nominal "540"
VAR ups ups.status "OL"
VAR ups ups.test.result "No test initiated"
END LIST VAR ups
OK Goodbye
`

// fakeUPSD : faux upsd qui vérifie la commande reçue et répond reply.
func fakeUPSD(t *testing.T, reply string) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				if l, _ := r.ReadString('\n'); l != "LIST VAR ups\n" {
					c.Write([]byte("ERR INVALID-ARGUMENT\n"))
					return
				}
				c.Write([]byte(reply))
			}()
		}
	}()
	return ln.Addr().String()
}

func TestNUT(t *testing.T) {
	host, port, _ := net.SplitHostPort(fakeUPSD(t, nutReply))
	src, err := newNUT(config.Tile{Options: map[string]string{"host": host, "port": port}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := src.(*nut).list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v := nutValues(vars)
	for id, want := range map[string]string{"status": "sur secteur", "charge": "100 %", "runtime": "31 min", "load": "20 %", "input": "240 V", "battv": "13,1 V", "model": "CPS CP900EPFCLCD"} {
		if v[id].Text != want {
			t.Errorf("%s = %q, attendu %q", id, v[id].Text, want)
		}
	}
	if v["status"].Level != "ok" || v["load"].Sub != "≈ 108 W sur 540" {
		t.Errorf("état %+v, charge %+v", v["status"], v["load"])
	}
}

func TestNUTOnBattery(t *testing.T) {
	v := nutValues(map[string]string{"ups.status": "OB DISCHRG LB", "battery.charge": "4", "battery.charge.low": "5", "battery.runtime": "300", "battery.runtime.low": "600"})
	if v["status"].Text != "batterie faible" || v["status"].Level != "crit" || !strings.Contains(v["status"].Sub, "sur batterie") {
		t.Errorf("état %+v", v["status"])
	}
	if v["charge"].Level != "crit" || v["runtime"].Level != "crit" {
		t.Errorf("batterie %+v, autonomie %+v", v["charge"], v["runtime"])
	}
}

func TestNUTErrors(t *testing.T) {
	host, port, _ := net.SplitHostPort(fakeUPSD(t, "ERR UNKNOWN-UPS\n"))
	src, _ := newNUT(config.Tile{Options: map[string]string{"host": host, "port": port}}, nil)
	if _, err := src.(*nut).list(context.Background()); err == nil || !strings.Contains(err.Error(), "UNKNOWN-UPS") {
		t.Errorf("erreur %v", err)
	}
	for _, o := range []map[string]string{{}, {"host": "x", "port": "0"}, {"host": "x", "ups": "a b"}} {
		if _, err := newNUT(config.Tile{Options: o}, nil); err == nil {
			t.Errorf("options %v acceptées", o)
		}
	}
}
