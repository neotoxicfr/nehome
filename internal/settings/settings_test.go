package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// L'ancien format (tuiles seules) doit rester lisible après la mise à jour.
func TestLegacy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, []byte(`{"plex":{"hidden":true}}`), 0o644)
	s, err := Open(p)
	if err != nil || !s.Get("plex").Hidden {
		t.Fatalf("ancien format perdu : %v %+v", err, s.Get("plex"))
	}
	if err := s.SetPage(Page{Layout: "auto"}); err != nil {
		t.Fatal(err)
	}
	s2, _ := Open(p)
	if !s2.Get("plex").Hidden || s2.Page().Layout != "auto" {
		t.Fatalf("relecture : %+v %+v", s2.Get("plex"), s2.Page())
	}
}
