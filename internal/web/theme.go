package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/neotoxicfr/nehome/internal/settings"
)

// theme : préréglage de couleurs ; les teintes secondaires dérivent de celles-ci (app.css).
type theme struct {
	Name, Label            string
	Bg, Surface, Text, Acc string
	Light                  bool
}

// Modes (fond, cartes, texte) ; la couleur principale se choisit à part.
var themes = []theme{
	{"sombre", "Sombre", "#0b0f17", "#172131", "#e5e9f0", "#7aa2ff", false},
	{"ardoise", "Ardoise", "#111315", "#1c1f23", "#e6e6e6", "#4fd1c5", false},
	{"oled", "OLED", "#000000", "#0e0e10", "#f2f2f2", "#ff9f43", false},
	{"clair", "Clair", "#eef1f6", "#ffffff", "#1b2230", "#3b6fe0", true},
}

// Pastilles de couleur principale (comme rustcraft-manager), plus une couleur libre.
var accents = []struct{ Name, Label, Hex string }{
	{"bleu", "Bleu", "#7aa2ff"}, {"cyan", "Turquoise", "#4fd1c5"}, {"vert", "Vert", "#7bd88f"}, {"jaune", "Ambre", "#e0b44c"},
	{"orange", "Orange", "#ff9f43"}, {"rouge", "Rouge", "#f27a7a"}, {"rose", "Rose", "#f472b6"}, {"violet", "Violet", "#b49cf5"},
}

func themeOf(name string) theme {
	switch name { // anciens noms
	case "nuit", "foret":
		name = "sombre"
	}
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return themes[0]
}

