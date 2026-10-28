// nehome : tableau de bord d'accueil, intégrations API côté serveur, temps réel par SSE.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/neotoxicfr/nehome/internal/board"
	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/icons"
	"github.com/neotoxicfr/nehome/internal/integ"
	"github.com/neotoxicfr/nehome/internal/settings"
	"github.com/neotoxicfr/nehome/internal/tile"
	"github.com/neotoxicfr/nehome/internal/web"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", envOr("NEHOME_CONFIG", "/config/config.yaml"), "fichier de configuration")
	flag.Parse()
	var err error
	switch flag.Arg(0) {
	case "", "serve":
		err = serve(*cfgPath)
	case "healthcheck":
		err = healthcheck(*cfgPath)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("commande inconnue %q (serve, healthcheck, version)", flag.Arg(0))
	}
	if err != nil {
		log.Fatal(err)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func serve(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if cfg.Auth.Disabled {
		log.Print("ATTENTION : auth.disabled, n'importe qui joignant le port voit tout (banc local seulement)")
	}
	st, err := settings.Open(filepath.Join(cfg.DataDir, "settings.json"))
	if err != nil {
		return err
	}
	b := board.New(nil, st)
	b.OnActive = integ.SetActive
	srv := web.New(cfg, b, st)
	r := &reloader{path: path, b: b, notice: srv.SetNotice, apply: srv.SetConfig}
	r.cfg.Store(cfg)
	b.New = func(t config.Tile) tile.Source { return integ.New(r.current(), t) }
	srv.OnEdit = func() error { return r.check(true) } // après une modification faite dans l'interface
	srv.Icons = icons.New(filepath.Join(cfg.DataDir, "icons"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.Icons.Start(ctx)
	b.Start(ctx)
	b.Apply(specs(cfg))
	r.stamp = r.fingerprint(cfg)
	go func() { // config.yaml et secrets relus à chaud
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.check(false)
			}
		}
	}()

	hs := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// Pas de WriteTimeout : le flux SSE reste ouvert.
	}
	// Profilage à la demande (NEHOME_PPROF=127.0.0.1:6060), jamais exposé par défaut.
	if addr := os.Getenv("NEHOME_PPROF"); addr != "" {
		go func() { log.Print(http.ListenAndServe(addr, pprofMux())) }()
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	log.Printf("nehome %s : écoute sur %s, %d tuiles", version, cfg.Listen, len(cfg.Tiles))
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func pprofMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	return mux
}

func healthcheck(path string) error {
	listen := ":8080"
	if cfg, err := config.Load(path); err == nil {
		listen = cfg.Listen
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return err
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz : HTTP %d", resp.StatusCode)
	}
	return nil
}
