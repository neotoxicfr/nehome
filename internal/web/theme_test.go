package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/integ"
	"github.com/neotoxicfr/nehome/internal/settings"
)

func TestIdentity(t *testing.T) {
	srv, st := setup(t, trusted)
	h := http.Header{"Remote-User": {"neo"}, "Sec-Fetch-Site": {"same-origin"}}
	// Couleur libre piégée : rejetée.
	post(t, srv.URL+"/identity", url.Values{"name": {"x"}, "theme": {"sombre"}, "accent": {"custom"}, "accent_custom": {"red;}body{background:url(//x)"}}, h)
	if p := st.Page(); p.Accent != "" {
		t.Fatalf("couleur piégée enregistrée : %+v", p)
	}
	form := url.Values{"name": {"<b>Maison</b>"}, "theme": {"clair"}, "accent": {"jaune"}}
	if r := post(t, srv.URL+"/identity", form, h); r.StatusCode != http.StatusSeeOther {
		t.Fatalf("identité : %d", r.StatusCode)
	}
	if p := st.Page(); p.Accent != "#e0b44c" || p.Theme != "clair" {
		t.Fatalf("thème enregistré : %+v", p)
	}
	_, css := get(t, srv.URL+"/theme.css", h)
	if strings.Contains(css, "url(") || !strings.Contains(css, "color-scheme:light") || strings.Contains(css, "--accent:#e0b44c") {
		t.Fatalf("theme.css (accent pâle à foncer sur fond clair) : %s", css)
	}
	_, page := get(t, srv.URL+"/", h)
	if strings.Contains(page, "<b>Maison</b>") || !strings.Contains(page, "&lt;b&gt;Maison&lt;/b&gt;") {
		t.Fatal("nom non échappé")
	}
	// Enregistrer la disposition ne remet pas l'identité à zéro.
	post(t, srv.URL+"/page", url.Values{"layout": {"auto"}}, h)
	if q := st.Page(); q.Layout != "auto" || q.Accent != "#e0b44c" || q.Name == "" {
		t.Fatalf("identité perdue : %+v", q)
	}
}

func TestOnAccent(t *testing.T) {
	if paletteOf(settings.Page{Accent: "#ffff00"}).OnAccent != "#0b0f17" || paletteOf(settings.Page{Accent: "#1a237e"}).OnAccent != "#ffffff" {
		t.Fatal("texte sur l'accent illisible")
	}
}

func TestFaviconUpload(t *testing.T) {
	srv, _ := setup(t, trusted)
	send := func(name string, body []byte, extra map[string]string) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range extra {
			mw.WriteField(k, v)
		}
		if body != nil {
			fw, _ := mw.CreateFormFile("favicon", name)
			fw.Write(body)
		}
		mw.Close()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/identity/favicon", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Remote-User", "neo")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("X-Requested-With", "fetch")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	if r := send("logo.png", png, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("PNG refusé : %d", r.StatusCode)
	}
	h := http.Header{"Remote-User": {"neo"}}
	if r, _ := get(t, srv.URL+"/favicon", h); r.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("favicon servi en %q", r.Header.Get("Content-Type"))
	}
	for _, c := range []struct {
		name string
		body []byte
	}{{"x.svg", []byte(`<svg onload="alert(1)"/>`)}, {"x.png", []byte("<html><script>alert(1)</script>")}, {"gros.png", append(png, make([]byte, 600<<10)...)}} {
		if r := send(c.name, c.body, nil); r.StatusCode == http.StatusOK {
			t.Fatalf("%s accepté", c.name)
		}
	}
	if r, _ := get(t, srv.URL+"/favicon", h); r.Header.Get("Content-Type") != "image/png" {
		t.Fatal("le favicon valide a été remplacé par un envoi refusé")
	}
	send("", nil, map[string]string{"remove": "1"})
	if r, _ := get(t, srv.URL+"/favicon", h); r.StatusCode != http.StatusNotFound {
		t.Fatalf("favicon non retiré : %d", r.StatusCode)
	}
}

// Chaque type connu a sa catégorie et son titre dans la galerie d'ajout.
func TestDefaultsCoverTypes(t *testing.T) {
	for _, typ := range integ.Types() {
		if _, ok := defaults[typ]; !ok {
			t.Errorf("type %q absent de la table des valeurs par défaut (galerie, préremplissage)", typ)
		}
	}
}
