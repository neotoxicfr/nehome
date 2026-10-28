// Package web sert la page, le flux SSE, le détail sans JS et les réglages.
package web

import (
	"bytes"
	"cmp"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/icons"
	"github.com/neotoxicfr/nehome/internal/integ"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

//go:embed templates/*.html
var tplFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	conf     atomic.Pointer[config.Config] // remplacée à chaud (SetConfig)
	notice   atomic.Value                  // bandeau d'avertissement (string)
	board    *board.Board
	settings *settings.Store
	tpl      *template.Template

	flashMu sync.Mutex
	flash   map[string]flashMsg // résultat d'action par utilisateur (formulaires sans script)

	Icons  *icons.Store // facultatif
	OnEdit func() error // relit la configuration après une modification depuis l'interface
}

func New(cfg *config.Config, b *board.Board, st *settings.Store) *Server {
	s := &Server{board: b, settings: st}
	s.conf.Store(cfg)
	s.notice.Store("")
	s.tpl = template.Must(template.New("").Funcs(funcs).ParseFS(tplFS, "templates/*.html"))
	b.Render = s.renderTile
	return s
}

func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(staticFS, "static")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	files := http.StripPrefix("/static/", http.FileServerFS(static))
	mux.Handle("GET /static/", s.authed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == assetV { // URL versionnée : immuable
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /{$}", s.authed(s.index))
	mux.HandleFunc("GET /events", s.authed(s.events))
	mux.HandleFunc("GET /tile/{id}", s.authed(s.detail))
	mux.HandleFunc("GET /settings", s.authed(s.settingsPage))
	mux.HandleFunc("POST /settings/{id}", s.authed(s.saveSettings))
	mux.HandleFunc("GET /settings/form", s.adminOnly(s.addForm))
	mux.HandleFunc("POST /page", s.authed(s.savePage))
	mux.HandleFunc("POST /identity", s.adminOnly(s.saveIdentity))
	mux.HandleFunc("POST /action/{tile}/{action}", s.adminOnly(s.action))
	mux.HandleFunc("GET /theme.css", s.authed(s.themeCSS))
	mux.HandleFunc("POST /identity/favicon", s.adminOnly(s.saveFavicon))
	mux.HandleFunc("GET /favicon", s.authed(s.favicon))
	mux.HandleFunc("POST /tiles", s.adminOnly(s.saveTile))
	mux.HandleFunc("POST /tiles/{id}/delete", s.adminOnly(s.deleteTile))
	mux.HandleFunc("GET /icon/{file}", s.authed(s.icon))
	return headers(mux)
}

// headers : en-têtes de contenu posés par l'appli (Traefik garde HSTS et la compression).
func headers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin")
		hd.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.ServeHTTP(w, r)
	})
}

