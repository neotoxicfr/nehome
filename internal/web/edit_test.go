package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
)

func TestEditLinks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	os.WriteFile(p, []byte("data_dir: "+dir+"\nsecrets_dir: "+dir+"/docker\nauth:\n  header: Remote-User\n  trusted_proxies: [127.0.0.0/8]\ntiles:\n  - {id: x, type: link, title: X}\n"), 0o644)
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := settings.Open(filepath.Join(dir, "settings.json"))
	b := board.New(nil, st)
	b.New = func(config.Tile) tile.Source { return fake{} }
	s := New(cfg, b, st)
	s.OnEdit = func() error { // comme le rechargeur : relire puis appliquer
		c, err := config.Load(p)
		if err != nil {
			return err
		}
		s.SetConfig(c)
		var sp []board.Spec
		for _, t := range c.Tiles {
			sp = append(sp, board.Spec{Cfg: t, Sig: t.Title + t.URL})
		}
		b.Apply(sp)
		return nil
	}
	b.Start(t.Context())
	s.OnEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	h := http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}}

	form := url.Values{"new": {"1"}, "id": {"son"}, "type": {"sonarr"}, "title": {"Sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.value": {"SUPERSECRET"}}
	if r := post(t, srv.URL+"/tiles", form, h); r.StatusCode != http.StatusSeeOther || !strings.Contains(r.Header.Get("Location"), "ok=son") {
		t.Fatalf("ajout : %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	c, _ := config.Load(p)
	if len(c.Tiles) != 2 || c.Tiles[1].Secrets["key"] != "son_key" || c.Tiles[1].URL != "http://sonarr:8989" {
		t.Fatalf("config après ajout : %+v", c.Tiles)
	}
	fi, err := os.Stat(filepath.Join(dir, "secrets", "son_key"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret non écrit en 0600 : %v", err)
	}
	if b.Tile("son") == nil {
		t.Fatal("tuile ajoutée non démarrée sans redémarrage")
	}
	_, page := get(t, srv.URL+"/settings", h)
	if strings.Contains(page, "SUPERSECRET") {
		t.Fatal("la clé apparaît dans la page")
	}
	if !strings.Contains(page, "son_key (interface)") {
		t.Fatal("le secret relié n'est pas indiqué")
	}

	// Adresse invalide refusée, rien écrit.
	bad := url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"javascript:alert(1)"}}
	if r := post(t, srv.URL+"/tiles", bad, h); !strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatal("adresse invalide acceptée")
	}
	// Formulaire venu d'un autre site refusé.
	if r := post(t, srv.URL+"/tiles", form, http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"cross-site"}}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("POST d'un autre site : %d", r.StatusCode)
	}
	// Suppression : confirmation obligatoire.
	if r := post(t, srv.URL+"/tiles/son/delete", url.Values{}, h); !strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatal("suppression sans confirmation acceptée")
	}
	post(t, srv.URL+"/tiles/son/delete", url.Values{"confirm": {"1"}}, h)
	if c, _ := config.Load(p); len(c.Tiles) != 1 || b.Tile("son") != nil {
		t.Fatal("liaison non supprimée")
	}
	// La page d'accueil ne casse pas sans icônes (magasin absent).
	r, _ := get(t, srv.URL+"/", h)
	io.Copy(io.Discard, r.Body)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("accueil : %d", r.StatusCode)
	}
}

