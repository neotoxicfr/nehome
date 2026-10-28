package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
)

// reloader applique config.yaml à chaud : une empreinte (fichier + secrets
// utilisés) est relevée régulièrement, la config n'est relue que si elle a
// changé, et seules les tuiles modifiées redémarrent.
type reloader struct {
	path   string
	b      *board.Board
	notice func(string) // bandeau affiché sur les pages ("" = rien)
	apply  func(*config.Config)

	cfg   atomic.Pointer[config.Config]
	mu    sync.Mutex
	stamp string
}

func (r *reloader) current() *config.Config { return r.cfg.Load() }

func (r *reloader) fingerprint(c *config.Config) string {
	var sb strings.Builder
	if st, err := os.Stat(r.path); err == nil {
		fmt.Fprintf(&sb, "%d/%d", st.ModTime().UnixNano(), st.Size())
	}
	for _, n := range secretNames(c) {
		sb.WriteString("|" + n + "=" + c.SecretStamp(n))
	}
	return sb.String()
}

func secretNames(c *config.Config) []string {
	var out []string
	for _, t := range c.Tiles {
		for _, n := range t.Secrets {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// specs : signature = définition de la tuile + état de ses secrets.
func specs(c *config.Config) []board.Spec {
	out := make([]board.Spec, 0, len(c.Tiles))
	for _, t := range c.Tiles {
		b, _ := json.Marshal(t)
		sig := string(b)
		names := make([]string, 0, len(t.Secrets))
		for _, n := range t.Secrets {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			sig += "|" + n + "=" + c.SecretStamp(n)
		}
		out = append(out, board.Spec{Cfg: t, Sig: sig})
	}
	return out
}

// check relit la configuration si elle a changé (toujours si force).
func (r *reloader) check(force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.current()
	if !force && r.fingerprint(cur) == r.stamp {
		return nil
	}
	c, err := config.Load(r.path)
	if err != nil {
		r.stamp = r.fingerprint(cur) // ne pas répéter l'erreur à chaque passage
		r.notice("config.yaml invalide, la configuration précédente reste active : " + err.Error())
		log.Print(err)
		return err
	}
	msg := ""
	if c.Listen != cur.Listen || c.DataDir != cur.DataDir {
		msg = "listen et data_dir ne changent qu'au redémarrage du conteneur"
		c.Listen, c.DataDir = cur.Listen, cur.DataDir
	}
	r.cfg.Store(c)
	r.apply(c)
	r.b.Apply(specs(c))
	r.stamp = r.fingerprint(c)
	r.notice(msg)
	return nil
}
