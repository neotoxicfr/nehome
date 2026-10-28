package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/integ"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// tileData : vue d'une tuile + URL de son icône pour le gabarit.
type tileData struct {
	board.View
	Icon string
}

// SetConfig remplace la configuration (relecture à chaud).
func (s *Server) SetConfig(c *config.Config) { s.conf.Store(c) }

// SetNotice affiche un bandeau sur les pages ("" l'efface).
func (s *Server) SetNotice(msg string) { s.notice.Store(msg) }

func (s *Server) Notice() string { m, _ := s.notice.Load().(string); return m }

func (s *Server) iconFor(c config.Tile) string {
	if s.Icons == nil {
		return ""
	}
	st := s.settings.Get(c.ID)
	if st.NoIcon {
		return ""
	}
	if n := s.Icons.Guess(st.Icon, c.Type, c.Options["container"], c.ID, c.Title); n != "" {
		return "/icon/" + n + ".png"
	}
	return ""
}

func (s *Server) icon(w http.ResponseWriter, r *http.Request) {
	if s.Icons == nil {
		http.NotFound(w, r)
		return
	}
	s.Icons.ServeHTTP(w, r)
}

// linkForm : formulaire d'une liaison (ajout ou modification).
type linkForm struct {
	New          bool
	Cfg          config.Tile
	Type, Desc   string
	Options      []kv // options connues du type, pré-remplies
	ExtraOptions string
	Roles        []roleForm
	ExtraSecrets string
	Secrets      []config.SecretInfo
	HasFields    bool // champs customapi : à éditer dans config.yaml
}

type kv struct{ Key, Value string }

type roleForm struct {
	Role, Label, Name, Source string // nom du secret relié et son origine
}

// roleLabels : nom lisible des rôles de secrets courants.
var roleLabels = map[string]string{"key": "Clé API", "token": "Jeton", "user": "Identifiant", "pass": "Mot de passe", "secret": "Secret API"}

func roleLabel(r string) string {
	if l, ok := roleLabels[r]; ok {
		return l
	}
	return r
}

// optLabels : libellé des options courantes (le nom brut reste en infobulle).
var optLabels = map[string]string{
	"container": "Conteneur", "docker_url": "Proxy Docker", "health_url": "URL de santé", "protect": "Conteneurs protégés",
	"update_log": "Journal des mises à jour", "post_url": "URL du bouton (POST)", "post_label": "Libellé du bouton",
	"device": "Appareil", "slugs": "Pages de statut", "topic": "Sujet", "user": "Utilisateur", "wan": "Interface WAN",
	"zone": "Zone", "zone_name": "Domaine", "lat": "Latitude", "lon": "Longitude", "timezone": "Fuseau horaire",
	"site": "Site", "unifi_os": "UniFi OS", "interval": "Intervalle", "targets": "Cibles", "host": "Hôte", "port": "Port",
	"disks": "Disques", "entities": "Entités", "method": "Méthode", "key_header": "En-tête de la clé",
}

func optLabel(k string) string {
	if l, ok := optLabels[k]; ok {
		return l
	}
	return k
}

type typeChoice struct{ Type, Desc string }

func typeChoices() []typeChoice {
	var out []typeChoice
	for _, t := range integ.Types() {
		out = append(out, typeChoice{t, integ.SpecOf(t).Desc})
	}
	return out
}

// allowedSecret : sans ressaisie, un rôle ne garde que le secret qui lui est
// relié en ce moment dans config.yaml. Rien d'autre (ni préfixe, ni secret
// laissé sur le disque, ni autre secret Docker) : sinon un compte de
// l'interface pourrait envoyer une clé vers une adresse de son choix, au
// besoin en plusieurs étapes. Relier un autre secret se fait dans config.yaml.
func allowedSecret(c config.Tile, role, name string) bool {
	return name != "" && name == c.Secrets[role]
}

// sameDest : même protocole, hôte et port effectif (comparaison sans casse).
func sameDest(a, b string) bool {
	pa, ea := url.Parse(a)
	pb, eb := url.Parse(b)
	if ea != nil || eb != nil {
		return a == b
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		return map[string]string{"http": "80", "https": "443"}[strings.ToLower(u.Scheme)]
	}
	return strings.EqualFold(pa.Scheme, pb.Scheme) && strings.EqualFold(pa.Hostname(), pb.Hostname()) && port(pa) == port(pb)
}

