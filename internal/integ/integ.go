// Package integ : une intégration par fichier, enregistrée dans registry.
package integ

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

//go:generate go run ../../cmd/genspec

// Spec : ce que lit un type d'intégration (formulaire de l'interface).
type Spec struct {
	Secrets, Options []string
	Desc             string
}

// Types : types d'intégration connus, triés.
func Types() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// roleHints : rôles que le générateur ne peut pas lire dans le code (choisis
// à l'exécution) — Pi-hole selon sa version, types génériques selon l'API.
var roleHints = map[string][]string{
	"pihole":     {"pass", "token"},
	"customapi":  {"token", "key", "user", "pass", "post_key"},
	"prometheus": {"token", "key", "user", "pass"},
}

// SpecOf : rôles de secrets et options lus par un type.
func SpecOf(typ string) Spec {
	s := specs[typ]
	for _, r := range roleHints[typ] {
		if !slices.Contains(s.Secrets, r) {
			s.Secrets = append(s.Secrets, r)
		}
	}
	return s
}

// Known : le type existe.
func Known(typ string) bool { _, ok := registry[typ]; return ok }

type factory func(t config.Tile, secret func(role string) (string, error)) (tile.Source, error)

// fields est connu sans démarrer la source : une tuile en panne (secret
// absent) reste réglable sur /settings.
var registry = map[string]struct {
	new    factory
	fields func() []tile.Field
}{
	"truenas":        {newTrueNAS, (&trueNAS{}).Fields},
	"qbittorrent":    {newQbit, (&qbit{}).Fields},
	"mikrotik":       {newMikroTik, (&mikrotik{}).Fields},
	"docker":         {newDocker, (&docker{}).Fields},
	"link":           {newLink, (&link{}).Fields},
	"weather":        {newWeather, (&weather{}).Fields},
	"adguard":        {newAdGuard, (&adguard{}).Fields},
	"crowdsec":       {newCrowdSec, (&crowdsec{}).Fields},
	"traefik":        {newTraefik, (&traefik{}).Fields},
	"gluetun":        {newGluetun, (&gluetun{}).Fields},
	"uptimekuma":     {newUptimeKuma, (&uptimeKuma{}).Fields},
	"cloudflare":     {newCloudflare, (&cloudflare{}).Fields},
	"sonarr":         {newSonarr, (&arr{}).Fields},
	"radarr":         {newRadarr, (&arr{radarr: true}).Fields},
	"jackett":        {newJackett, (&jackett{}).Fields},
	"crossseed":      {newCrossSeed, (&crossSeed{}).Fields},
	"nextcloud":      {newNextcloud, (&nextcloud{}).Fields},
	"ntfy":           {newNtfy, (&ntfy{}).Fields},
	"jdownloader":    {newJDownloader, (&jdown{}).Fields},
	"plex":           {newPlex, (&plex{}).Fields},
	"tautulli":       {newTautulli, (&tautulli{}).Fields},
	"tracearr":       {newTracearr, (&tracearr{}).Fields},
	"seerr":          {newSeerr, (&seerr{}).Fields},
	"maintainerr":    {newMaintainerr, (&maintainerr{}).Fields},
	"wizarr":         {newWizarr, (&wizarr{}).Fields},
	"overseerr":      {newSeerr, (&seerr{}).Fields}, // même API que seerr
	"jellyseerr":     {newSeerr, (&seerr{}).Fields},
	"lidarr":         {newLidarr, (&arr{lidarr: true}).Fields},
	"readarr":        {newReadarr, (&arr{readarr: true}).Fields},
	"prowlarr":       {newProwlarr, (&prowlarr{}).Fields},
	"jellyfin":       {newJellyfin, (&jellyfin{}).Fields},
	"emby":           {newEmby, (&jellyfin{emby: true}).Fields},
	"bazarr":         {newBazarr, (&bazarr{}).Fields},
	"audiobookshelf": {newAudiobookshelf, (&audiobookshelf{}).Fields},
	"navidrome":      {newNavidrome, (&navidrome{}).Fields},
	"immich":         {newImmich, (&immich{}).Fields},
	"transmission":   {newTransmission, (&transmission{}).Fields},
	"deluge":         {newDeluge, (&deluge{}).Fields},
	"sabnzbd":        {newSABnzbd, (&sabnzbd{}).Fields},
	"nzbget":         {newNZBGet, (&nzbget{}).Fields},
	"gitea":          {newGitea, (&gitea{}).Fields},
	"forgejo":        {newForgejo, (&gitea{}).Fields},
	"grafana":        {newGrafana, (&grafana{}).Fields},
	"paperless":      {newPaperless, (&paperless{}).Fields},
	"gotify":         {newGotify, (&gotify{}).Fields},
	"healthchecks":   {newHealthchecks, (&healthchecks{}).Fields},
	"speedtest":      {newSpeedtest, (&speedtest{}).Fields},
	"scrutiny":       {newScrutiny, (&scrutiny{}).Fields},
	"frigate":        {newFrigate, (&frigate{}).Fields},
	"authentik":      {newAuthentik, (&authentik{}).Fields},
	"npm":            {newNPM, (&npm{}).Fields},
	"wgeasy":         {newWGEasy, (&wgeasy{}).Fields},
	"minecraft":      {newMinecraft, (&minecraft{}).Fields},
	"proxmox":        {newProxmox, (&proxmox{}).Fields},
	"portainer":      {newPortainer, (&portainer{}).Fields},
	"glances":        {newGlances, (&glances{}).Fields},
	"netdata":        {newNetdata, (&netdata{}).Fields},
	"pihole":         {newPiHole, (&pihole{}).Fields},
	"homeassistant":  {newHomeAssistant, (&homeassistant{}).Fields},
	"synology":       {newSynology, (&synology{}).Fields},
	"openmediavault": {newOMV, (&omv{}).Fields},
	"opnsense":       {newOPNsense, (&opnsense{}).Fields},
	"unifi":          {newUniFi, (&unifi{}).Fields},
	"system":         {newSystem, (&system{}).Fields},
	"customapi":      {newCustomAPI, (&customAPI{}).Fields},
	"prometheus":     {newPrometheus, (&prometheus{}).Fields},
	"cleanuparr":     {newCleanuparr, (&cleanuparr{}).Fields},
	"tcp":            {newTCP, (&tcpCheck{}).Fields},
}

