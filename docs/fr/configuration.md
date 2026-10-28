# Configuration

nehome se règle à deux endroits :

- `config.yaml` : l'infrastructure (adresses, secrets attendus, authentification). Lu au démarrage, monté en lecture seule.
- `data/settings.json` : les choix d'affichage, faits dans la page `/settings`. Écrit par nehome.

Chemin du fichier : option `-config` ou variable `NEHOME_CONFIG` (défaut `/config/config.yaml`). Commandes : `serve` (défaut), `healthcheck`, `version`.

## config.yaml

```yaml
listen: ":8080"            # défaut
data_dir: /data            # défaut ; contient settings.json
secrets_dir: /run/secrets  # défaut ; secrets Docker

auth:
  header: Remote-User               # défaut
  trusted_proxies: [172.20.0.2/32]  # obligatoire sauf auth.disabled
  disabled: false                   # banc local seulement

tiles:
  - id: truenas
    type: truenas
    title: TrueNAS
    section: Système
    href: https://truenas.exemple.org/
    url: https://192.0.2.10:444
    insecure_tls: true
    secrets: {key: truenas_key}
    options: {}
```

Les clés inconnues sont refusées au démarrage (faute de frappe = erreur claire).

### Section `auth`

| Clé | Rôle |
|---|---|
| `header` | en-tête HTTP qui porte l'utilisateur authentifié par le proxy (défaut `Remote-User`) |
| `trusted_proxies` | liste de réseaux (CIDR) dont l'en-tête est cru. Une requête qui vient d'ailleurs, ou sans en-tête, reçoit 401 |
| `disabled` | désactive toute authentification (tout le monde est « banc »). Un avertissement est journalisé. À réserver à un essai local |

Sans `disabled`, une liste `trusted_proxies` vide fait échouer le démarrage. Les routes `/healthz` et `/favicon.ico` sont ouvertes ; tout le reste exige un utilisateur. Les envois de formulaire (POST) doivent en plus venir du même site (`Sec-Fetch-Site` ou `Origin`).

### Tuiles

