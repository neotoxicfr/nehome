package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const base = `# commentaire à garder
listen: ":8080"
auth:
  disabled: true
# les tuiles
tiles:
  - {id: a, type: link, title: A}
  - {id: b, type: link, title: B}
`

func setup(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	c.DataDir, c.SecretsDir = dir, filepath.Join(dir, "docker")
	return c
}

func TestUpsertDelete(t *testing.T) {
	c := setup(t)
	if err := c.UpsertTile(Tile{ID: "b", Type: "sonarr", Title: "B2", URL: "http://x", Secrets: map[string]string{"key": "b_key"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertTile(Tile{ID: "c", Type: "link"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteTile("a"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(c.Path())
	s := string(raw)
	if !strings.Contains(s, "# commentaire à garder") || !strings.Contains(s, "# les tuiles") {
		t.Fatalf("commentaires perdus :\n%s", s)
	}
	n, err := Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Tiles) != 2 || n.Tiles[0].ID != "b" || n.Tiles[0].Type != "sonarr" || n.Tiles[0].Secrets["key"] != "b_key" || n.Tiles[1].ID != "c" {
		t.Fatalf("tuiles : %+v", n.Tiles)
	}
	if err := c.DeleteTile("inconnue"); err == nil {
		t.Fatal("suppression d'une tuile absente acceptée")
	}
	// Une modification qui rendrait le fichier invalide est refusée, fichier intact.
	before, _ := os.ReadFile(c.Path())
	if err := c.UpsertTile(Tile{ID: "Mauvais Id", Type: "link"}); err == nil {
		t.Fatal("id invalide accepté")
	}
	after, _ := os.ReadFile(c.Path())
	if string(before) != string(after) {
		t.Fatal("fichier modifié malgré le refus")
	}
}

func TestSecrets(t *testing.T) {
	c := setup(t)
	os.MkdirAll(c.SecretsDir, 0o700)
	os.WriteFile(filepath.Join(c.SecretsDir, "docker_key"), []byte("d\n"), 0o600)
	if err := c.PutSecret("ui_key", "valeur"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(filepath.Join(c.UISecretsDir(), "ui_key"))
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("droits %v, attendu 0600", st.Mode().Perm())
	}
	if v, _ := c.Secret("ui_key"); v != "valeur" {
		t.Fatalf("secret interface : %q", v)
	}
	if v, _ := c.Secret("docker_key"); v != "d" {
		t.Fatalf("secret docker : %q", v)
	}
	if err := c.PutSecret("docker_key", "x"); err == nil {
		t.Fatal("écrasement d'un nom de secret Docker accepté")
	}
	for _, bad := range []string{"../x", ".cache", "a/b", ""} {
		if _, err := c.Secret(bad); err == nil {
			t.Fatalf("nom %q accepté", bad)
		}
	}
	names := c.SecretNames()
	if len(names) != 2 || names[0].Source != "docker" || names[1].Source != "interface" {
		t.Fatalf("noms : %+v", names)
	}
	s1 := c.SecretStamp("ui_key")
	c.PutSecret("ui_key", "autre valeur")
	if c.SecretStamp("ui_key") == s1 {
		t.Fatal("l'empreinte ne change pas avec le secret")
	}
}