// New construit la source d'une tuile. Si elle ne peut pas démarrer (type
// inconnu, secret absent), la tuile s'affiche quand même avec l'erreur.
func New(cfg *config.Config, t config.Tile) tile.Source {
	f, ok := registry[t.Type]
	if !ok {
		return broken{err: fmt.Errorf("type d'intégration inconnu : %q", t.Type)}
	}
	src, err := f.new(t, func(role string) (string, error) {
		name, ok := t.Secrets[role]
		if !ok {
			return "", fmt.Errorf("secrets.%s manquant dans config.yaml", role)
		}
		return cfg.Secret(name)
	})
	if err != nil {
		return broken{err: err, fields: f.fields()}
	}
	return src
}

type broken struct {
	err    error
	fields []tile.Field
}

func (b broken) Fields() []tile.Field { return b.fields }
func (b broken) Run(ctx context.Context, emit func(tile.Values, error)) {
	emit(nil, b.err)
	<-ctx.Done()
}

func httpClient(insecure, jar bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // certificat auto-signé déclaré dans config.yaml
	}
	c := &http.Client{Transport: tr, Timeout: 15 * time.Second, CheckRedirect: sameHost}
	if jar {
		c.Jar, _ = cookiejar.New(nil)
	}
	return c
}

// sameHost refuse une redirection vers un autre hôte : Go n'y retire que
// Authorization et Cookie, une clé en en-tête maison (X-Api-Key…) y partirait.
func sameHost(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("trop de redirections")
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("redirection vers un autre hôte refusée (%s)", req.URL.Host)
	}
	return nil
}

// split donne à chaque boucle d'une source son propre emplacement d'erreur :
// sans lui, la réussite d'une boucle effacerait la panne d'une autre.
func split(emit func(tile.Values, error), n int) []func(tile.Values, error) {
	var mu sync.Mutex
	errs := make([]error, n)
	out := make([]func(tile.Values, error), n)
	for i := range out {
		out[i] = func(v tile.Values, err error) {
			mu.Lock()
			errs[i] = err
			joined := errors.Join(errs...)
			mu.Unlock()
			emit(v, joined)
		}
	}
	return out
}

// statusError garde le code HTTP pour que l'appelant sache relancer un login.
type statusError struct {
	Code int
	URL  string
}

func (e statusError) Error() string { return fmt.Sprintf("%s : HTTP %d", e.URL, e.Code) }

// getJSON décode une réponse JSON ; le corps est borné (32 Mo) contre une
// réponse démesurée.
func getJSON(ctx context.Context, c *http.Client, url string, out any, set func(*http.Request)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if set != nil {
		set(req)
	}
	resp, err := c.Do(req)
	if err != nil {
		return cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return statusError{resp.StatusCode, redact(url)}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// redact retire la chaîne de requête (peut porter une clé) des messages affichés.
func redact(u string) string {
	u, _, _ = strings.Cut(u, "?")
	return u
}

func cleanErr(err error) error {
	msg := err.Error()
	if i := strings.Index(msg, "?"); i >= 0 {
		if j := strings.IndexAny(msg[i:], "\": "); j > 0 {
			msg = msg[:i] + msg[i+j:]
		}
	}
	return fmt.Errorf("%s", msg)
}

// Sans navigateur connecté, personne ne regarde : les sondages plus rapides
// qu'idlePeriod ralentissent (moins de CPU ici, moins de charge sur les
// services). Une connexion les réveille aussitôt.
const idlePeriod = time.Minute

var (
	actMu  sync.Mutex
	active bool
	wake   = make(chan struct{})
)

// SetActive est appelé par le tableau quand le premier navigateur se connecte
// ou que le dernier se déconnecte.
func SetActive(a bool) {
	actMu.Lock()
	defer actMu.Unlock()
	if a && !active {
		close(wake)
		wake = make(chan struct{})
	}
	active = a
}

// every appelle f tout de suite puis à chaque période, jusqu'à l'annulation.
func every(ctx context.Context, d time.Duration, f func()) {
	for {
		f()
		actMu.Lock()
		wait, w := d, wake
		if active || d >= idlePeriod {
			w = nil // pas de réveil à attendre
		} else {
			wait = idlePeriod
		}
		actMu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-w:
			t.Stop()
		}
	}
}
