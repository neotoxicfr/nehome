package integ

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Portique (passerelle RDP/SSH). GET /api/machine/status avec la clé de
// lecture (rôle token) : nom, accès, joignable ou non, sessions ouvertes ;
// ni adresse, ni MAC, ni compte. Le rôle wake (facultatif) active le bouton
// de réveil : POST /api/machine/wake/<id>. Relevé toutes les 30 s.
type portique struct {
	url, token, wake string
	hc               *http.Client
	mu               sync.Mutex
	wakeable         map[string]int // nom -> id, machines réveillables
}

func newPortique(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	p := &portique{url: strings.TrimRight(t.URL, "/") + "/api/machine", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if p.token, err = secret("token"); err != nil {
		return nil, err
	}
	if _, ok := t.Secrets["wake"]; ok {
		if p.wake, err = secret("wake"); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *portique) Fields() []tile.Field {
	return []tile.Field{
		tile.F("online", "En ligne", tile.Stat, tile.OnTile),
		tile.F("sessions", "Sessions", tile.Stat, tile.OnTile),
		tile.F("machines", "Machines", tile.Table, tile.OnTile),
	}
}

func (p *portique) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		v, err := p.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (p *portique) poll(ctx context.Context) (tile.Values, error) {
	var s struct {
		Machines []struct {
			ID       int
			Name     string
			RDP, SSH bool
			CanWake  bool  `json:"can_wake"`
			Online   *bool // nil : jointe seulement par un rebond
			Sessions struct{ RDP, SSH int }
		}
		Online   int
		Sessions struct{ RDP, SSH int }
	}
	if err := getJSON(ctx, p.hc, p.url+"/status", &s, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+p.token) }); err != nil {
		return nil, err
	}
	wakeable := map[string]int{}
	var rows [][]tile.Cell
	down := 0
	for _, m := range s.Machines {
		state := tile.Cell{Text: "Par rebond"}
		switch {
		case m.Online == nil:
		case *m.Online:
			state = tile.Cell{Text: "En ligne", Level: "ok"}
		case m.CanWake: // machine qu'on réveille à la demande : éteinte n'est pas une panne
			state = tile.Cell{Text: "Éteinte"}
		default:
			state = tile.Cell{Text: "Hors ligne", Level: "crit"}
			down++
		}
		var access []string
		if m.RDP {
			access = append(access, "RDP")
		}
		if m.SSH {
			access = append(access, "SSH")
		}
		if m.CanWake {
			access = append(access, "réveil")
			if p.wake != "" {
				wakeable[m.Name] = m.ID
			}
		}
		open := sessionsText(m.Sessions.RDP, m.Sessions.SSH)
		if open == "aucune" {
			open = "—"
		}
		rows = append(rows, []tile.Cell{{Text: m.Name}, state, {Text: strings.Join(access, " · ")}, {Text: open}})
	}
	p.mu.Lock()
	p.wakeable = wakeable
	p.mu.Unlock()
	online := tile.Value{Text: fmt.Sprintf("%d / %d", s.Online, len(s.Machines))}
	if down > 0 {
		online.Level, online.Sub = "warn", fmt.Sprintf("%d hors ligne", down)
	}
	return tile.Values{
		"online":   online,
		"sessions": {Text: sessionsText(s.Sessions.RDP, s.Sessions.SSH)},
		"machines": {Head: []string{"Machine", "État", "Accès", "Sessions"}, Rows: rows},
	}, nil
}

// sessionsText : « aucune », « 1 bureau », « 1 bureau · 2 terminaux ».
func sessionsText(rdp, ssh int) string {
	var parts []string
	if rdp > 0 {
		parts = append(parts, plural(rdp, "bureau", "bureaux"))
	}
	if ssh > 0 {
		parts = append(parts, plural(ssh, "terminal", "terminaux"))
	}
	if len(parts) == 0 {
		return "aucune"
	}
	return strings.Join(parts, " · ")
}

func plural(n int, one, many string) string {
	if n > 1 {
		return fmt.Sprintf("%d %s", n, many)
	}
	return fmt.Sprintf("%d %s", n, one)
}

func (p *portique) Actions() []tile.Action {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.wakeable) == 0 {
		return nil
	}
	names := make([]string, 0, len(p.wakeable))
	for n := range p.wakeable {
		names = append(names, n)
	}
	slices.Sort(names)
	return []tile.Action{{ID: "wake", Label: "Réveiller une machine", Choices: names}}
}

func (p *portique) Do(ctx context.Context, id, arg string) (string, error) {
	p.mu.Lock()
	mid, ok := p.wakeable[arg]
	p.mu.Unlock()
	if id != "wake" || !ok {
		return "", errUnknownAction
	}
	err := callAction(ctx, p.hc, http.MethodPost, fmt.Sprintf("%s/wake/%d", p.url, mid), nil, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+p.wake)
	})
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusTooManyRequests {
		return "", errors.New("refusé : trop tôt après le précédent réveil, réessaie dans quelques minutes")
	}
	if err != nil {
		return "", err
	}
	return arg + " : réveil envoyé", nil
}
