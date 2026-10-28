package web

import (
	"fmt"
	"slices"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/integ"
)

// typeDefault : de quoi préremplir le formulaire d'ajout (hôte = nom de
// conteneur habituel, port et protocole par défaut du service).
type typeDefault struct {
	Cat, Title, Host string
	Port             int
	HTTPS, Insecure  bool
	Options          map[string]string
}

// Catégories, dans l'ordre de la galerie ; la section proposée par défaut suit.
var categories = []struct{ ID, Label, Section string }{
	{"sys", "Système et OS", "Supervision"},
	{"net", "Réseau et sécurité", "Réseau & Sécurité"},
	{"mon", "Supervision", "Supervision"},
	{"media", "Médias", "Média"},
	{"dl", "Téléchargement", "Téléchargement"},
	{"apps", "Applications", "Cloud & Outils"},
	{"gen", "Génériques", "Divers"},
}

var defaults = map[string]typeDefault{
	"system":         {Cat: "sys", Title: "Serveur", Options: map[string]string{"proc": "/host/proc", "sys": "/host/sys", "disks": "/"}},
	"docker":         {Cat: "sys", Title: "Conteneurs", Host: "socket-proxy", Port: 2375},
	"truenas":        {Cat: "sys", Title: "TrueNAS", Host: "truenas", Port: 443, HTTPS: true, Insecure: true},
	"proxmox":        {Cat: "sys", Title: "Proxmox", Host: "proxmox", Port: 8006, HTTPS: true, Insecure: true},
	"synology":       {Cat: "sys", Title: "Synology", Host: "synology", Port: 5001, HTTPS: true, Insecure: true},
	"openmediavault": {Cat: "sys", Title: "OpenMediaVault", Host: "openmediavault", Port: 80},
	"glances":        {Cat: "sys", Title: "Glances", Host: "glances", Port: 61208},
	"netdata":        {Cat: "sys", Title: "Netdata", Host: "netdata", Port: 19999},
	"portainer":      {Cat: "sys", Title: "Portainer", Host: "portainer", Port: 9443, HTTPS: true, Insecure: true},
	"scrutiny":       {Cat: "sys", Title: "Scrutiny", Host: "scrutiny", Port: 8080},
	"traefik":        {Cat: "net", Title: "Traefik", Host: "traefik", Port: 8080},
	"npm":            {Cat: "net", Title: "Nginx Proxy Manager", Host: "nginx-proxy-manager", Port: 81},
	"crowdsec":       {Cat: "net", Title: "CrowdSec", Host: "crowdsec", Port: 8080},
	"authentik":      {Cat: "net", Title: "Authentik", Host: "authentik-server", Port: 9000},
	"adguard":        {Cat: "net", Title: "AdGuard Home", Host: "adguard", Port: 80},
	"pihole":         {Cat: "net", Title: "Pi-hole", Host: "pihole", Port: 80},
	"opnsense":       {Cat: "net", Title: "OPNsense", Host: "opnsense", Port: 443, HTTPS: true, Insecure: true},
	"unifi":          {Cat: "net", Title: "UniFi", Host: "unifi", Port: 8443, HTTPS: true, Insecure: true},
	"mikrotik":       {Cat: "net", Title: "MikroTik", Host: "router", Port: 80, Options: map[string]string{"wan": "ether1"}},
	"gluetun":        {Cat: "net", Title: "VPN", Host: "gluetun", Port: 8000},
	"wgeasy":         {Cat: "net", Title: "WireGuard", Host: "wg-easy", Port: 51821},
	"cloudflare":     {Cat: "net", Title: "Cloudflare"},
	"tcp":            {Cat: "net", Title: "Ports"},
	"uptimekuma":     {Cat: "mon", Title: "Uptime Kuma", Host: "uptime-kuma", Port: 3001},
	"healthchecks":   {Cat: "mon", Title: "Healthchecks", Host: "healthchecks", Port: 8000},
	"grafana":        {Cat: "mon", Title: "Grafana", Host: "grafana", Port: 3000},
	"prometheus":     {Cat: "mon", Title: "Prometheus", Host: "prometheus", Port: 9090},
	"speedtest":      {Cat: "mon", Title: "Speedtest", Host: "speedtest-tracker", Port: 80},
	"ntfy":           {Cat: "mon", Title: "ntfy", Host: "ntfy", Port: 80},
	"gotify":         {Cat: "mon", Title: "Gotify", Host: "gotify", Port: 80},
	"plex":           {Cat: "media", Title: "Plex", Host: "plex", Port: 32400},
	"jellyfin":       {Cat: "media", Title: "Jellyfin", Host: "jellyfin", Port: 8096},
	"emby":           {Cat: "media", Title: "Emby", Host: "emby", Port: 8096},
	"tautulli":       {Cat: "media", Title: "Tautulli", Host: "tautulli", Port: 8181},
	"tracearr":       {Cat: "media", Title: "Tracearr", Host: "tracearr", Port: 3000},
	"seerr":          {Cat: "media", Title: "Seerr", Host: "seerr", Port: 5055},
	"overseerr":      {Cat: "media", Title: "Overseerr", Host: "overseerr", Port: 5055},
	"jellyseerr":     {Cat: "media", Title: "Jellyseerr", Host: "jellyseerr", Port: 5055},
	"maintainerr":    {Cat: "media", Title: "Maintainerr", Host: "maintainerr", Port: 6246},
	"wizarr":         {Cat: "media", Title: "Wizarr", Host: "wizarr", Port: 5690},
	"audiobookshelf": {Cat: "media", Title: "Audiobookshelf", Host: "audiobookshelf", Port: 80},
	"navidrome":      {Cat: "media", Title: "Navidrome", Host: "navidrome", Port: 4533},
	"immich":         {Cat: "media", Title: "Immich", Host: "immich-server", Port: 2283},
	"sonarr":         {Cat: "dl", Title: "Sonarr", Host: "sonarr", Port: 8989},
	"radarr":         {Cat: "dl", Title: "Radarr", Host: "radarr", Port: 7878},
	"lidarr":         {Cat: "dl", Title: "Lidarr", Host: "lidarr", Port: 8686},
	"readarr":        {Cat: "dl", Title: "Readarr", Host: "readarr", Port: 8787},
	"prowlarr":       {Cat: "dl", Title: "Prowlarr", Host: "prowlarr", Port: 9696},
	"bazarr":         {Cat: "dl", Title: "Bazarr", Host: "bazarr", Port: 6767},
	"jackett":        {Cat: "dl", Title: "Jackett", Host: "jackett", Port: 9117},
	"crossseed":      {Cat: "dl", Title: "cross-seed", Host: "cross-seed", Port: 2468},
	"qbittorrent":    {Cat: "dl", Title: "qBittorrent", Host: "qbittorrent", Port: 8080},
	"transmission":   {Cat: "dl", Title: "Transmission", Host: "transmission", Port: 9091},
	"deluge":         {Cat: "dl", Title: "Deluge", Host: "deluge", Port: 8112},
	"sabnzbd":        {Cat: "dl", Title: "SABnzbd", Host: "sabnzbd", Port: 8080},
	"nzbget":         {Cat: "dl", Title: "NZBGet", Host: "nzbget", Port: 6789},
	"jdownloader":    {Cat: "dl", Title: "JDownloader"},
	"cleanuparr":     {Cat: "dl", Title: "Cleanuparr", Host: "cleanuparr", Port: 11011},
	"nextcloud":      {Cat: "apps", Title: "Nextcloud", Host: "nextcloud", Port: 80, Options: map[string]string{"user": "admin"}},
	"paperless":      {Cat: "apps", Title: "Paperless", Host: "paperless", Port: 8000},
	"gitea":          {Cat: "apps", Title: "Gitea", Host: "gitea", Port: 3000},
	"forgejo":        {Cat: "apps", Title: "Forgejo", Host: "forgejo", Port: 3000},
	"frigate":        {Cat: "apps", Title: "Frigate", Host: "frigate", Port: 5000},
	"homeassistant":  {Cat: "apps", Title: "Home Assistant", Host: "homeassistant", Port: 8123},
	"minecraft":      {Cat: "apps", Title: "Minecraft", Options: map[string]string{"host": "minecraft", "port": "25565"}},
	"weather":        {Cat: "apps", Title: "Météo", Options: map[string]string{"lat": "48.85", "lon": "2.35", "timezone": "Europe/Paris"}},
	"link":           {Cat: "gen", Title: "Lien"},
	"customapi":      {Cat: "gen", Title: "API"},
}

