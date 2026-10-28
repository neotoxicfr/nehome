package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"go.yaml.in/yaml/v3"
)

// UISecretsDir : secrets saisis dans l'interface (fichiers 0600).
func (c *Config) UISecretsDir() string { return filepath.Join(c.DataDir, "secrets") }

// Path : fichier de configuration chargé.
func (c *Config) Path() string { return c.path }

// Writable : l'interface peut-elle modifier config.yaml ? (faux sur un montage :ro)
func (c *Config) Writable() bool {
	return c.path != "" && syscall.Access(c.path, 2) == nil && syscall.Access(filepath.Dir(c.path), 2) == nil
}

// SecretInfo : un secret connu, sans sa valeur.
type SecretInfo struct {
	Name, Source string // Source : "docker" ou "interface"
}

// SecretNames liste les secrets disponibles (noms seulement, jamais les valeurs).
func (c *Config) SecretNames() []SecretInfo {
	var out []SecretInfo
	seen := map[string]bool{}
	for _, d := range []struct{ dir, src string }{{c.SecretsDir, "docker"}, {c.UISecretsDir(), "interface"}} {
		ents, _ := os.ReadDir(d.dir)
		for _, e := range ents {
			if e.Type().IsRegular() && secretRe.MatchString(e.Name()) && !seen[e.Name()] {
				seen[e.Name()] = true
				out = append(out, SecretInfo{e.Name(), d.src})
			}
		}
	}
	slices.SortFunc(out, func(a, b SecretInfo) int { return cmpStr(a.Name, b.Name) })
	return out
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// SecretStamp change quand le fichier du secret change (rotation, création) :
// le tableau redémarre alors les tuiles qui l'utilisent.
func (c *Config) SecretStamp(name string) string {
	p, err := c.secretPath(name)
	if err != nil {
		return "absent"
	}
	st, err := os.Stat(p)
	if err != nil {
		return "absent"
	}
	return fmt.Sprintf("%d/%d", st.ModTime().UnixNano(), st.Size())
}

// PutSecret enregistre un secret saisi dans l'interface. Un secret Docker du
// même nom reste prioritaire : on refuse pour ne pas créer de doublon trompeur.
func (c *Config) PutSecret(name, value string) error {
	if !secretRe.MatchString(name) {
		return fmt.Errorf("nom de secret invalide : %q", name)
	}
	if _, err := os.Stat(filepath.Join(c.SecretsDir, name)); err == nil {
		return fmt.Errorf("%s est déjà un secret Docker : le modifier côté hôte", name)
	}
	dir := c.UISecretsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, name), []byte(value+"\n"), 0o600)
}

// UpsertTile remplace la tuile de même id, ou l'ajoute à la fin. Le fichier
// est modifié via son arbre YAML (commentaires et style gardés), revalidé,
// puis remplacé de façon atomique.
func (c *Config) UpsertTile(t Tile) error {
	return c.editTiles(func(seq *yaml.Node) error {
		var n yaml.Node
		if err := n.Encode(t); err != nil {
			return err
		}
		n.Style = yaml.FlowStyle
		if i := tileIndex(seq, t.ID); i >= 0 {
			seq.Content[i] = &n
		} else {
			seq.Content = append(seq.Content, &n)
		}
		return nil
	})
}

// DeleteTile retire la tuile id.
func (c *Config) DeleteTile(id string) error {
	return c.editTiles(func(seq *yaml.Node) error {
		i := tileIndex(seq, id)
		if i < 0 {
			return fmt.Errorf("tuile %q introuvable", id)
		}
		seq.Content = slices.Delete(seq.Content, i, i+1)
		return nil
	})
}

func tileIndex(seq *yaml.Node, id string) int {
	for i, it := range seq.Content {
		if it.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(it.Content); j += 2 {
			if it.Content[j].Value == "id" && it.Content[j+1].Value == id {
				return i
			}
		}
	}
	return -1
}

func (c *Config) editTiles(f func(seq *yaml.Node) error) error {
	if !c.Writable() {
		return errors.New("config.yaml n'est pas modifiable (montage en lecture seule ?)")
	}
	b, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	var seq *yaml.Node
	for j := 0; j+1 < len(root.Content); j += 2 {
		if root.Content[j].Value == "tiles" {
			seq = root.Content[j+1]
		}
	}
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "tiles"}, seq)
	}
	if err := f(seq); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // style du fichier d'origine
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	enc.Close()
	out := buf.Bytes()
	if _, err := parse(out); err != nil {
		return fmt.Errorf("modification refusée : %w", err)
	}
	return writeAtomic(c.path, out, 0o644)
}

// writeAtomic : fichier temporaire dans le même dossier puis renommage.
func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nehome-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// DropUISecrets efface les secrets « <id>_… » saisis dans l'interface pour une
// tuile supprimée (jamais les secrets Docker).
func (c *Config) DropUISecrets(id string) {
	ents, _ := os.ReadDir(c.UISecretsDir())
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), id+"_") && e.Type().IsRegular() {
			os.Remove(filepath.Join(c.UISecretsDir(), e.Name()))
		}
	}
}