func TestEditGuards(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	os.MkdirAll(filepath.Join(dir, "docker"), 0o700)
	os.WriteFile(filepath.Join(dir, "docker", "nas_key"), []byte("CLE-NAS\n"), 0o600)
	os.WriteFile(p, []byte("data_dir: "+dir+"\nsecrets_dir: "+dir+"/docker\nauth:\n  header: Remote-User\n  trusted_proxies: [127.0.0.0/8]\n  admins: [admin]\ntiles:\n  - {id: nas, type: truenas, url: 'https://nas:444', secrets: {key: nas_key}}\n  - {id: son, type: sonarr, url: 'http://sonarr:8989'}\n"), 0o644)
	cfg, _ := config.Load(p)
	st, _ := settings.Open(filepath.Join(dir, "settings.json"))
	b := board.New(nil, st)
	s := New(cfg, b, st)
	s.OnEdit = func() error { c, err := config.Load(p); s.SetConfig(c); return err }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	adm := http.Header{"Remote-User": {"admin"}, "Sec-Fetch-Site": {"same-origin"}}
	refused := func(form url.Values, h http.Header, why string) {
		t.Helper()
		r := post(t, srv.URL+"/tiles", form, h)
		if r.StatusCode == http.StatusSeeOther && !strings.Contains(r.Header.Get("Location"), "err=") {
			t.Fatalf("%s : accepté", why)
		}
	}
	// Relier la clé du NAS à Sonarr (adresse au choix) : refusé.
	refused(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.name": {"nas_key"}}, adm, "secret d'une autre tuile")
	refused(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "extra_secrets": {"key=nas_key"}}, adm, "secret d'une autre tuile (avancé)")
	// Garder la clé du NAS en changeant d'hôte : refusé ; avec la clé ressaisie : accepté.
	refused(url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://pirate.example:444"}, "sec.key.name": {"nas_key"}}, adm, "changement d'hôte sans ressaisie")
	if r := post(t, srv.URL+"/tiles", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://nas:444"}, "title": {"NAS"}, "sec.key.name": {"nas_key"}}, adm); strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatalf("même hôte, clé gardée : refusé (%s)", r.Header.Get("Location"))
	}
	// Une valeur saisie va toujours dans <id>_<rôle>, jamais dans le secret d'une autre tuile.
	post(t, srv.URL+"/tiles", url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.name": {"nas_key"}, "sec.key.value": {"CLE-SONARR"}}, adm)
	if v, _ := os.ReadFile(filepath.Join(dir, "docker", "nas_key")); string(v) != "CLE-NAS\n" {
		t.Fatal("secret d'une autre tuile écrasé")
	}
	if c, _ := config.Load(p); c.Tiles[1].Secrets["key"] != "son_key" {
		t.Fatalf("clé saisie mal rattachée : %+v", c.Tiles[1].Secrets)
	}
	// Hors auth.admins : modification interdite.
	if r := post(t, srv.URL+"/tiles", url.Values{"id": {"son"}, "type": {"sonarr"}}, http.Header{"Remote-User": {"invite"}, "Sec-Fetch-Site": {"same-origin"}}); r.StatusCode != http.StatusForbidden {
		t.Fatalf("utilisateur non admin : %d", r.StatusCode)
	}
}

func TestEditGuardsBypass(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	os.MkdirAll(filepath.Join(dir, "docker"), 0o700)
	os.WriteFile(filepath.Join(dir, "docker", "hp_nas"), []byte("CLE-NAS\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "docker", "son_key"), []byte("DOCKER\n"), 0o600) // secret Docker au nom « de la tuile »
	os.WriteFile(p, []byte("data_dir: "+dir+"\nsecrets_dir: "+dir+"/docker\nauth:\n  header: Remote-User\n  trusted_proxies: [127.0.0.0/8]\ntiles:\n  - {id: nas, type: truenas, url: 'https://nas:444', secrets: {key: hp_nas}}\n  - {id: hp, type: sonarr, url: 'http://sonarr:8989'}\n  - {id: son, type: sonarr, url: 'http://sonarr:8989'}\n"), 0o644)
	cfg, _ := config.Load(p)
	st, _ := settings.Open(filepath.Join(dir, "settings.json"))
	s := New(cfg, board.New(nil, st), st)
	s.OnEdit = func() error { c, err := config.Load(p); s.SetConfig(c); return err }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	h := http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}}
	for _, c := range []struct {
		why  string
		form url.Values
	}{
		{"préfixe : tuile « hp » vers hp_nas", url.Values{"id": {"hp"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.name": {"hp_nas"}}},
		{"secret Docker nommé comme la tuile", url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.name": {"son_key"}}},
		{"rôle libre vers la clé d'un autre rôle, hôte changé", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://pirate:444"}, "extra_secrets": {"autre=hp_nas"}}},
		{"protocole rétrogradé https→http", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"http://nas:444"}, "sec.key.name": {"hp_nas"}}},
		{"port changé", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://nas:8443"}, "sec.key.name": {"hp_nas"}}},
		{"vérification TLS coupée", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://nas:444"}, "insecure_tls": {"1"}, "sec.key.name": {"hp_nas"}}},
	} {
		r := post(t, srv.URL+"/tiles", c.form, h)
		if !strings.Contains(r.Header.Get("Location"), "err=") {
			t.Errorf("%s : accepté", c.why)
		}
	}
	// Même destination (casse et port implicite près) : la clé rattachée reste.
	if r := post(t, srv.URL+"/tiles", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://NAS:444"}, "sec.key.name": {"hp_nas"}}, h); strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatalf("même destination refusée : %s", r.Header.Get("Location"))
	}
	// Nouvelle destination avec clé ressaisie : accepté, clé propre à la tuile.
	if r := post(t, srv.URL+"/tiles", url.Values{"id": {"nas"}, "type": {"truenas"}, "url": {"https://nas2:444"}, "sec.key.value": {"NOUVELLE"}}, h); strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatalf("clé ressaisie refusée : %s", r.Header.Get("Location"))
	}
	if c, _ := config.Load(p); c.Tiles[0].Secrets["key"] != "nas_key" {
		t.Fatalf("clé ressaisie mal rattachée : %+v", c.Tiles[0].Secrets)
	}
}

// Contournement en plusieurs étapes : délier, changer d'adresse, relier le
// secret resté sur le disque.
func TestEditGuardsSteps(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	os.WriteFile(p, []byte("data_dir: "+dir+"\nsecrets_dir: "+dir+"/docker\nauth:\n  header: Remote-User\n  trusted_proxies: [127.0.0.0/8]\ntiles:\n  - {id: son, type: sonarr, url: 'http://sonarr:8989'}\n"), 0o644)
	cfg, _ := config.Load(p)
	st, _ := settings.Open(filepath.Join(dir, "settings.json"))
	s := New(cfg, board.New(nil, st), st)
	s.OnEdit = func() error { c, err := config.Load(p); s.SetConfig(c); return err }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	h := http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}}
	step := func(f url.Values) string { return post(t, srv.URL+"/tiles", f, h).Header.Get("Location") }
	if l := step(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}, "sec.key.value": {"CLE"}}); strings.Contains(l, "err=") {
		t.Fatal(l)
	}
	step(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://sonarr:8989"}})                                   // délier
	step(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://pirate:8989"}})                                   // changer d'adresse (aucune clé reliée)
	l := step(url.Values{"id": {"son"}, "type": {"sonarr"}, "url": {"http://pirate:8989"}, "sec.key.name": {"son_key"}}) // relier l'ancien secret
	if !strings.Contains(l, "err=") {
		t.Fatal("secret resté sur le disque relié vers la nouvelle adresse")
	}
	// Suppression : les secrets saisis pour la tuile disparaissent.
	post(t, srv.URL+"/tiles/son/delete", url.Values{"confirm": {"1"}}, h)
	if _, err := os.Stat(filepath.Join(dir, "secrets", "son_key")); err == nil {
		t.Fatal("secret de la tuile supprimée encore présent")
	}
}

func TestExtraKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	os.WriteFile(p, []byte("data_dir: "+dir+"\nsecrets_dir: "+dir+"/docker\nauth:\n  header: Remote-User\n  trusted_proxies: [127.0.0.0/8]\ntiles: []\n"), 0o644)
	cfg, _ := config.Load(p)
	st, _ := settings.Open(filepath.Join(dir, "settings.json"))
	s := New(cfg, board.New(nil, st), st)
	s.OnEdit = func() error { c, err := config.Load(p); s.SetConfig(c); return err }
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	h := http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}}
	if r := post(t, srv.URL+"/tiles", url.Values{"new": {"1"}, "id": {"dns"}, "type": {"pihole"}, "url": {"http://pihole"}, "xrole": {"pass"}, "xrole_value": {"MDP"}}, h); strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatal(r.Header.Get("Location"))
	}
	if c, _ := config.Load(p); c.Tiles[0].Secrets["pass"] != "dns_pass" {
		t.Fatalf("clé ajoutée : %+v", c.Tiles[0].Secrets)
	}
	if v, _ := os.ReadFile(filepath.Join(dir, "secrets", "dns_pass")); strings.TrimSpace(string(v)) != "MDP" {
		t.Fatal("valeur non enregistrée")
	}
	if r := post(t, srv.URL+"/tiles", url.Values{"id": {"dns"}, "type": {"pihole"}, "url": {"http://pihole"}, "xrole": {"custom"}, "xrole_custom": {"../x"}, "xrole_value": {"v"}}, h); !strings.Contains(r.Header.Get("Location"), "err=") {
		t.Fatal("nom de clé invalide accepté")
	}
}