func defaultOf(typ string) typeDefault {
	if d, ok := defaults[typ]; ok {
		return d
	}
	return typeDefault{Cat: "gen", Title: typ}
}

// prefill : tuile de départ pour un nouveau service (identifiant libre, section
// de sa catégorie, adresse habituelle, options par défaut).
func prefill(typ string, cfg *config.Config) config.Tile {
	d := defaultOf(typ)
	t := config.Tile{Type: typ, Title: d.Title, InsecureTLS: d.Insecure, Options: map[string]string{}}
	for k, v := range d.Options {
		t.Options[k] = v
	}
	taken := map[string]bool{}
	for _, c := range cfg.Tiles {
		taken[c.ID] = true
	}
	t.ID = typ
	for i := 2; taken[t.ID]; i++ {
		t.ID = fmt.Sprintf("%s-%d", typ, i)
	}
	for _, c := range categories {
		if c.ID == d.Cat {
			t.Section = c.Section
		}
	}
	if d.Host != "" {
		scheme := "http"
		if d.HTTPS {
			scheme = "https"
		}
		t.URL = fmt.Sprintf("%s://%s:%d", scheme, d.Host, d.Port)
	}
	return t
}

// gallery : services par catégorie, pour le choix visuel du type à ajouter.
type galleryCat struct {
	Label string
	Items []galleryItem
}

type galleryItem struct{ Type, Title, Desc, Icon string }

func (s *Server) gallery() []galleryCat {
	byCat := map[string][]galleryItem{}
	for _, typ := range integ.Types() {
		d := defaultOf(typ)
		it := galleryItem{Type: typ, Title: d.Title, Desc: integ.SpecOf(typ).Desc}
		if s.Icons != nil {
			if n := s.Icons.Guess("", typ, "", typ, d.Title); n != "" {
				it.Icon = "/icon/" + n + ".png"
			}
		}
		byCat[d.Cat] = append(byCat[d.Cat], it)
	}
	var out []galleryCat
	for _, c := range categories {
		items := byCat[c.ID]
		slices.SortFunc(items, func(a, b galleryItem) int { return cmpFold(a.Title, b.Title) })
		if len(items) > 0 {
			out = append(out, galleryCat{c.Label, items})
		}
	}
	return out
}

func cmpFold(a, b string) int {
	la, lb := []rune(a), []rune(b)
	for i := 0; i < len(la) && i < len(lb); i++ {
		x, y := la[i]|0x20, lb[i]|0x20
		if x != y {
			return int(x) - int(y)
		}
	}
	return len(la) - len(lb)
}

// sectionNames : sections existantes, proposées à la saisie.
func sectionNames(cfg *config.Config) []string {
	var out []string
	for _, t := range cfg.Tiles {
		if t.Section != "" && !slices.Contains(out, t.Section) {
			out = append(out, t.Section)
		}
	}
	return out
}
