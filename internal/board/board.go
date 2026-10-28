// Package board tient l'état de chaque tuile, la re-rend quand une valeur ou
// un réglage change, et diffuse le fragment HTML aux navigateurs abonnés.
package board

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

type Tile struct {
	Cfg    config.Tile
	Src    tile.Source
	Fields []tile.Field

	rmu     sync.Mutex // un rendu à la fois : un fragment ancien ne passe pas après un récent
	mu      sync.Mutex
	vals    tile.Values
	err     string
	updated time.Time
	html    string
	dirty   bool // valeurs reçues sans rendu (personne ne regardait)

	sig     string             // définition + état des secrets : change → redémarrage
	cancel  context.CancelFunc // arrête la source
	stopped atomic.Bool        // émissions tardives d'une source arrêtée ignorées
}

// Spec : une tuile voulue par la configuration.
type Spec struct {
	Cfg config.Tile
	Sig string
}

// FieldView : un champ prêt pour le gabarit.
type FieldView struct {
	tile.Field
	tile.Value
	Has bool
}

// ActionView : une action prête pour le gabarit.
type ActionView struct {
	tile.Action
	TileID string
}

type View struct {
	ActOnTile, ActDetail     []ActionView
	Cfg                      config.Tile
	ID, Title, Href, Section string
	Err                      string
	Updated                  time.Time
	OnTile, Detail           []FieldView
}

// Status : couleur du point de la tuile.
func (v View) Status() string {
	switch {
	case v.Err != "":
		return "crit"
	case v.Updated.IsZero():
		return "wait"
	}
	return "ok"
}

type Section struct {
	Name  string
	Tiles []*Tile
}

type Board struct {
	Render   func(View) string             // fourni par le paquet web
	OnActive func(bool)                    // premier abonné arrivé / dernier parti
	New      func(config.Tile) tile.Source // fabrique les sources (Apply)

	ctx      context.Context
	tmu      sync.RWMutex // tiles, byID
	tiles    []*Tile
	byID     map[string]*Tile
	settings *settings.Store

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func New(tiles []*Tile, st *settings.Store) *Board {
	b := &Board{tiles: tiles, byID: map[string]*Tile{}, settings: st, subs: map[chan []byte]struct{}{}}
	for _, t := range tiles {
		b.byID[t.Cfg.ID] = t
	}
	return b
}

func (b *Board) Start(ctx context.Context) {
	b.ctx = ctx
	for _, t := range b.All() {
		b.start(t)
	}
}

func (b *Board) start(t *Tile) {
	ctx, cancel := context.WithCancel(b.ctx)
	t.cancel = cancel
	b.rerender(t)
	go t.Src.Run(ctx, func(v tile.Values, err error) { b.emit(t, v, err) })
}

func (b *Board) stop(t *Tile) {
	t.stopped.Store(true)
	if t.cancel != nil {
		t.cancel()
	}
}

// Apply met le tableau en conformité avec la configuration : tuiles
// inchangées gardées telles quelles (valeurs comprises), nouvelles ou
// modifiées (re)démarrées, retirées arrêtées. Les navigateurs rechargent la
// page si la liste a changé.
func (b *Board) Apply(specs []Spec) {
	b.tmu.Lock()
	var next, started []*Tile
	byID := map[string]*Tile{}
	keep := map[*Tile]bool{}
	for _, s := range specs {
		if old := b.byID[s.Cfg.ID]; old != nil && old.sig == s.Sig {
			next = append(next, old)
			byID[s.Cfg.ID] = old
			keep[old] = true
			continue
		}
		src := b.New(s.Cfg)
		t := &Tile{Cfg: s.Cfg, Src: src, Fields: src.Fields(), sig: s.Sig}
		next = append(next, t)
		byID[s.Cfg.ID] = t
		started = append(started, t)
	}
	var stopped []*Tile
	for _, t := range b.tiles {
		if !keep[t] {
			stopped = append(stopped, t)
		}
	}
	changed := len(started) > 0 || len(stopped) > 0 || !slices.Equal(ids(next), ids(b.tiles))
	b.tiles, b.byID = next, byID
	b.tmu.Unlock()
	for _, t := range stopped {
		b.stop(t)
	}
	for _, t := range started {
		b.start(t)
	}
	if changed {
		b.broadcast([]byte("event: reload\ndata: 1\n\n"))
	}
}

func ids(ts []*Tile) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Cfg.ID
	}
	return out
}

