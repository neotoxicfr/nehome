// Package settings garde les choix d'affichage faits sur la page /settings.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/neotoxicfr/nehome/internal/tile"
)

type Tile struct {
	Hidden  bool                      `json:"hidden,omitempty"`
	Icon    string                    `json:"icon,omitempty"`    // nom imposé dans la bibliothèque d'icônes
	NoIcon  bool                      `json:"no_icon,omitempty"` // icône masquée sur l'accueil
	Pos     int                       `json:"pos,omitempty"`     // position sur l'accueil (0 : ordre de config.yaml)
	Fields  map[string]tile.Placement `json:"fields,omitempty"`
	Actions map[string]tile.Placement `json:"actions,omitempty"` // placement des actions (défaut de l'intégration sinon)
	Order   []string                  `json:"order,omitempty"`
}

// Page : réglages de l'accueil.
type Page struct {
	Layout  string `json:"layout,omitempty"` // "" grille ordonnée, "auto" colonnes équilibrées
	Name    string `json:"name,omitempty"`   // nom affiché (défaut nehome)
	Theme   string `json:"theme,omitempty"`  // préréglage de couleurs (défaut nuit)
	Accent  string `json:"accent,omitempty"` // couleurs imposées (#rrggbb), sinon celles du thème
	Bg      string `json:"bg,omitempty"`
	Surface string `json:"surface,omitempty"`
	Text    string `json:"text,omitempty"`
	Favicon string `json:"favicon,omitempty"` // nom d'icône (bibliothèque ou data/icons)
}

type file struct {
	Tiles map[string]Tile `json:"tiles"`
	Page  Page            `json:"page"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	m    map[string]Tile
	page Page
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, m: map[string]Tile{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err == nil && f.Tiles != nil {
		s.m, s.page = f.Tiles, f.Page
		return s, nil
	}
	return s, json.Unmarshal(b, &s.m) // ancien format : tuiles seules
}

func (s *Store) Page() Page {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.page
}

func (s *Store) SetPage(p Page) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.page = p
	return s.save()
}

func (s *Store) Get(id string) Tile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[id]
}

// Set écrit le fichier entier de façon atomique (fichier temporaire puis
// renommage) : une coupure ne laisse jamais un JSON tronqué.
func (s *Store) Set(id string, t Tile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = t
	return s.save()
}

// save écrit tout le fichier (s.mu tenu).
func (s *Store) save() error {
	b, err := json.MarshalIndent(file{Tiles: s.m, Page: s.page}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Placement d'un champ : choix enregistré, sinon défaut de l'intégration.
func (t Tile) Placement(f tile.Field) tile.Placement {
	if p, ok := t.Fields[f.ID]; ok && p.Valid() {
		return p
	}
	return f.Default
}

// ActionPlacement : placement d'une action (réglage, sinon défaut).
func (t Tile) ActionPlacement(a tile.Action) tile.Placement {
	if p, ok := t.Actions[a.ID]; ok && p.Valid() {
		return p
	}
	if a.Default == "" {
		return tile.InDetail
	}
	return a.Default
}

// Sort range les champs : ceux de Order d'abord, le reste dans l'ordre déclaré.
func (t Tile) Sort(fields []tile.Field) []tile.Field {
	byID := map[string]tile.Field{}
	for _, f := range fields {
		byID[f.ID] = f
	}
	out := make([]tile.Field, 0, len(fields))
	done := map[string]bool{}
	for _, id := range t.Order {
		if f, ok := byID[id]; ok && !done[id] {
			out = append(out, f)
			done[id] = true
		}
	}
	for _, f := range fields {
		if !done[f.ID] {
			out = append(out, f)
		}
	}
	return out
}
