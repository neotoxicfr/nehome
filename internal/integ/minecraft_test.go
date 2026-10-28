package integ

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Faux serveur du protocole Server List Ping (wiki.vg « Server List Ping » :
// handshake 0x00, requête 0x00, réponse 0x00 + JSON, ping 0x01 renvoyé tel quel).
func mcFake(t *testing.T, status string) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				readPkt := func() []byte {
					n, err := mcReadVarint(r)
					if err != nil {
						return nil
					}
					b := make([]byte, n)
					io.ReadFull(r, b)
					return b
				}
				hs := readPkt() // handshake
				if len(hs) == 0 || hs[0] != 0x00 {
					return
				}
				readPkt() // requête d'état
				js := append(mcVarint(int32(len(status))), status...)
				c.Write(mcPacket(0x00, js))
				if p := readPkt(); len(p) > 0 { // ping : renvoyer le même contenu
					c.Write(mcPacket(0x01, p[1:]))
				}
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestMinecraft(t *testing.T) {
	port := mcFake(t, `{"version":{"name":"1.21.4","protocol":769},"players":{"max":20,"online":2,"sample":[{"name":"Alex","id":"x"},{"name":"Steve","id":"y"}]},"description":{"text":"","extra":[{"color":"gold","text":"Serveur d"},{"text":"e §ctest"}]}}`)
	src, err := newMinecraft(config.Tile{Options: map[string]string{"host": "127.0.0.1", "port": strconv.Itoa(port)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := src.(*minecraft).poll(t.Context())
	if v["online"].Text != "En ligne" || v["players"].Text != "2 / 20" || v["version"].Text != "1.21.4" || v["motd"].Text != "Serveur de test" || v["latency"].Text == "" || len(v["player_list"].Rows) != 2 {
		t.Fatalf("en ligne : %+v", v)
	}
	// Description en simple chaîne.
	port = mcFake(t, `{"version":{"name":"1.20"},"players":{"max":5,"online":0},"description":"§aSalut"}`)
	src, _ = newMinecraft(config.Tile{URL: "http://127.0.0.1", Options: map[string]string{"port": strconv.Itoa(port)}}, nil)
	if v := src.(*minecraft).poll(t.Context()); v["motd"].Text != "Salut" || v["players"].Text != "0 / 5" {
		t.Fatalf("chaîne : %+v", v)
	}
	// Port fermé : hors ligne, pas de panne de source.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().(*net.TCPAddr).Port
	l.Close()
	src, _ = newMinecraft(config.Tile{Options: map[string]string{"host": "127.0.0.1", "port": strconv.Itoa(closed)}}, nil)
	if v := src.(*minecraft).poll(t.Context()); v["online"].Text != "Hors ligne" || v["online"].Level != "crit" {
		t.Fatalf("hors ligne : %+v", v)
	}
	if _, err := newMinecraft(config.Tile{}, nil); err == nil {
		t.Fatal("hôte manquant accepté")
	}
	if _, err := newMinecraft(config.Tile{Options: map[string]string{"host": "h", "port": "99999"}}, nil); err == nil {
		t.Fatal("port invalide accepté")
	}
}
