package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// f4Serve sert des réponses fixes par chemin (avec ou sans requête). ok
// décide si la requête est autorisée (401 sinon) ; hdr s'ajoute à chaque réponse.
func f4Serve(t *testing.T, ok func(*http.Request) bool, hdr map[string]string, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok != nil && !ok(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, found := routes[r.URL.RequestURI()]
		if !found {
			body, found = routes[r.URL.Path]
		}
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.Write([]byte(body))
	}))
}

func f4Hdr(name, want string) func(*http.Request) bool {
	return func(r *http.Request) bool { return r.Header.Get(name) == want }
}
