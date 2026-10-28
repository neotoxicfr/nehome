// Package icons sert les icônes de la bibliothèque dashboard-icons depuis
// l'origine de nehome (la CSP n'autorise que img-src 'self') : chaque PNG est
// téléchargé une fois, vérifié, puis gardé dans data_dir/icons.
package icons

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const cdn = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons"

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

// Noms d'icône qui diffèrent du type d'intégration.
var byType = map[string]string{
	"truenas": "truenas-scale", "uptimekuma": "uptime-kuma", "adguard": "adguard-home",
	"seerr": "overseerr", "crossseed": "cross-seed", "npm": "nginx-proxy-manager",
	"wgeasy": "wireguard", "homeassistant": "home-assistant", "pihole": "pi-hole",
	"speedtest": "speedtest-tracker", "paperless": "paperless-ngx", "system": "linux",
}

type Store struct {
	dir  string
	base string
	hc   *http.Client

	mu    sync.RWMutex
	names map[string]bool
	fetch map[string]*sync.Mutex // une récupération à la fois par icône
}

func New(dir string) *Store {
	return &Store{dir: dir, base: cdn, hc: &http.Client{Timeout: 15 * time.Second}, fetch: map[string]*sync.Mutex{}}
}

// Start charge l'index (cache local s'il a moins d'un jour) puis le rafraîchit
// chaque jour. Sans réseau, simplement pas d'icônes.
func (s *Store) Start(ctx context.Context) {
	os.MkdirAll(s.dir, 0o755)
	idx := filepath.Join(s.dir, "index.json")
	if st, err := os.Stat(idx); err == nil && time.Since(st.ModTime()) < 24*time.Hour {
		if b, err := os.ReadFile(idx); err == nil {
			s.load(b)
		}
	}
	go func() {
		for {
			if s.empty() || stale(idx) {
				if b, err := s.get(ctx, s.base+"/tree.json", 8<<20); err == nil && s.load(b) {
					os.WriteFile(idx, b, 0o644)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()
}

func stale(p string) bool {
	st, err := os.Stat(p)
	return err != nil || time.Since(st.ModTime()) > 24*time.Hour
}

func (s *Store) empty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.names) == 0
}

func (s *Store) load(b []byte) bool {
	var tree struct {
		PNG []string `json:"png"`
	}
	if json.Unmarshal(b, &tree) != nil || len(tree.PNG) == 0 {
		return false
	}
	names := make(map[string]bool, len(tree.PNG))
	for _, f := range tree.PNG {
		if n, ok := strings.CutSuffix(f, ".png"); ok && nameRe.MatchString(n) {
			names[n] = true
		}
	}
	s.mu.Lock()
	s.names = names
	s.mu.Unlock()
	return true
}

// Has : l'icône existe dans la bibliothèque, ou un PNG du même nom a été
// déposé à la main dans data_dir/icons (applis absentes de la bibliothèque).
func (s *Store) Has(name string) bool {
	s.mu.RLock()
	ok := s.names[name]
	s.mu.RUnlock()
	if ok || !nameRe.MatchString(name) {
		return ok
	}
	_, err := os.Stat(filepath.Join(s.dir, name+".png"))
	return err == nil
}

// Slug : « Château (Rhône) » → « chateau-rhone ».
func Slug(s string) string {
	r := strings.NewReplacer("é", "e", "è", "e", "ê", "e", "à", "a", "ç", "c", "ô", "o", "î", "i", "û", "u", "ù", "u")
	s = r.Replace(strings.ToLower(s))
	var b strings.Builder
	dash := false
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Guess : première icône existante parmi le nom imposé, le type, le
// conteneur, l'id et le titre de la tuile ; "" si aucune.
func (s *Store) Guess(forced, typ, container, id, title string) string {
	for _, c := range []string{forced, byType[typ], typ, container, id, Slug(title)} {
		if c = Slug(c); c != "" && s.Has(c) {
			return c
		}
	}
	return ""
}

// ServeHTTP sert /icon/{nom}.png. Seuls les noms de l'index sont récupérés
// (pas de téléchargement arbitraire), et seulement de vrais PNG.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(r.PathValue("file"), ".png")
	if !ok || !nameRe.MatchString(name) || !s.Has(name) {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(s.dir, name+".png")
	if _, err := os.Stat(p); err != nil {
		if err := s.download(r.Context(), name, p); err != nil {
			http.Error(w, "icône indisponible", http.StatusBadGateway)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, p)
}

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

func (s *Store) download(ctx context.Context, name, p string) error {
	s.mu.Lock()
	m := s.fetch[name]
	if m == nil {
		m = &sync.Mutex{}
		s.fetch[name] = m
	}
	s.mu.Unlock()
	m.Lock()
	defer m.Unlock()
	if _, err := os.Stat(p); err == nil {
		return nil // récupérée entre-temps
	}
	b, err := s.get(ctx, s.base+"/png/"+name+".png", 512<<10)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(b, pngMagic) {
		return errors.New("pas un PNG")
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *Store) get(ctx context.Context, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if int64(len(b)) > max {
		return nil, errors.New("réponse trop grande")
	}
	return b, err
}
