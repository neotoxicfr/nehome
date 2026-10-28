> Version française : [fr/configuration.md](fr/configuration.md)

# Configuration

nehome is configured in two places:

- `config.yaml`: the infrastructure (addresses, expected secrets, authentication). Read at startup, mounted read-only.
- `data/settings.json`: display choices, made on the `/settings` page. Written by nehome.

File path: `-config` option or `NEHOME_CONFIG` variable (default `/config/config.yaml`). Commands: `serve` (default), `healthcheck`, `version`.

## config.yaml

```yaml
listen: ":8080"            # default
data_dir: /data            # default; holds settings.json
secrets_dir: /run/secrets  # default; Docker secrets

auth:
  header: Remote-User               # default
  trusted_proxies: [172.20.0.2/32]  # required unless auth.disabled
  disabled: false                   # local testing only

tiles:
  - id: truenas
    type: truenas
    title: TrueNAS
    section: Système
    href: https://truenas.example.org/
    url: https://192.0.2.10:444
    insecure_tls: true
    secrets: {key: truenas_key}
    options: {}
```

Unknown keys are rejected at startup (a typo gives a clear error).

### `auth` section

| Key | Role |
|---|---|
| `header` | HTTP header carrying the user authenticated by the proxy (default `Remote-User`) |
| `trusted_proxies` | list of networks (CIDR) whose header is trusted. A request from anywhere else, or without the header, gets 401 |
| `disabled` | turns off all authentication (everyone is "banc"). A warning is logged. Reserve for a local trial |

Without `disabled`, an empty `trusted_proxies` list makes startup fail. The `/healthz` and `/favicon.ico` routes are open; everything else requires a user. Form submissions (POST) must also come from the same site (`Sec-Fetch-Site` or `Origin`).

### Tiles

