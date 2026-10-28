# Transmission

> Version française : [../fr/integrations/transmission.md](../fr/integrations/transmission.md)


Monitors the Transmission BitTorrent client (daemon or app): rate charts, active and seeding torrents, free space. Read-only.

## Options and secrets
- `url`: Transmission address (`http://host:9091`). The `/transmission/rpc` path is added if missing.
- `insecure_tls`: accept a self-signed certificate.
- Secrets `user` and `pass`: optional, to be set if `rpc-authentication-required` is on. Transmission has no read-only account: the RPC account has full rights, but nehome only sends read requests.

```yaml
- id: transmission
  type: transmission
  title: Transmission
  url: http://transmission:9091
  secrets: {user: transmission_user, pass: transmission_pass}
```

## Fields
On the tile: « Réception » (download rate, chart), « Envoi » (upload rate, chart), « En téléchargement » (downloading), « En partage » (seeding).
In the detail view: « Ratio global » (global ratio), « Limite alternative » (alternative limit), « Limites » (limits), « Espace libre » (free space), « Total depuis l'origine » (all-time total), « Version », « En erreur » (errored), « En vérification » (verifying), the « Téléchargements en cours » (active downloads) table, the « Envois les plus actifs » (most active uploads) table.

## Limits
- The global ratio is computed from cumulative counters (sent / received).
- The full torrent list is re-read every 3 s; with thousands of torrents, this is the heaviest request.
- Version, limits and free space are refreshed every minute. Free space requires Transmission 4.0 or later (`free-space` method).
- Tested only against a fake server matching the RPC specification, not against a real Transmission.