func (s *Server) linkFor(c config.Tile, typ string) linkForm {
	sp := integ.SpecOf(typ)
	cfg := s.conf.Load()
	f := linkForm{Cfg: c, Type: typ, Desc: sp.Desc, HasFields: len(c.Fields) > 0}
	for _, si := range cfg.SecretNames() {
		for _, r := range sp.Secrets {
			if c.ID != "" && allowedSecret(c, r, si.Name) {
				f.Secrets = append(f.Secrets, si)
				break
			}
		}
	}
	src := map[string]string{}
	for _, si := range f.Secrets {
		src[si.Name] = si.Source
	}
	known := map[string]bool{}
	for _, o := range sp.Options {
		known[o] = true
		f.Options = append(f.Options, kv{o, c.Options[o]})
	}
	var extra []string
	for k, v := range c.Options {
		if !known[k] {
			extra = append(extra, k+"="+v)
		}
	}
	slices.Sort(extra)
	f.ExtraOptions = strings.Join(extra, "\n")
	roles := map[string]bool{}
	for _, r := range sp.Secrets {
		roles[r] = true
		f.Roles = append(f.Roles, roleForm{r, roleLabel(r), c.Secrets[r], src[c.Secrets[r]]})
	}
	var xs []string
	for r, n := range c.Secrets {
		if !roles[r] {
			xs = append(xs, r+"="+n)
		}
	}
	slices.Sort(xs)
	f.ExtraSecrets = strings.Join(xs, "\n")
	return f
}

