package board

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

type src struct {
	runs, stops *atomic.Int32
}

func (src) Fields() []tile.Field { return []tile.Field{tile.F("x", "X", tile.Stat, tile.OnTile)} }
func (s src) Run(ctx context.Context, emit func(tile.Values, error)) {
	s.runs.Add(1)
	emit(tile.Values{"x": {Text: "1"}}, nil)
	<-ctx.Done()
	s.stops.Add(1)
}

func TestApply(t *testing.T) {
	st, _ := settings.Open(filepath.Join(t.TempDir(), "s.json"))
	b := New(nil, st)
	var runs, stops atomic.Int32
	b.New = func(config.Tile) tile.Source { return src{&runs, &stops} }
	b.Render = func(v View) string { return v.ID }
	b.Start(t.Context())
	ch, _, cancel := b.Subscribe()
	defer cancel()

	b.Apply([]Spec{{config.Tile{ID: "a"}, "1"}, {config.Tile{ID: "b"}, "1"}})
	a := b.Tile("a")
	b.Apply([]Spec{{config.Tile{ID: "a"}, "1"}, {config.Tile{ID: "b"}, "2"}, {config.Tile{ID: "c"}, "1"}})
	if b.Tile("a") != a {
		t.Fatal("tuile inchangée redémarrée")
	}
	b.Apply([]Spec{{config.Tile{ID: "a"}, "1"}})
	if b.Tile("b") != nil || b.Tile("c") != nil || len(b.All()) != 1 {
		t.Fatal("tuiles retirées encore présentes")
	}
	deadline := time.Now().Add(2 * time.Second)
	for stops.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// a, b, b', c lancées ; b, b', c arrêtées.
	if runs.Load() != 4 || stops.Load() != 3 {
		t.Fatalf("lancées %d, arrêtées %d", runs.Load(), stops.Load())
	}
	reload := false
	for len(ch) > 0 {
		if string(<-ch)[:13] == "event: reload" {
			reload = true
		}
	}
	if !reload {
		t.Fatal("pas d'ordre de rechargement aux navigateurs")
	}
}

func TestSectionsOrder(t *testing.T) {
	st, _ := settings.Open(filepath.Join(t.TempDir(), "s.json"))
	tiles := []*Tile{{Cfg: config.Tile{ID: "a", Section: "S1"}}, {Cfg: config.Tile{ID: "b", Section: "S2"}}, {Cfg: config.Tile{ID: "c", Section: "S1"}}}
	b := New(tiles, st)
	st.Set("c", settings.Tile{Pos: 5}) // c avant a : S1 reste première, c en tête
	st.Set("b", settings.Tile{Pos: 1}) // b tout en haut : S2 passe devant
	secs := b.Sections()
	if len(secs) != 2 || secs[0].Name != "S2" || secs[1].Tiles[0].Cfg.ID != "c" || secs[1].Tiles[1].Cfg.ID != "a" {
		t.Fatalf("ordre : %+v", secs)
	}
}
