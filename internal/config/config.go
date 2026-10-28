// Package config lit config.yaml : l'infrastructure (adresses, secrets
// attendus). Les choix d'affichage vivent dans data/settings.json.
package config

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Listen     string `yaml:"listen"`
	DataDir    string `yaml:"data_dir"`
	SecretsDir string `yaml:"secrets_dir"`
	Auth       Auth   `yaml:"auth"`
	Tiles      []Tile `yaml:"tiles"`

	path string // fichier d'origine (édition depuis l'interface)
}

// Auth : l'utilisateur vient de l'en-tête posé par Authelia, cru seulement
// si la connexion arrive d'un proxy de confiance. Disabled sert au banc local.
type Auth struct {
	Header         string         `yaml:"header"`
	TrustedProxies []netip.Prefix `yaml:"trusted_proxies"`
	Disabled       bool           `yaml:"disabled"`
	Admins         []string       `yaml:"admins"` // seuls ceux-ci modifient les liaisons depuis l'interface (vide : tout utilisateur authentifié)
}

type Tile struct {
	ID          string            `yaml:"id"`
	Type        string            `yaml:"type"`
	Title       string            `yaml:"title,omitempty"`
	Section     string            `yaml:"section,omitempty"`
	Href        string            `yaml:"href,omitempty"`
	URL         string            `yaml:"url,omitempty"`
	InsecureTLS bool              `yaml:"insecure_tls,omitempty"` // certificat auto-signé (TrueNAS)
	Secrets     map[string]string `yaml:"secrets,omitempty"`      // rôle → nom du secret (secrets_dir, puis data_dir/secrets)
	Options     map[string]string `yaml:"options,omitempty"`
	Fields      []FieldMap        `yaml:"fields,omitempty"` // champs déclarés à la main (type customapi)
}

// FieldMap : un champ lu dans une réponse JSON quelconque.
type FieldMap struct {
	ID        string   `yaml:"id"`
	Label     string   `yaml:"label,omitempty"`
	Path      string   `yaml:"path"`                // chemin pointé : data.items.0.count, ou data.items.# pour une longueur
	Format    string   `yaml:"format,omitempty"`    // text (défaut), number, bytes, rate, percent, duration, date
	Placement string   `yaml:"placement,omitempty"` // tile (défaut) ou detail
	Warn      *float64 `yaml:"warn,omitempty"`      // seuils facultatifs (nombres)
	Crit      *float64 `yaml:"crit,omitempty"`
}

var idRe = regexp.MustCompile(`^[a-z0-9-]+$`)

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	c.path = path
	return c, nil
}

// parse décode et valide ; sert aussi à vérifier une édition avant écriture.
func parse(b []byte) (*Config, error) {
	c := &Config{Listen: ":8080", DataDir: "/data", SecretsDir: "/run/secrets", Auth: Auth{Header: "Remote-User"}}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if !c.Auth.Disabled && len(c.Auth.TrustedProxies) == 0 {
		return nil, errors.New("auth.trusted_proxies vide : déclarer le proxy (Traefik) ou auth.disabled: true pour un banc local")
	}
	seen := map[string]bool{}
	for i, t := range c.Tiles {
		if !idRe.MatchString(t.ID) || seen[t.ID] {
			return nil, fmt.Errorf("tuile %d : id %q invalide ou en double", i+1, t.ID)
		}
		seen[t.ID] = true
		if t.Title == "" {
			c.Tiles[i].Title = t.ID
		}
		if t.Section == "" {
			c.Tiles[i].Section = "Divers"
		}
	}
	return c, nil
}

// Secret lit un secret : d'abord les secrets Docker (secrets_dir), puis ceux
// saisis dans l'interface (data_dir/secrets). Le nom est un simple nom de
// fichier : une config ne peut rien lire ailleurs.
func (c *Config) Secret(name string) (string, error) {
	p, err := c.secretPath(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("secret %s illisible", name)
	}
	return strings.TrimSpace(string(b)), nil
}

var secretRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)

func (c *Config) secretPath(name string) (string, error) {
	if !secretRe.MatchString(name) {
		return "", fmt.Errorf("nom de secret invalide : %q", name)
	}
	for _, dir := range []string{c.SecretsDir, c.UISecretsDir()} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("secret %s absent", name)
}