func peer(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

// user : en-tête du proxy, cru seulement si la connexion vient d'un proxy déclaré.
func (s *Server) user(r *http.Request) string {
	if s.conf.Load().Auth.Disabled {
		return "banc"
	}
	ip := peer(r)
	for _, p := range s.conf.Load().Auth.TrustedProxies {
		if p.Contains(ip) {
			return strings.TrimSpace(r.Header.Get(s.conf.Load().Auth.Header))
		}
	}
	return ""
}

func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.user(r) == "" {
			http.Error(w, "authentification requise", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			if !sameOrigin(r) {
				http.Error(w, "requête refusée (origine)", http.StatusForbidden)
				return
			}
			limit := int64(64 << 10)
			if r.URL.Path == "/identity/favicon" {
				limit = 1 << 20 // fichier envoyé (512 Ko au plus, vérifié ensuite)
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		h(w, r)
	}
}

// sameOrigin : un formulaire posté depuis un autre site est refusé
// (Sec-Fetch-Site des navigateurs actuels, sinon Origin comparé à Host).
func sameOrigin(r *http.Request) bool {
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
		return sfs == "same-origin"
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

func (s *Server) renderTile(v board.View) string {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, "tile", tileData{v, s.iconFor(v.Cfg)}); err != nil {
		log.Printf("rendu de la tuile %s : %v", v.ID, err)
		return ""
	}
	return buf.String()
}

func (s *Server) page(w http.ResponseWriter, name string, data any) {
	if m, ok := data.(map[string]any); ok {
		s.brand(m)
	}
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("page %s : %v", name, err)
		http.Error(w, "erreur de rendu", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

type sectionHTML struct {
	Name  string
	N     int // colonnes voulues (nombre de tuiles, plafonné à 8)
	Tiles []template.HTML
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	var secs []sectionHTML
	for _, sec := range s.board.Sections() {
		sh := sectionHTML{Name: sec.Name}
		for _, t := range sec.Tiles {
			sh.Tiles = append(sh.Tiles, template.HTML(s.board.HTML(t.Cfg.ID))) // rendu par html/template
		}
		sh.N = min(len(sh.Tiles), 8)
		secs = append(secs, sh)
	}
	notice := s.Notice()
	if m := s.takeFlash(s.user(r)); m != "" {
		notice = m // résultat d'une action (formulaire sans script), gardé côté serveur
	}
	s.page(w, "index", map[string]any{"Sections": secs, "Nav": "home", "Notice": notice, "Layout": s.settings.Page().Layout})
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	v, ok := s.board.View(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.page(w, "detailpage", map[string]any{"V": tileData{v, s.iconFor(v.Cfg)}, "Nav": "", "Notice": s.Notice()})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	ch, snapshot, cancel := s.board.Subscribe()
	defer cancel()
	fmt.Fprint(w, "retry: 3000\n\n")
	for _, f := range snapshot {
		w.Write(f)
	}
	if rc.Flush() != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second) // garde la connexion ouverte à travers les proxys
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.Write(msg)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}

type settingsTile struct {
	ID, Title, Section string
	Hidden             bool
	Fields             []settingsField
	Pos                int
	Actions            []settingsAction
	Icon               string // URL de l'icône retenue ("" si aucune)
	IconName           string // nom deviné ou imposé
	IconForced         string
	NoIcon             bool
	Link               linkForm
}

type settingsAction struct {
	tile.Action
	Placement tile.Placement
}

type settingsField struct {
	tile.Field
	Placement tile.Placement
	Order     int
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	var tiles []settingsTile
	for i, t := range s.board.All() {
		st := s.settings.Get(t.Cfg.ID)
		ts := settingsTile{ID: t.Cfg.ID, Title: t.Cfg.Title, Section: t.Cfg.Section, Hidden: st.Hidden, Pos: s.board.Pos(i, t.Cfg.ID),
			IconForced: st.Icon, NoIcon: st.NoIcon, Link: s.linkFor(t.Cfg, t.Cfg.Type)}
		if s.Icons != nil {
			ts.IconName = s.Icons.Guess(st.Icon, t.Cfg.Type, t.Cfg.Options["container"], t.Cfg.ID, t.Cfg.Title)
			if ts.IconName != "" {
				ts.Icon = "/icon/" + ts.IconName + ".png"
			}
		}
		for i, f := range st.Sort(t.Fields) {
			ts.Fields = append(ts.Fields, settingsField{Field: f, Placement: st.Placement(f), Order: (i + 1) * 10})
		}
		if a, ok := t.Src.(tile.Actor); ok {
			for _, act := range a.Actions() {
				ts.Actions = append(ts.Actions, settingsAction{act, st.ActionPlacement(act)})
			}
		}
		tiles = append(tiles, ts)
	}
	slices.SortStableFunc(tiles, func(a, b settingsTile) int { return cmp.Compare(a.Pos, b.Pos) })
	q := r.URL.Query()
	add := q.Get("add")
	var addForm *linkForm
	cfg := s.conf.Load()
	if integ.Known(add) {
		f := s.linkFor(prefill(add, cfg), add)
		f.New = true
		addForm = &f
	}
	s.page(w, "settings", map[string]any{"Tiles": tiles, "Nav": "settings", "Saved": q.Get("ok"), "Err": q.Get("err"),
		"Types": typeChoices(), "Layout": s.settings.Page().Layout, "Identity": s.identityForm(),
		"Gallery": s.gallery(), "Sections": sectionNames(cfg), "AddTitle": defaultOf(add).Title, "ShowGallery": q.Get("gallery") == "1", "Add": addForm, "AddType": add, "Writable": cfg.Writable() && s.admin(r), "ConfPath": cfg.Path(), "Notice": s.Notice()})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t := s.board.Tile(id)
	if t == nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	old := s.settings.Get(id)
	st := settings.Tile{Hidden: r.PostForm.Get("visible") == "", Fields: map[string]tile.Placement{},
		Icon: icons.Slug(r.PostForm.Get("icon")), NoIcon: r.PostForm.Get("showicon") == ""}
	if a, ok := t.Src.(tile.Actor); ok {
		st.Actions = map[string]tile.Placement{}
		for _, act := range a.Actions() {
			if p := tile.Placement(r.PostForm.Get("a." + act.ID)); p.Valid() && p != st.ActionPlacement(act) {
				st.Actions[act.ID] = p
			}
		}
	}
	if p, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("pos"))); err == nil && p > 0 && p < 1e6 {
		st.Pos = p
	}
	order := map[string]int{}
	for _, f := range t.Fields {
		if p := tile.Placement(r.PostForm.Get("f." + f.ID)); p.Valid() && p != f.Default {
			st.Fields[f.ID] = p
		}
		n, err := strconv.Atoi(r.PostForm.Get("o." + f.ID))
		if err != nil {
			n = 1 << 20
		}
		order[f.ID] = n
	}
	fields := append([]tile.Field(nil), t.Fields...)
	slices.SortStableFunc(fields, func(a, b tile.Field) int { return cmp.Compare(order[a.ID], order[b.ID]) })
	for _, f := range fields {
		st.Order = append(st.Order, f.ID)
	}
	if err := s.settings.Set(id, st); err != nil {
		log.Printf("réglages %s : %v", id, err)
		http.Error(w, "enregistrement impossible", http.StatusInternalServerError)
		return
	}
	s.board.Refresh(id)
	if st.Pos != old.Pos || st.Hidden != old.Hidden {
		s.board.Reload() // ordre ou liste des tuiles changés : accueils ouverts rechargés
	}
	http.Redirect(w, r, "/settings?ok="+url.QueryEscape(id)+"#s-"+id, http.StatusSeeOther)
}