var (
	idRe   = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	roleRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
	optRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{0,60}$`)
)

func back(w http.ResponseWriter, r *http.Request, id string, err error) {
	if err != nil {
		http.Redirect(w, r, "/settings?err="+url.QueryEscape(err.Error())+"#liaisons", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?ok="+url.QueryEscape(id)+"#s-"+id, http.StatusSeeOther)
}

// saveTile crée ou modifie une liaison dans config.yaml, enregistre les clés
// saisies (écriture seule) et relit la configuration.
func (s *Server) saveTile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	p := r.PostForm
	id := strings.TrimSpace(p.Get("id"))
	err := s.applyTile(id, p.Get("new") == "1", p)
	if err != nil {
		log.Printf("liaison %s : %v", id, err)
	}
	back(w, r, id, err)
}

func (s *Server) applyTile(id string, isNew bool, p url.Values) error {
	cfg := s.conf.Load()
	if !idRe.MatchString(id) {
		return errors.New("identifiant : minuscules, chiffres et tirets (40 caractères au plus)")
	}
	var t config.Tile
	exists := false
	for _, c := range cfg.Tiles {
		if c.ID == id {
			t, exists = c, true
		}
	}
	if isNew && exists {
		return fmt.Errorf("l'identifiant %q existe déjà", id)
	}
	if !isNew && !exists {
		return fmt.Errorf("liaison %q introuvable", id)
	}
	typ := strings.TrimSpace(p.Get("type"))
	if !integ.Known(typ) {
		return fmt.Errorf("type d'intégration inconnu : %q", typ)
	}
	old := t // pour comparer l'hôte et les secrets déjà rattachés
	old.ID = id
	t.ID, t.Type = id, typ
	t.Title = strings.TrimSpace(p.Get("title"))
	t.Section = strings.TrimSpace(p.Get("section"))
	t.InsecureTLS = p.Get("insecure_tls") == "1"
	for _, f := range []struct {
		dst  *string
		name string
	}{{&t.Href, "href"}, {&t.URL, "url"}} {
		v := strings.TrimSpace(p.Get(f.name))
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("%s : adresse http(s) attendue", f.name)
			}
		}
		*f.dst = v
	}
	// Options : celles connues du type, puis « clé=valeur » libres.
	opts := map[string]string{}
	for _, o := range integ.SpecOf(typ).Options {
		if v := strings.TrimSpace(p.Get("opt." + o)); v != "" {
			opts[o] = v
		}
	}
	for _, l := range strings.Split(p.Get("extra_opts"), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); !optRe.MatchString(k) {
			return fmt.Errorf("option %q : nom invalide", k)
		}
		opts[k] = strings.TrimSpace(v)
	}
	t.Options = opts
	// Secrets : clé saisie (écrite dans data/secrets, jamais réaffichée) ou
	// secret existant choisi ; rôles libres « rôle=nom ».
	// Nouvelle destination (protocole, hôte, port) ou vérification TLS coupée :
	// toute clé doit être ressaisie dans cette requête, aucune clé existante
	// n'y est envoyée.
	destChanged := !sameDest(old.URL, t.URL) || (!old.InsecureTLS && t.InsecureTLS)
	secs := map[string]string{}
	entered := map[string]bool{}
	bind := func(rr, name string) error {
		if name == "" {
			return nil
		}
		if entered[rr] && name == id+"_"+rr {
			secs[rr] = name // valeur saisie à l'instant pour cette tuile
			return nil
		}
		if destChanged {
			return fmt.Errorf("nouvelle destination (protocole, hôte, port ou TLS) : ressaisir la clé « %s » ; une clé existante n'y est jamais envoyée", rr)
		}
		if !allowedSecret(old, rr, name) {
			return fmt.Errorf("secret %q : depuis l'interface, seul le secret déjà relié à ce rôle peut être gardé (sinon, saisir la clé) ; relier un autre secret se fait dans config.yaml", name)
		}
		secs[rr] = name
		return nil
	}
	for _, rr := range integ.SpecOf(typ).Secrets {
		name := strings.TrimSpace(p.Get("sec." + rr + ".name"))
		if v := strings.TrimSpace(p.Get("sec." + rr + ".value")); v != "" {
			name = id + "_" + rr // toujours le secret de cette tuile : jamais celui d'une autre
			if err := cfg.PutSecret(name, v); err != nil {
				return err
			}
			entered[rr] = true
		}
		if err := bind(rr, name); err != nil {
			return err
		}
	}
	// « Autre clé » : type choisi (clé API, jeton, identifiant, mot de passe) ou nom libre.
	if xr := strings.TrimSpace(p.Get("xrole")); xr != "" {
		if xr == "custom" {
			xr = strings.TrimSpace(p.Get("xrole_custom"))
		}
		v := strings.TrimSpace(p.Get("xrole_value"))
		if !roleRe.MatchString(xr) {
			return fmt.Errorf("type de clé %q : minuscules, chiffres et _", xr)
		}
		if v == "" {
			return fmt.Errorf("« %s » : valeur manquante", roleLabel(xr))
		}
		name := id + "_" + xr
		if err := cfg.PutSecret(name, v); err != nil {
			return err
		}
		entered[xr] = true
		if err := bind(xr, name); err != nil {
			return err
		}
	}
	for _, l := range strings.Split(p.Get("extra_secrets"), "\n") {
		rr, n, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		rr, n = strings.TrimSpace(rr), strings.TrimSpace(n)
		if !roleRe.MatchString(rr) {
			return fmt.Errorf("rôle %q : nom invalide", rr)
		}
		if err := bind(rr, n); err != nil {
			return err
		}
	}
	t.Secrets = secs
	if err := cfg.UpsertTile(t); err != nil {
		return err
	}
	return s.reload()
}

// admin : peut modifier les liaisons (auth.admins vide : tout utilisateur authentifié).
func (s *Server) admin(r *http.Request) bool {
	a := s.conf.Load().Auth.Admins
	return len(a) == 0 || slices.Contains(a, s.user(r))
}

// adminOnly protège les routes qui modifient config.yaml ou les secrets.
func (s *Server) adminOnly(h http.HandlerFunc) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(r) {
			http.Error(w, "réservé aux administrateurs (auth.admins)", http.StatusForbidden)
			return
		}
		h(w, r)
	})
}

func (s *Server) reload() error {
	if s.OnEdit == nil {
		return nil
	}
	return s.OnEdit()
}

// deleteTile retire une liaison (case de confirmation obligatoire).
func (s *Server) deleteTile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("confirm") != "1" {
		back(w, r, id, errors.New("suppression non confirmée : cocher la case « Confirmer »"))
		return
	}
	cfg := s.conf.Load()
	err := cfg.DeleteTile(id)
	if err == nil {
		cfg.DropUISecrets(id) // secrets saisis pour cette tuile : rien ne doit traîner
		err = s.reload()
	}
	if err != nil {
		back(w, r, id, err)
		return
	}
	http.Redirect(w, r, "/settings?ok=suppr#liaisons", http.StatusSeeOther)
}

// savePage enregistre la disposition de l'accueil.
func (s *Server) savePage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	layout := ""
	if r.PostForm.Get("layout") == "auto" {
		layout = "auto"
	}
	p := s.settings.Page()
	p.Layout = layout // les autres réglages de page (identité) restent
	if err := s.settings.SetPage(p); err != nil {
		back(w, r, "", err)
		return
	}
	s.board.Reload()
	http.Redirect(w, r, "/settings?ok=page#page", http.StatusSeeOther)
}

type identityForm struct {
	Name, Theme, Accent, AccentName string
	Themes                          []theme
	Accents                         []struct{ Name, Label, Hex string }
	FaviconURL                      string
}

func (s *Server) identityForm() identityForm {
	p := s.settings.Page()
	t := themeOf(p.Theme)
	acc := color(p.Accent, t.Acc)
	f := identityForm{Name: p.Name, Theme: t.Name, Accent: acc, AccentName: "custom", Themes: themes, Accents: accents, FaviconURL: s.faviconURL()}
	for _, a := range accents {
		if a.Hex == acc {
			f.AccentName = a.Name
		}
	}
	return f
}

// saveIdentity : nom, thème, couleurs, favicon. Une couleur égale à celle du
// thème n'est pas enregistrée : changer de thème la fait suivre.
func (s *Server) saveIdentity(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	f := r.PostForm
	p := s.settings.Page()
	p.Name = strings.TrimSpace(f.Get("name"))
	if len([]rune(p.Name)) > 40 {
		p.Name = string([]rune(p.Name)[:40])
	}
	p.Theme = themeOf(f.Get("theme")).Name
	acc := ""
	for _, a := range accents {
		if a.Name == f.Get("accent") {
			acc = a.Hex
		}
	}
	if f.Get("accent") == "custom" && hexRe.MatchString(f.Get("accent_custom")) {
		acc = strings.ToLower(f.Get("accent_custom"))
	}
	p.Accent = acc
	p.Bg, p.Surface, p.Text = "", "", "" // le mode fixe fond, cartes et texte
	if err := s.settings.SetPage(p); err != nil {
		back(w, r, "", err)
		return
	}
	s.brandEvent()                                   // pages ouvertes : nouveau thème appliqué sans rechargement
	if r.Header.Get("X-Requested-With") == "fetch" { // enregistrement automatique depuis la page
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/settings?ok=identity#apparence", http.StatusSeeOther)
}

// action exécute une action d'intégration (administrateurs seulement), avec
// confirmation exigée par le serveur pour les actions sensibles.
func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	tileID, act := r.PathValue("tile"), r.PathValue("action")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulaire illisible", http.StatusBadRequest)
		return
	}
	arg := r.PostForm.Get("arg")
	msg, err := s.runAction(r, tileID, act, arg)
	log.Printf("action %s/%s (%q) par %s : %v %s", tileID, act, arg, s.user(r), err, msg)
	if r.Header.Get("X-Requested-With") == "fetch" {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"message": msg})
		return
	}
	if err != nil {
		msg = "Échec : " + err.Error()
	}
	s.putFlash(s.user(r), msg)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type flashMsg struct {
	text string
	at   time.Time
}

// putFlash / takeFlash : message affiché une fois, à son auteur, dans la minute.
// Jamais dans l'URL : un lien forgé ne peut pas faire afficher un texte arbitraire.
func (s *Server) putFlash(user, msg string) {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	if s.flash == nil {
		s.flash = map[string]flashMsg{}
	}
	s.flash[user] = flashMsg{msg, time.Now()}
}

func (s *Server) takeFlash(user string) string {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	f, ok := s.flash[user]
	delete(s.flash, user)
	if !ok || time.Since(f.at) > time.Minute {
		return ""
	}
	return f.text
}

func (s *Server) runAction(r *http.Request, tileID, act, arg string) (string, error) {
	t := s.board.Tile(tileID)
	if t == nil {
		return "", fmt.Errorf("tuile %q introuvable", tileID)
	}
	if a, ok := t.Src.(tile.Actor); ok {
		for _, x := range a.Actions() {
			if x.ID == act && x.Confirm != "" && r.PostForm.Get("confirmed") != "1" {
				return "", errors.New("confirmation requise")
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	return s.board.Do(ctx, tileID, act, arg)
}

// addForm renvoie le formulaire prérempli d'un service (fragment chargé dans la fenêtre d'ajout).
func (s *Server) addForm(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("add")
	if !integ.Known(typ) {
		http.NotFound(w, r)
		return
	}
	f := s.linkFor(prefill(typ, s.conf.Load()), typ)
	f.New = true
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, "linkform", f); err != nil {
		log.Printf("formulaire %s : %v", typ, err)
		http.Error(w, "erreur de rendu", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
