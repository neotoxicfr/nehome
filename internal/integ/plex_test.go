package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// mediaServer sert des JSON par chemin (ou « chemin?requête » si la requête compte)
// et refuse toute requête sans l'en-tête d'authentification attendu.
func mediaServer(t *testing.T, hdr, want string, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hdr != "" && r.Header.Get(hdr) != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := routes[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
}

func mediaSecret(role string) (string, error) { return "factice", nil }

func TestPlex(t *testing.T) {
	srv := mediaServer(t, "X-Plex-Token", "factice", map[string]string{
		"/status/sessions": `{"MediaContainer":{"size":3,"Metadata":[
{"type":"episode","title":"Limite et éveil","grandparentTitle":"Hunter x Hunter","parentIndex":2,"index":66,"viewOffset":861873,"duration":1364661,
 "User":{"title":"Alice"},"Player":{"title":"Téléphone","product":"Plex for Android (Mobile)","state":"playing"},"Session":{"bandwidth":1989,"location":"wan"},
 "Media":[{"Part":[{"decision":"transcode"}]}],
 "TranscodeSession":{"videoDecision":"transcode","audioDecision":"transcode","transcodeHwEncoding":"nvenc","transcodeHwFullPipeline":true}},
{"type":"episode","title":"Le shérif est mort","grandparentTitle":"Meurtres au paradis","parentIndex":1,"index":1,"viewOffset":249880,"duration":3490560,
 "User":{"title":"Bob"},"Player":{"title":"Apple TV","product":"Infuse","state":"paused"},"Session":{"bandwidth":10702},"Media":[{"Part":[{"decision":"directplay"}]}]},
{"type":"movie","title":"The Dog Stars","viewOffset":3335327,"duration":7136417,
 "User":{"title":"Carole"},"Player":{"title":"Fire TV","product":"Plex for Amazon FireTV","state":"playing"},"Session":{"bandwidth":12376},"Media":[{"Part":[{"decision":"directplay"}]}]}]}}`,
		"/identity":   `{"MediaContainer":{"version":"1.43.5.11079-b3b13e506"}}`,
		"/activities": `{"MediaContainer":{"size":0}}`,
		"/library/recentlyAdded?X-Plex-Container-Start=0&X-Plex-Container-Size=10": `{"MediaContainer":{"Metadata":[
{"type":"season","title":"Season 1","parentTitle":"Sous le masque","index":1,"librarySectionTitle":"Séries","addedAt":1793068409},
{"type":"movie","title":"Adaline","year":2015,"librarySectionTitle":"Films","addedAt":1793066763}]}}`,
		"/library/sections":        `{"MediaContainer":{"Directory":[{"key":"1","type":"movie","title":"Films"},{"key":"2","type":"show","title":"Séries"},{"key":"13","type":"photo","title":"Photos"}]}}`,
		"/library/sections/1/all":  `{"MediaContainer":{"totalSize":955}}`,
		"/library/sections/2/all":  `{"MediaContainer":{"totalSize":126}}`,
		"/library/sections/13/all": `{"MediaContainer":{"totalSize":65}}`,
	})
	defer srv.Close()
	src, err := newPlex(config.Tile{URL: srv.URL}, mediaSecret)
	if err != nil {
		t.Fatal(err)
	}
	p := src.(*plex)
	v, err := p.sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["streams"].Text != "3" || !strings.Contains(v["streams"].Sub, "2 directe(s), 1 transcodage(s) matériel, 0 logiciel") {
		t.Fatalf("flux : %+v", v["streams"])
	}
	// 1989 + 10702 + 12376 kbit/s = 25067 kbit/s = 3,1 Mo/s
	if v["bandwidth"].Text != "3,1 Mo/s" {
		t.Fatalf("débit : %q", v["bandwidth"].Text)
	}
	rows := v["sessions"].Rows
	if rows[0][1].Text != "Hunter x Hunter S02E66" || rows[0][2].Text != "63,2 %" || rows[0][4].Text != "Transcodage matériel" || rows[1][2].Text != "7,2 % (pause)" || rows[2][1].Text != "The Dog Stars" {
		t.Fatalf("lignes : %+v", rows)
	}
	v, err = p.slow(t.Context())
	if err != nil || v["version"].Text != "1.43.5.11079" || v["activities"].Text != "aucune" || len(v["recent"].Rows) != 2 || v["recent"].Rows[0][0].Text != "Sous le masque, saison 1" || v["recent"].Rows[1][0].Text != "Adaline (2015)" {
		t.Fatalf("lent : %v %+v", err, v)
	}
	v, err = p.libraries(t.Context())
	if err != nil || v["movies"].Text != "955" || v["shows"].Text != "126" || len(v["libraries"].Rows) != 3 {
		t.Fatalf("bibliothèques : %v %+v", err, v)
	}

	bad, _ := newPlex(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*plex).sessions(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue sans fuite du jeton : %v", err)
	}
}