// mix mélange deux couleurs #rrggbb (part de b entre 0 et 1).
func mix(a, b string, part float64) string {
	pa, _ := strconv.ParseUint(a[1:], 16, 32)
	pb, _ := strconv.ParseUint(b[1:], 16, 32)
	c := func(sh uint) uint64 {
		x, y := float64(pa>>sh&255), float64(pb>>sh&255)
		return uint64(x + (y-x)*part + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", c(16), c(8), c(0))
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// color : couleur imposée si elle est valide, sinon celle du thème. Seul le
// format #rrggbb passe : aucune autre chaîne n'entre dans la feuille de style.
func color(custom, def string) string {
	if hexRe.MatchString(custom) {
		return strings.ToLower(custom)
	}
	return def
}

// luminance relative (sRGB) pour choisir un texte lisible sur l'accent.
func luminance(hex string) float64 {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	ch := func(c uint64) float64 {
		x := float64(c) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(v>>16&255) + 0.7152*ch(v>>8&255) + 0.0722*ch(v&255)
}

type palette struct {
	Bg, Surface, Text, Accent, OnAccent string
	Light                               bool
}

func paletteOf(p settings.Page) palette {
	t := themeOf(p.Theme)
	pl := palette{Bg: color(p.Bg, t.Bg), Surface: color(p.Surface, t.Surface), Text: color(p.Text, t.Text), Accent: color(p.Accent, t.Acc)}
	pl.Light = luminance(pl.Bg) > 0.4
	if pl.Light && luminance(pl.Accent) > 0.3 {
		pl.Accent = mix(pl.Accent, "#000000", 0.3) // sur fond clair, un accent pâle est foncé pour rester lisible
	}
	pl.OnAccent = "#0b0f17"
	if luminance(pl.Accent) < 0.35 {
		pl.OnAccent = "#ffffff"
	}
	return pl
}

func (pl palette) css() string {
	var b strings.Builder
	fmt.Fprintf(&b, ":root{--bg:%s;--surface:%s;--text:%s;--accent:%s;--on-accent:%s;", pl.Bg, pl.Surface, pl.Text, pl.Accent, pl.OnAccent)
	if pl.Light {
		b.WriteString("--ok:#138a5a;--warn:#b7791f;--crit:#d64545;color-scheme:light;")
	}
	b.WriteString("}\n")
	return b.String()
}

// themeCSS sert /theme.css (la CSP interdit les styles en ligne).
func (s *Server) themeCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // l'URL porte la version
	w.Write([]byte(paletteOf(s.settings.Page()).css()))
}

// brand : nom, version de la feuille de thème et favicon, pour toutes les pages.
func (s *Server) brand(data map[string]any) {
	p := s.settings.Page()
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "nehome"
	}
	sum := sha256.Sum256([]byte(paletteOf(p).css()))
	data["AssetV"] = assetV
	data["Brand"] = name
	data["ThemeV"] = hex.EncodeToString(sum[:6])
	if u := s.faviconURL(); u != "" {
		data["Favicon"] = u
	}
}

// brandEvent : identité poussée aux pages ouvertes (thème appliqué sans rechargement).
func (s *Server) brandEvent() {
	m := map[string]any{}
	s.brand(m)
	b, _ := json.Marshal(m)
	s.board.Event("brand", b)
}

// Favicon envoyé : un seul fichier data_dir/brand/favicon.<ext>, de type reconnu
// à sa signature (jamais d'après le nom ou l'en-tête du navigateur).
var faviconTypes = []struct {
	ext, ctype string
	magic      []byte
	off        int
}{
	{"png", "image/png", []byte("\x89PNG\r\n\x1a\n"), 0},
	{"ico", "image/x-icon", []byte{0, 0, 1, 0}, 0},
	{"webp", "image/webp", []byte("WEBP"), 8},
}

func (s *Server) brandDir() string { return filepath.Join(s.conf.Load().DataDir, "brand") }

func (s *Server) faviconFile() (path, ctype string, mod int64) {
	for _, ft := range faviconTypes {
		p := filepath.Join(s.brandDir(), "favicon."+ft.ext)
		if st, err := os.Stat(p); err == nil {
			return p, ft.ctype, st.ModTime().Unix()
		}
	}
	return "", "", 0
}

func (s *Server) faviconURL() string {
	if p, _, mod := s.faviconFile(); p != "" {
		return fmt.Sprintf("/favicon?v=%d", mod)
	}
	return ""
}

func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	p, ctype, _ := s.faviconFile()
	if p == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // l'URL porte la version
	http.ServeFile(w, r, p)
}

// saveFavicon enregistre (ou retire) le favicon envoyé.
func (s *Server) saveFavicon(w http.ResponseWriter, r *http.Request) {
	err := s.storeFavicon(r)
	if err == nil {
		s.brandEvent()
	}
	if r.Header.Get("X-Requested-With") == "fetch" {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"favicon": s.faviconURL()})
		return
	}
	if err != nil {
		back(w, r, "", err)
		return
	}
	http.Redirect(w, r, "/settings?ok=identity#identity", http.StatusSeeOther)
}

func (s *Server) storeFavicon(r *http.Request) error {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return errors.New("fichier trop gros (512 Ko au plus)")
	}
	dir := s.brandDir()
	if r.FormValue("remove") == "1" {
		for _, ft := range faviconTypes {
			os.Remove(filepath.Join(dir, "favicon."+ft.ext))
		}
		return nil
	}
	f, _, err := r.FormFile("favicon")
	if err != nil {
		return errors.New("aucun fichier reçu")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 512<<10+1))
	if err != nil || len(b) > 512<<10 {
		return errors.New("fichier trop gros (512 Ko au plus)")
	}
	for _, ft := range faviconTypes {
		if len(b) >= ft.off+len(ft.magic) && bytes.Equal(b[ft.off:ft.off+len(ft.magic)], ft.magic) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			for _, o := range faviconTypes {
				os.Remove(filepath.Join(dir, "favicon."+o.ext))
			}
			tmp := filepath.Join(dir, ".favicon.tmp")
			if err := os.WriteFile(tmp, b, 0o644); err != nil {
				return err
			}
			return os.Rename(tmp, filepath.Join(dir, "favicon."+ft.ext))
		}
	}
	return errors.New("format non reconnu : PNG, ICO ou WebP")
}

// assetV : empreinte de app.css et app.js, mise dans leur URL : un navigateur ne
// garde jamais une ancienne feuille de style après une mise à jour.
var assetV = func() string {
	h := sha256.New()
	for _, f := range []string{"static/app.css", "static/app.js"} {
		b, _ := staticFS.ReadFile(f)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)[:6])
}()