func (b *Board) emit(t *Tile, v tile.Values, err error) {
	if t.stopped.Load() {
		return
	}
	t.mu.Lock()
	if v != nil {
		if t.vals == nil {
			t.vals = tile.Values{}
		}
		for k, x := range v {
			t.vals[k] = x
		}
		t.updated = time.Now()
	}
	// err porte l'état de la source entière : nil efface (voir integ.split pour
	// les sources à plusieurs boucles).
	t.err = ""
	if err != nil {
		t.err = err.Error()
	}
	// Rendu seulement si un navigateur regarde ; sinon au prochain affichage.
	b.mu.Lock()
	watched := len(b.subs) > 0
	b.mu.Unlock()
	if !watched {
		t.dirty = true
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	b.rerender(t)
}

// fresh rend la tuile si des valeurs sont arrivées sans rendu.
func (b *Board) fresh(t *Tile) {
	t.mu.Lock()
	d := t.dirty
	t.mu.Unlock()
	if d {
		b.rerender(t)
	}
}

// Refresh re-rend une tuile après un changement de réglage.
func (b *Board) Refresh(id string) {
	if t := b.Tile(id); t != nil {
		b.rerender(t)
	}
}

func (b *Board) rerender(t *Tile) {
	t.rmu.Lock()
	defer t.rmu.Unlock()
	v := b.view(t)
	html := b.Render(v)
	t.mu.Lock()
	changed := html != t.html
	t.html = html
	t.dirty = false
	t.mu.Unlock()
	if changed {
		b.broadcast(frame(t.Cfg.ID, html))
	}
}

func (b *Board) view(t *Tile) View {
	st := b.settings.Get(t.Cfg.ID)
	t.mu.Lock()
	defer t.mu.Unlock()
	v := View{Cfg: t.Cfg, ID: t.Cfg.ID, Title: t.Cfg.Title, Href: t.Cfg.Href, Section: t.Cfg.Section, Err: t.err, Updated: t.updated}
	for _, f := range st.Sort(t.Fields) {
		val, has := t.vals[f.ID]
		if !has && (!t.updated.IsZero() || t.err != "") {
			continue // donnée absente de la source, ou source en panne : rien à attendre
		}
		fv := FieldView{Field: f, Value: val, Has: has}
		switch st.Placement(f) {
		case tile.OnTile:
			v.OnTile = append(v.OnTile, fv)
			v.Detail = append(v.Detail, fv)
		case tile.InDetail:
			v.Detail = append(v.Detail, fv)
		}
	}
	if a, ok := t.Src.(tile.Actor); ok {
		for _, act := range a.Actions() {
			av := ActionView{Action: act, TileID: t.Cfg.ID}
			switch st.ActionPlacement(act) {
			case tile.OnTile:
				v.ActOnTile = append(v.ActOnTile, av)
				v.ActDetail = append(v.ActDetail, av)
			case tile.InDetail:
				v.ActDetail = append(v.ActDetail, av)
			}
		}
	}
	return v
}

// Do exécute une action d'une tuile.
func (b *Board) Do(ctx context.Context, tileID, action, arg string) (string, error) {
	t := b.Tile(tileID)
	if t == nil {
		return "", fmt.Errorf("tuile %q introuvable", tileID)
	}
	a, ok := t.Src.(tile.Actor)
	if !ok {
		return "", fmt.Errorf("%s n'a pas d'action", tileID)
	}
	var found *tile.Action
	for _, x := range a.Actions() {
		if x.ID == action {
			found = &x
			break
		}
	}
	if found == nil {
		return "", fmt.Errorf("action %q inconnue", action)
	}
	if len(found.Choices) > 0 && !slices.Contains(found.Choices, arg) {
		return "", fmt.Errorf("élément %q non proposé", arg) // jamais un argument libre
	}
	return a.Do(ctx, action, arg)
}

func (b *Board) View(id string) (View, bool) {
	t := b.Tile(id)
	if t == nil {
		return View{}, false
	}
	return b.view(t), true
}

func (b *Board) HTML(id string) string {
	t := b.Tile(id)
	if t == nil {
		return ""
	}
	b.fresh(t)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.html
}

func (b *Board) Tile(id string) *Tile {
	b.tmu.RLock()
	defer b.tmu.RUnlock()
	return b.byID[id]
}

func (b *Board) All() []*Tile {
	b.tmu.RLock()
	defer b.tmu.RUnlock()
	return slices.Clone(b.tiles)
}

// Pos : position effective d'une tuile (réglage, sinon ordre de config.yaml).
func (b *Board) Pos(i int, id string) int {
	if p := b.settings.Get(id).Pos; p > 0 {
		return p
	}
	return (i + 1) * 10
}

// Event diffuse un événement SSE nommé (données sur une ligne, ex. JSON).
func (b *Board) Event(name string, data []byte) {
	b.broadcast(append(append([]byte("event: "+name+"\ndata: "), data...), '\n', '\n'))
}

// Reload demande aux navigateurs ouverts de recharger la page (ordre changé).
func (b *Board) Reload() { b.broadcast([]byte("event: reload\ndata: 1\n\n")) }

// Sections : tuiles visibles triées par position ; une section apparaît à la
// place de sa première tuile.
func (b *Board) Sections() []Section {
	var out []Section
	idx := map[string]int{}
	all := b.All()
	pos := map[string]int{}
	for i, t := range all {
		pos[t.Cfg.ID] = b.Pos(i, t.Cfg.ID)
	}
	slices.SortStableFunc(all, func(x, y *Tile) int { return pos[x.Cfg.ID] - pos[y.Cfg.ID] })
	for _, t := range all {
		if b.settings.Get(t.Cfg.ID).Hidden {
			continue
		}
		i, ok := idx[t.Cfg.Section]
		if !ok {
			i = len(out)
			idx[t.Cfg.Section] = i
			out = append(out, Section{Name: t.Cfg.Section})
		}
		out[i].Tiles = append(out[i].Tiles, t)
	}
	return out
}

func frame(id, html string) []byte {
	data, _ := json.Marshal(map[string]string{"id": id, "html": html})
	return append(append([]byte("event: tile\ndata: "), data...), '\n', '\n')
}

// Subscribe renvoie l'état courant de toutes les tuiles puis le flux des
// changements. Un abonné trop lent est coupé : le navigateur se reconnecte et
// repart de l'état courant, plutôt que de rater des fragments.
func (b *Board) Subscribe() (ch chan []byte, snapshot [][]byte, cancel func()) {
	// Tampon : l'instantané peut déclencher un rendu (donc une diffusion) par tuile.
	all := b.All()
	ch = make(chan []byte, len(all)+64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	if len(b.subs) == 1 && b.OnActive != nil {
		b.OnActive(true) // sous b.mu : ordre garanti face à drop
	}
	b.mu.Unlock()
	for _, t := range all {
		snapshot = append(snapshot, frame(t.Cfg.ID, b.HTML(t.Cfg.ID)))
	}
	return ch, snapshot, func() {
		b.mu.Lock()
		b.drop(ch)
		b.mu.Unlock()
	}
}

// drop retire un abonné (b.mu tenu) et signale quand plus personne ne regarde.
func (b *Board) drop(ch chan []byte) {
	if _, ok := b.subs[ch]; !ok {
		return
	}
	delete(b.subs, ch)
	close(ch)
	if len(b.subs) == 0 && b.OnActive != nil {
		b.OnActive(false) // SetActive ne prend que son propre verrou : pas de cycle
	}
}

func (b *Board) broadcast(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
			b.drop(ch)
		}
	}
}
