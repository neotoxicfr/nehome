# qBittorrent

> Version française : [../fr/integrations/qbittorrent.md](../fr/integrations/qbittorrent.md)


Transfer rates, active and seeding torrents, ratio, free space, categories and tags.

## Configuration

```yaml
- id: qbittorrent
  type: qbittorrent
  title: qBittorrent
  section: Téléchargement
  href: https://qbittorrent.example.org/
  url: http://qbittorrent:8080
  secrets: {user: qbit_user, pass: qbit_pass}
```

## Secrets

| Role | Content |
|---|---|
| `user` | web UI username |
| `pass` | password |

Both are optional: without `secrets.user`, nehome does not log in (qBittorrent can exempt a subnet from authentication). If `user` is declared, `pass` must be too. A regular web UI account is enough. nehome logs in again on a 403.

## Fields

| ID | Label | Default |
|---|---|---|
| `down`, `up` | « Réception » (download rate), « Envoi » (upload rate) (charts) | tile |
| `leech`, `seed` | « En téléchargement » (downloading), « En partage » (seeding) | tile |
| `ratio`, `conn`, `alt`, `limits`, `free`, `total`, `peers`, `errors`, `stalled` | global ratio, connection status, alternative limit, limits, free space, all-time total, peers, errored, stalled | detail |
| `dl`, `topup`, `cats`, `tags` | tables: active downloads, most active uploads, by category, by tag | detail |

## Polling and limits

Polled every 2 s on `sync/maindata` in incremental mode: the first call returns all torrents, later ones only the changes. Read-only.