| Clé | Rôle |
|---|---|
| `id` | identifiant unique, `a-z`, `0-9`, `-` ; sert d'adresse (`/tile/<id>`) et de clé dans `settings.json` |
| `type` | type d'intégration, voir [integrations/](integrations/) ; un type inconnu affiche une tuile en erreur |
| `title` | titre affiché (défaut : l'`id`) |
| `section` | regroupement sur la page (défaut : `Divers`) ; l'ordre est celui de première apparition |
| `href` | lien ouvert en cliquant sur le titre (adresse publique de l'application) |
| `url` | adresse que nehome interroge (souvent interne : `http://service:port`) |
| `insecure_tls` | accepte un certificat auto-signé pour `url`. Désactivé par défaut |
| `secrets` | rôle → nom de fichier dans `secrets_dir` |
| `options` | réglages propres au type (textes) |
| `fields` | champs lus à la main dans une réponse JSON (voir plus bas) |

Une tuile dont le secret manque ou dont la configuration est invalide s'affiche quand même, avec le message d'erreur, et reste réglable sur `/settings`.

### Secrets

Chaque rôle (`key`, `token`, `user`, `pass`, selon le type) désigne **un fichier** de `secrets_dir`, dont le contenu (espaces de fin retirés) est le secret. Avec Docker Compose, ce sont des secrets :

```yaml
services:
  nehome:
    secrets: [truenas_key]
secrets:
  truenas_key: {file: ./secrets/truenas_key}   # lisible par l'utilisateur 65532, mode 400
```

Le nom doit être un simple nom de fichier (pas de `/`, pas de point initial) : une configuration ne peut pas lire en dehors de `secrets_dir`. Les secrets n'apparaissent ni dans les erreurs affichées ni dans les journaux. Ne mettez jamais un secret dans `config.yaml`.

### Champs déclarés à la main (`fields`)

Pour les types qui lisent une réponse JSON quelconque, chaque entrée de `fields` décrit une valeur :

| Clé | Rôle |
|---|---|
| `id` | identifiant du champ |
| `label` | libellé affiché |
| `path` | chemin pointé : `data.items.0.count` ; `data.items.#` donne la longueur d'un tableau |
| `format` | `text` (défaut), `number`, `bytes`, `rate`, `percent`, `duration`, `date` |
| `placement` | `tile` (défaut) ou `detail` |
| `warn`, `crit` | seuils facultatifs : au-delà, la valeur passe en avertissement ou en critique |

```yaml
fields:
  - {id: users, label: Utilisateurs, path: data.users.#, placement: tile}
  - {id: disk, label: Disque, path: data.disk.used_pct, format: percent, warn: 80, crit: 95}
```

Cette section n'est utilisée que par les types qui la lisent ; les autres l'ignorent.

## Page /settings et data/settings.json

`/settings` liste chaque tuile avec : visible ou non, et, pour chaque champ, son emplacement (sur la tuile, dans le détail, masqué) et son rang. L'enregistrement est immédiat et mis à jour en direct pour tous les navigateurs ouverts.

Les choix sont écrits dans `data_dir/settings.json` (écriture atomique) :

```json
{
  "truenas": {
    "hidden": true,
    "fields": {"net_tx": "tile", "load": "hidden"},
    "order": ["cpu", "ram", "pools"]
  }
}
```

Seuls les écarts avec les emplacements par défaut de l'intégration sont gardés. Supprimer le fichier remet tout par défaut. Les emplacements par défaut de chaque champ sont listés dans la page de chaque [intégration](integrations/).

## Déploiement Docker derrière un proxy d'authentification

nehome ne fait pas d'authentification lui-même : il fait confiance à un en-tête (`Remote-User`) posé par un proxy qui authentifie (Authelia, Authentik, oauth2-proxy...) et qui est le seul à pouvoir joindre le port 8080.

```yaml
services:
  nehome:
    image: ghcr.io/neotoxicfr/nehome:latest   # tag de version sans « v », par exemple 1.0.0
    container_name: nehome
    restart: unless-stopped
    read_only: true
    security_opt: ["no-new-privileges:true"]
    cap_drop: [ALL]
    environment:
      - TZ=Europe/Paris
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./data:/data                # settings.json ; propriétaire 65532
    secrets: [truenas_key, qbit_user, qbit_pass]
    networks: [proxy]
    labels:
      traefik.enable: "true"
      traefik.http.routers.nehome.rule: Host(`home.exemple.org`)
      traefik.http.routers.nehome.entrypoints: https
      # vide les en-têtes d'identité envoyés par le client, puis la chaîne d'authentification pose les siens
      traefik.http.middlewares.nehome-noid.headers.customrequestheaders.Remote-User: ""
      traefik.http.routers.nehome.middlewares: nehome-noid, authelia@file
      traefik.http.services.nehome.loadbalancer.server.port: "8080"

networks:
  proxy: {external: true}

secrets:
  truenas_key: {file: ./secrets/truenas_key}
  qbit_user: {file: ./secrets/qbit_user}
  qbit_pass: {file: ./secrets/qbit_pass}
```

Points à respecter :

- L'image est basée sur distroless, utilisateur `nonroot` (UID 65532) : le dossier `data` et les fichiers de secrets doivent lui appartenir (`chown 65532:65532`, mode 400 pour les secrets).
- `trusted_proxies` = adresse (ou réseau) **du proxy vu par nehome**, par exemple `172.20.0.2/32` si Traefik a cette adresse sur le réseau `proxy`. Ne mettez pas un réseau large : tout conteneur de ce réseau pourrait alors usurper l'en-tête.
- Le port 8080 ne doit pas être publié sur l'hôte (`ports:`).
- Le proxy doit effacer toute valeur de `Remote-User` envoyée par le client avant de lancer l'authentification, comme dans l'exemple (la défense en profondeur compte : seule la connexion venant du proxy est crue).
- Le flux temps réel (`/events`, SSE) ne doit pas être mis en tampon : nehome envoie `X-Accel-Buffering: no` et un battement toutes les 25 s ; vérifiez qu'aucun délai d'inactivité trop court ne coupe la connexion.
- Pour lire l'état de Docker (types `docker`, `link`), passez par un proxy de socket en lecture seule sur un réseau dédié, jamais par `docker.sock` monté directement.
- Contrôle de santé : l'image en contient un (`nehome healthcheck`, qui interroge `/healthz` sur le port d'écoute).
- Mise à jour : tirez la nouvelle image, `docker compose up -d`. Les réglages de `data/` sont conservés.

## Modifier sans redémarrer

- **config.yaml est relu à chaud** (vérification toutes les 2 s). Une tuile ajoutée, modifiée ou
  supprimée est démarrée, redémarrée ou arrêtée seule ; les autres gardent leurs valeurs, et les
  navigateurs ouverts se rechargent. Seuls `listen` et `data_dir` demandent un redémarrage.
  Un fichier invalide n'est pas appliqué : la configuration précédente reste active et un bandeau
  affiche l'erreur.
- **Un secret modifié** (fichier remplacé dans `secrets_dir` ou `data_dir/secrets`) redémarre les
  tuiles qui l'utilisent.
- **Depuis l'interface** (Réglages) : « Ajouter un service » ouvre une galerie avec recherche, adresse
  et port usuels préremplis. Chaque carte montre la connexion (type, adresse, options, clés) avec
  « Enregistrer la liaison » et « Supprimer la liaison » (case de confirmation) ; « Affichage et
  actions » ouvre ce que la tuile montre et ses boutons. Il faut que le fichier soit modifiable par
  nehome : placez-le dans le volume de données (`NEHOME_CONFIG=/data/config.yaml`). Les
  commentaires du fichier sont conservés.
- **Clés saisies dans l'interface** : écrites dans `data_dir/secrets/<id>_<rôle>` (droits 0600),
  jamais réaffichées. Un secret Docker du même nom reste prioritaire. Le formulaire propose aussi
  de relier un secret existant (Docker ou interface).

## Icônes

Les icônes viennent de la bibliothèque [dashboard-icons](https://github.com/homarr-labs/dashboard-icons) :
nehome télécharge le PNG une fois, vérifie que c'en est un, le garde dans `data_dir/icons` et le
sert lui-même (la CSP n'autorise que les images de sa propre origine). L'icône est devinée d'après
le type, l'option `container`, l'id puis le titre ; dans les Réglages, chaque carte permet
d'imposer un autre nom ou de masquer l'icône sur l'accueil. Pour une application absente de la
bibliothèque, déposez `data_dir/icons/<nom>.png`.

## Identité graphique

Dans **Réglages → Apparence** : le nom affiché en haut et dans l'onglet, un mode (Sombre, Ardoise,
OLED, Clair), une couleur d'accent (huit pastilles ou n'importe quel `#rrggbb`) et un favicon
téléversé (PNG, ICO ou WebP, 512 Ko au plus). L'aperçu est immédiat et tout s'enregistre seul. Les
couleurs sont servies par `/theme.css` ; le reste de la palette en dérive, et la couleur du texte
sur l'accent est choisie pour le contraste. Les pages ouvertes changent sans rechargement.
