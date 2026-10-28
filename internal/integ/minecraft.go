package integ

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Minecraft Java : Server List Ping en TCP natif (handshake, requête d'état,
// ping), sans mod ni plugin. Options : host (sinon l'hôte de url), port (25565
// par défaut). Aucun secret. Un serveur injoignable n'est pas une panne de la
// source : il s'affiche « Hors ligne ». Relevé toutes les 30 s.
type minecraft struct {
	host string
	port int
	hist *tile.Ring
}

func newMinecraft(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	m := &minecraft{host: t.Options["host"], port: 25565, hist: tile.NewRing(60)}
	if p := t.Options["port"]; p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("options.port invalide : %q", p)
		}
		m.port = n
	}
	if m.host == "" {
		if u, err := url.Parse(t.URL); err == nil {
			m.host = u.Hostname()
		}
	}
	if m.host == "" {
		return nil, errors.New("options.host manquant (ou url)")
	}
	return m, nil
}

func (m *minecraft) Fields() []tile.Field {
	return []tile.Field{
		tile.F("online", "État", tile.Stat, tile.OnTile),
		tile.F("players", "Joueurs", tile.Stat, tile.OnTile),
		tile.F("version", "Version", tile.Stat, tile.OnTile),
		tile.F("latency", "Latence", tile.Stat, tile.InDetail),
		tile.F("motd", "Message du jour", tile.Stat, tile.InDetail),
		tile.F("player_list", "Joueurs connectés", tile.Table, tile.InDetail),
		tile.F("history", "Joueurs (historique)", tile.Spark, tile.InDetail),
	}
}

func (m *minecraft) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v := m.poll(ctx)
		if ctx.Err() == nil {
			emit(v, nil)
		}
	})
}

func mcVarint(n int32) []byte {
	var b []byte
	u := uint32(n)
	for {
		if u < 0x80 {
			return append(b, byte(u))
		}
		b = append(b, byte(u&0x7f|0x80))
		u >>= 7
	}
}

func mcReadVarint(r io.ByteReader) (int32, error) {
	var n uint32
	for i := 0; i < 5; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		n |= uint32(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return int32(n), nil
		}
	}
	return 0, errors.New("varint trop long")
}

func mcPacket(id byte, data ...[]byte) []byte {
	body := []byte{id}
	for _, d := range data {
		body = append(body, d...)
	}
	return append(mcVarint(int32(len(body))), body...)
}

type mcStatus struct {
	Version struct {
		Name string
	}
	Players struct {
		Max, Online int
		Sample      []struct{ Name string }
	}
	Description json.RawMessage
}

// ping ouvre la connexion, lit l'état, puis mesure l'aller-retour du ping.
func (m *minecraft) ping(ctx context.Context) (st mcStatus, lat time.Duration, err error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(m.host, strconv.Itoa(m.port)))
	if err != nil {
		return st, 0, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	r := bufio.NewReader(conn)

	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, uint16(m.port))
	host := append(mcVarint(int32(len(m.host))), m.host...)
	// Handshake : protocole (valeur quelconque, l'état répond pour toute version), hôte, port, état suivant 1.
	hs := mcPacket(0x00, mcVarint(765), host, port, mcVarint(1))
	if _, err = conn.Write(append(hs, mcPacket(0x00)...)); err != nil {
		return st, 0, err
	}
	size, err := mcReadVarint(r)
	if err != nil {
		return st, 0, err
	}
	if size < 2 || size > 1<<20 {
		return st, 0, fmt.Errorf("paquet d'état de taille invalide (%d)", size)
	}
	pkt := make([]byte, size)
	if _, err = io.ReadFull(r, pkt); err != nil {
		return st, 0, err
	}
	pr := bufio.NewReader(strings.NewReader(string(pkt)))
	if id, e := pr.ReadByte(); e != nil || id != 0x00 {
		return st, 0, errors.New("réponse d'état inattendue")
	}
	n, err := mcReadVarint(pr)
	if err != nil || n < 0 || int(n) > len(pkt) {
		return st, 0, errors.New("réponse d'état illisible")
	}
	js := make([]byte, n)
	if _, err = io.ReadFull(pr, js); err != nil {
		return st, 0, err
	}
	if err = json.Unmarshal(js, &st); err != nil {
		return st, 0, err
	}

	// Ping : un seul aller-retour ; s'il échoue, l'état reste valable.
	start := time.Now()
	if _, err := conn.Write(mcPacket(0x01, make([]byte, 8))); err == nil {
		if sz, err := mcReadVarint(r); err == nil && sz > 0 && sz < 64 {
			if _, err := io.CopyN(io.Discard, r, int64(sz)); err == nil {
				lat = time.Since(start)
			}
		}
	}
	return st, lat, nil
}

// mcText aplatit une description Minecraft (chaîne ou arbre de composants) et
// retire les codes de couleur « § ».
func mcText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return mcStrip(s)
	}
	var c struct {
		Text  string
		Extra []json.RawMessage
	}
	if json.Unmarshal(raw, &c) != nil {
		return ""
	}
	out := c.Text
	for _, e := range c.Extra {
		out += mcText(e)
	}
	return mcStrip(out)
}

func mcStrip(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '§' {
			i++
			continue
		}
		b.WriteRune(rs[i])
	}
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(b.String(), "\n", " ")), " "))
}

func (m *minecraft) poll(ctx context.Context) tile.Values {
	st, lat, err := m.ping(ctx)
	if err != nil {
		return tile.Values{
			"online":  {Text: "Hors ligne", Level: "crit"},
			"players": {Text: "-"},
			"history": {Points: m.hist.Push(0)},
		}
	}
	v := tile.Values{
		"online":      {Text: "En ligne", Level: "ok"},
		"players":     {Text: fmt.Sprintf("%d / %d", st.Players.Online, st.Players.Max)},
		"version":     {Text: st.Version.Name},
		"motd":        {Text: mcText(st.Description)},
		"history":     {Points: m.hist.Push(float64(st.Players.Online)), Text: fmt.Sprint(st.Players.Online)},
		"player_list": {Head: []string{"Joueur"}},
	}
	var rows [][]tile.Cell
	for _, p := range st.Players.Sample {
		if len(rows) < 15 {
			rows = append(rows, []tile.Cell{{Text: p.Name}})
		}
	}
	pl := v["player_list"]
	pl.Rows = rows
	v["player_list"] = pl
	if lat > 0 {
		ms := float64(lat) / float64(time.Millisecond)
		v["latency"] = tile.Value{Text: tile.Num(ms) + " ms", Level: tile.Level(ms, 100, 300)}
	}
	return v
}
