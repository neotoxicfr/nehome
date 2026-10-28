<div align="center">

# nehome

**Un tableau de bord d'accueil qui parle vraiment à vos services.**

Auto-hébergé, écrit en Go. Chaque tuile interroge l'API d'un service côté serveur, en direct dans
le navigateur, sans que vos clés ne quittent jamais le serveur.

[English](README.md) · [Configuration](docs/fr/configuration.md) · [Intégrations](docs/fr/integrations/) · [Architecture](docs/fr/architecture.md)

[![ci](https://github.com/neotoxicfr/nehome/actions/workflows/ci.yml/badge.svg)](https://github.com/neotoxicfr/nehome/actions/workflows/ci.yml)

![Tableau de bord nehome](docs/images/dashboard.png)

</div>

## Pourquoi nehome

Homepage, Homer, Heimdall et Homarr affichent des liens et quelques compteurs. nehome va plus loin
sur les données : avancement des files, débits en direct, remplissage des pools, conteneurs en
panne, requêtes DNS bloquées, derniers imports, températures SMART… et vous laisse décider, champ
par champ, de ce qui mérite une place sur la tuile.

- **En direct.** Les valeurs arrivent par Server-Sent Events ; les débits se mettent à jour toutes
  les 2 à 5 s.
- **Votre disposition.** Dans **Réglages**, chaque champ de chaque tuile est *toujours visible*,
  *dans le détail* (ouvert au clic sur la tuile) ou *masqué*, dans l'ordre voulu. Les tuiles ont une
  position, et les sections se rangent en grille alignée ou en colonnes équilibrées.
- **Actions rapides.** Des boutons sur la tuile ou dans le détail : rafraîchir les flux, suspendre
  le blocage DNS, redémarrer un conteneur ou le VPN, débannir une IP, tester les indexeurs, lancer
  une tâche planifiée, réveiller un PC… Réservées aux admins, journalisées, et les plus sensibles
  demandent confirmation.
- **Votre identité.** Nom, thème (sombre, ardoise, OLED ou clair), couleur d'accent et favicon, avec
  aperçu en direct.
- **Configuré sans redémarrage.** Ajoutez, modifiez ou supprimez une liaison depuis l'interface, ou
  éditez `config.yaml` : il est relu à chaud, et seules les tuiles modifiées redémarrent.
- **Des secrets qui restent secrets.** Les clés viennent des secrets Docker ou sont saisies une fois
  dans l'interface (stockées en 0600, jamais réaffichées). Le navigateur ne reçoit que du HTML rendu.
- **Léger.** Un binaire statique, environ 35 Mo de RAM et moins de 1 % d'un cœur avec plus de 45 tuiles.
  Sans navigateur ouvert, rien n'est rendu et les sondages rapides passent à une fois par minute.
- **Verrouillé.** Authentification déléguée à votre proxy (Authelia, Authentik, oauth2-proxy…), CSP
  stricte, intégrations en lecture seule hors actions cliquées, image distroless sans root.

<p align="center"><img src="docs/images/detail.png" alt="Fenêtre de détail d'une tuile TrueNAS" width="820"></p>

## Intégrations

68 types. Une page par type dans [docs/fr/integrations/](docs/fr/integrations/),
avec les options, les rôles de secrets et les droits minimaux à accorder.

| Domaine | Types |
|---|---|
| Système et OS | `system` (tout hôte Linux, sans agent), `docker`, `truenas`, `proxmox`, `synology`, `openmediavault`, `glances`, `netdata`, `portainer`, `scrutiny` |
| Réseau et sécurité | `traefik`, `npm`, `crowdsec`, `authentik`, `adguard`, `pihole`, `opnsense`, `unifi`, `mikrotik`, `gluetun`, `wgeasy`, `cloudflare`, `tcp` |
| Supervision | `uptimekuma`, `healthchecks`, `grafana`, `prometheus`, `speedtest`, `ntfy`, `gotify` |
| Médias | `plex`, `jellyfin`, `emby`, `tautulli`, `tracearr`, `seerr` / `overseerr` / `jellyseerr`, `maintainerr`, `wizarr`, `audiobookshelf`, `navidrome`, `immich` |
| Téléchargement | `sonarr`, `radarr`, `lidarr`, `readarr`, `prowlarr`, `bazarr`, `jackett`, `crossseed`, `cleanuparr`, `qbittorrent`, `transmission`, `deluge`, `sabnzbd`, `nzbget`, `jdownloader` |
| Applications | `nextcloud`, `paperless`, `gitea` / `forgejo`, `frigate`, `homeassistant`, `minecraft`, `weather` |
| Génériques | `link` (lien + état du conteneur + sonde HTTP), `customapi` (n'importe quelle API JSON, champs déclarés en YAML, bouton POST facultatif), `prometheus` |

Les icônes viennent de la bibliothèque [dashboard-icons](https://github.com/homarr-labs/dashboard-icons)
et sont devinées automatiquement ; chaque tuile peut en choisir une autre ou la masquer.

> **Éprouvées contre de vrais services :** truenas, qbittorrent, mikrotik, docker, link, weather,
> adguard, crowdsec, traefik, gluetun, uptimekuma, sonarr, radarr, jackett, crossseed, cleanuparr,
> nextcloud, ntfy, jdownloader, plex, tautulli, tracearr, seerr, maintainerr, wizarr, cloudflare,
> customapi, system, tcp. Les autres sont
> testées contre des réponses reconstituées d'après la documentation ou le code de chaque service et
> peuvent demander des ajustements : les retours d'installations réelles sont les bienvenus.

## Démarrage rapide

nehome s'attend à être derrière un reverse proxy qui authentifie les utilisateurs et transmet leur
nom dans un en-tête.

```yaml
# compose.yml
services:
  nehome:
    image: ghcr.io/neotoxicfr/nehome:latest
    restart: unless-stopped
    read_only: true
    security_opt: ["no-new-privileges:true"]
    cap_drop: [ALL]
    environment:
      - NEHOME_CONFIG=/data/config.yaml   # modifiable depuis l'interface
    volumes:
      - ./data:/data                      # chown 65532:65532
    secrets: [sonarr_key]
secrets:
  sonarr_key: {file: ./secrets/sonarr_key}
```

```yaml
# data/config.yaml
auth:
  header: Remote-User
  trusted_proxies: [192.0.2.10/32]   # votre reverse proxy
  admins: [alice]                    # qui peut modifier les liaisons depuis l'interface
tiles:
  - {id: sonarr, type: sonarr, title: Sonarr, section: Médias,
     href: "https://sonarr.example.org", url: "http://sonarr:8989", secrets: {key: sonarr_key}}
```

Ouvrez ensuite **Réglages** pour ajouter d'autres liaisons (le formulaire indique les options et les
clés dont chaque service a besoin) et choisir ce que chaque tuile affiche. Exemple complet :
[compose.example.yml](compose.example.yml), [config.example.yaml](config.example.yaml). Toutes les
options : [docs/fr/configuration.md](docs/fr/configuration.md).

Pour un essai local sans proxy, mettez `auth.disabled: true` et lancez
`go run ./cmd/nehome -config config.yaml`.

## À savoir

- L'interface est en **français** pour l'instant ; la documentation existe en anglais et en français.
- Chaque action s'affiche sur la tuile, dans le détail ou nulle part, comme les champs.
- Les images sont étiquetées `X.Y.Z` (sans « v ») et `latest` ; fixez une version en production.

## Contribuer

Rapports de bogue, demandes d'intégration (avec des exemples de réponses de l'API) et pull requests
sont les bienvenus. Voir [CONTRIBUTING.md](CONTRIBUTING.md) ; les failles de sécurité passent par
[SECURITY.md](SECURITY.md).

## Licence

[MIT](LICENSE)
