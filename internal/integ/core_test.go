package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/tile"
)

// La réussite d'une boucle ne doit pas effacer la panne d'une autre.
func TestSplit(t *testing.T) {
	var last error
	e := split(func(_ tile.Values, err error) { last = err }, 2)
	e[1](nil, errStr("prévisions en panne"))
	e[0](tile.Values{"heure": {Text: "12:00"}}, nil)
	if last == nil || !strings.Contains(last.Error(), "prévisions") {
		t.Fatalf("erreur effacée par l'autre boucle : %v", last)
	}
	e[1](tile.Values{}, nil)
	if last != nil {
		t.Fatalf("erreur non levée après rétablissement : %v", last)
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }

// Une redirection vers un autre hôte ne doit pas emporter la clé.
func TestRedirectOtherHost(t *testing.T) {
	got := ""
	tiers := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Header.Get("X-Api-Key") }))
	defer tiers.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(tiers.URL, "127.0.0.1", "localhost", 1)+"/vol", http.StatusFound)
	}))
	defer srv.Close()
	var out any
	err := getJSON(t.Context(), httpClient(false, false), srv.URL, &out, func(r *http.Request) { r.Header.Set("X-Api-Key", "factice") })
	if err == nil || got != "" {
		t.Fatalf("redirection suivie (err=%v, clé reçue=%q)", err, got)
	}
}
