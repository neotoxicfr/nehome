package integ

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// ntfy : messages des dernières 24 h d'un sujet (option topic, « uptime » par
// défaut), relus toutes les 30 s par GET …/json?poll=1. La réponse est du JSON
// ligne à ligne (un message par ligne), pas un tableau.
type ntfy struct {
	url, topic, user, pass string
	hc                     *http.Client
}

type ntfyMsg struct {
	Event    string   `json:"event"`
	Time     int64    `json:"time"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"` // absent = 3 (défaut)
	Tags     []string `json:"tags"`
}

func newNtfy(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &ntfy{url: strings.TrimRight(t.URL, "/"), topic: t.Options["topic"], hc: httpClient(t.InsecureTLS, false)}
	if n.topic == "" {
		n.topic = "uptime"
	}
	// Identifiants facultatifs : un sujet peut être lisible anonymement.
	if _, ok := t.Secrets["user"]; ok {
		var err error
		if n.user, err = secret("user"); err != nil {
			return nil, err
		}
		if n.pass, err = secret("pass"); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func (n *ntfy) Fields() []tile.Field {
	return []tile.Field{
		tile.F("count", "Messages (24 h)", tile.Stat, tile.OnTile),
		tile.F("last", "Dernier message", tile.Stat, tile.OnTile),
		tile.F("urgent", "Priorité haute (24 h)", tile.Stat, tile.OnTile),
		tile.F("messages", "Messages des dernières 24 h", tile.Table, tile.InDetail),
	}
}

func (n *ntfy) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		msgs, err := n.poll(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			emit(nil, err)
			return
		}
		emit(ntfyValues(msgs, time.Now()), nil)
	})
}

func (n *ntfy) poll(ctx context.Context) ([]ntfyMsg, error) {
	u := n.url + "/" + url.PathEscape(n.topic) + "/json?poll=1&since=24h"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if n.user != "" {
		req.SetBasicAuth(n.user, n.pass)
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("ntfy refuse la lecture du sujet %q (HTTP %d)", n.topic, resp.StatusCode)
		}
		return nil, statusError{resp.StatusCode, redact(u)}
	}
	var msgs []ntfyMsg
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 32<<20))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var m ntfyMsg
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Event == "message" {
			msgs = append(msgs, m)
		}
	}
	return msgs, sc.Err()
}

// ntfyLevel : priorités ntfy 1 (min) à 5 (urgente).
func ntfyLevel(p int) string {
	switch {
	case p >= 5:
		return "crit"
	case p == 4:
		return "warn"
	}
	return ""
}

// ntfyValues : les messages arrivent du plus ancien au plus récent.
func ntfyValues(msgs []ntfyMsg, now time.Time) tile.Values {
	urgent := 0
	var rows [][]tile.Cell
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Priority >= 4 {
			urgent++
		}
		if len(rows) < 15 {
			t := time.Unix(m.Time, 0)
			title := m.Title
			if title == "" {
				title = strings.Join(m.Tags, ", ")
			}
			rows = append(rows, []tile.Cell{{Text: t.Format("02/01 15:04")}, {Text: title, Level: ntfyLevel(m.Priority)}, {Text: m.Message}})
		}
	}
	v := tile.Values{
		"count":    {Text: fmt.Sprint(len(msgs))},
		"urgent":   {Text: fmt.Sprint(urgent), Level: "ok"},
		"messages": {Head: []string{"Heure", "Titre", "Message"}, Rows: rows, Text: "aucun message"},
		"last":     {Text: "aucun"},
	}
	if urgent > 0 {
		v["urgent"] = tile.Value{Text: fmt.Sprint(urgent), Level: "warn"}
	}
	if len(msgs) > 0 {
		m := msgs[len(msgs)-1]
		text := m.Title
		if text == "" {
			text = m.Message
		}
		v["last"] = tile.Value{Text: text, Sub: "il y a " + tile.Dur(now.Sub(time.Unix(m.Time, 0))), Level: ntfyLevel(m.Priority)}
	}
	return v
}