| Key | Role |
|---|---|
| `id` | unique identifier, `a-z`, `0-9`, `-`; used as the address (`/tile/<id>`) and as the key in `settings.json` |
| `type` | integration type, see [integrations/](integrations/); an unknown type shows an error tile |
| `title` | displayed title (default: the `id`) |
| `section` | grouping on the page (default: `Divers`, i.e. "Misc"); order is that of first appearance |
| `href` | link opened by clicking the title (the application's public address) |
| `url` | address nehome queries (often internal: `http://service:port`) |
| `insecure_tls` | accepts a self-signed certificate for `url`. Off by default |
| `secrets` | role -> file name in `secrets_dir` |
| `options` | type-specific settings (strings) |
| `fields` | fields read by hand from a JSON response (see below) |

A tile whose secret is missing or whose configuration is invalid is still displayed, with the error message, and stays configurable on `/settings`.

### Secrets

Each role (`key`, `token`, `user`, `pass`, depending on the type) points to **a file** in `secrets_dir`, whose content (trailing whitespace removed) is the secret. With Docker Compose, these are secrets:

```yaml
services:
  nehome:
    secrets: [truenas_key]
secrets:
  truenas_key: {file: ./secrets/truenas_key}   # readable by user 65532, mode 400
```

The name must be a plain file name (no `/`, no leading dot): a configuration cannot read outside `secrets_dir`. Secrets appear neither in displayed errors nor in logs. Never put a secret in `config.yaml`.

### Hand-declared fields (`fields`)

For types that read an arbitrary JSON response, each `fields` entry describes one value:

| Key | Role |
|---|---|
| `id` | field identifier |
| `label` | displayed label |
| `path` | dotted path: `data.items.0.count`; `data.items.#` gives an array's length |
| `format` | `text` (default), `number`, `bytes`, `rate`, `percent`, `duration`, `date` |
| `placement` | `tile` (default) or `detail` |
| `warn`, `crit` | optional thresholds: beyond them, the value turns to warning or critical |

```yaml
fields:
  - {id: users, label: Utilisateurs, path: data.users.#, placement: tile}
  - {id: disk, label: Disque, path: data.disk.used_pct, format: percent, warn: 80, crit: 95}
```

This section is used only by the types that read it; others ignore it.

## The /settings page and data/settings.json

`/settings` lists each tile with: visible or not, and, for each field, its placement (on the tile, in the detail view, hidden) and its rank. Saving is immediate and pushed live to every open browser.

Choices are written to `data_dir/settings.json` (atomic write):

```json
{
  "truenas": {
    "hidden": true,
    "fields": {"net_tx": "tile", "load": "hidden"},
    "order": ["cpu", "ram", "pools"]
  }
}
```

Only differences from the integration's default placements are kept. Deleting the file resets everything to defaults. The default placement of each field is listed on each [integration](integrations/) page.

## Docker deployment behind an authentication proxy

nehome does no authentication itself: it trusts a header (`Remote-User`) set by an authenticating proxy (Authelia, Authentik, oauth2-proxy...) that is the only one able to reach port 8080.

```yaml
services:
  nehome:
    image: ghcr.io/neotoxicfr/nehome:latest   # version tag without "v", for example 1.0.0
    container_name: nehome
    restart: unless-stopped
    read_only: true
    security_opt: ["no-new-privileges:true"]
    cap_drop: [ALL]
    environment:
      - TZ=Europe/Paris
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./data:/data                # settings.json; owner 65532
    secrets: [truenas_key, qbit_user, qbit_pass]
    networks: [proxy]
    labels:
      traefik.enable: "true"
      traefik.http.routers.nehome.rule: Host(`home.example.org`)
      traefik.http.routers.nehome.entrypoints: https
      # clears identity headers sent by the client, then the authentication chain sets its own
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

Points to respect:

- The image is distroless-based, running as the `nonroot` user (UID 65532): the `data` folder and the secret files must belong to it (`chown 65532:65532`, mode 400 for secrets).
- `trusted_proxies` = address (or network) **of the proxy as seen by nehome**, for example `172.20.0.2/32` if Traefik has that address on the `proxy` network. Do not use a wide network: any container on it could then spoof the header.
- Port 8080 must not be published on the host (`ports:`).
- The proxy must erase any `Remote-User` value sent by the client before running authentication, as in the example (defense in depth matters: only the connection coming from the proxy is trusted).
- The real-time stream (`/events`, SSE) must not be buffered: nehome sends `X-Accel-Buffering: no` and a heartbeat every 25 s; make sure no idle timeout that is too short cuts the connection.
- To read Docker's state (types `docker`, `link`), go through a read-only socket proxy on a dedicated network, never through a directly mounted `docker.sock`.
- Health check: the image includes one (`nehome healthcheck`, which queries `/healthz` on the listen port).
- Update: pull the new image, `docker compose up -d`. Settings in `data/` are kept.

## Changing things without a restart

- **config.yaml is reloaded live** (checked every 2 s). A tile that is added, modified or
  removed is started, restarted or stopped on its own; the others keep their values, and open
  browsers reload. Only `listen` and `data_dir` require a restart.
  An invalid file is not applied: the previous configuration stays active and a banner
  shows the error.
- **A modified secret** (file replaced in `secrets_dir` or `data_dir/secrets`) restarts the
  tiles that use it.
- **From the interface** (Réglages, i.e. Settings): « Ajouter un service » (add a service) opens a
  searchable gallery with the usual address and port prefilled. Each card shows the connection
  (type, address, options, keys) with « Enregistrer la liaison » (save) and « Supprimer la liaison »
  (delete, with a confirmation box); « Affichage et actions » opens what the tile shows and its
  buttons. The file must be writable by nehome: put it in the data volume
  (`NEHOME_CONFIG=/data/config.yaml`). The file's comments are preserved.
- **Keys entered in the interface**: written to `data_dir/secrets/<id>_<role>` (mode 0600),
  never displayed again. A Docker secret of the same name keeps priority. The form also offers
  to link an existing secret (Docker or interface).

## Icons

Icons come from the [dashboard-icons](https://github.com/homarr-labs/dashboard-icons) library:
nehome downloads the PNG once, checks that it is one, keeps it in `data_dir/icons` and
serves it itself (the CSP only allows images from its own origin). The icon is guessed from
the type, the `container` option, the id, then the title; in Réglages, each card lets you
force another name or hide the icon on the home page. For an application missing from the
library, drop `data_dir/icons/<name>.png`.

## Look and feel

On **Settings → Apparence**: the name shown in the header and the browser tab, a mode (Sombre,
Ardoise, OLED, Clair), an accent colour (eight swatches or any `#rrggbb`) and an uploaded favicon
(PNG, ICO or WebP, 512 KB at most). Changes preview live and save on their own. Colours are served
as `/theme.css`; everything else in the palette is derived from them, and the text colour on the
accent is chosen for contrast. Open pages pick up the new look without a reload.
