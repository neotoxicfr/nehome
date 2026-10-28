# Deluge

> Version française : [../fr/integrations/deluge.md](../fr/integrations/deluge.md)

Monitors Deluge through its web interface (JSON-RPC): transfer rates as curves, torrents downloading or seeding, connections, free space. Read-only.

## Options and secrets
- `url`: address of the web interface (`http://host:8112`). The `/json` path is appended.
- `insecure_tls`: self-signed certificate accepted.
- Secret `pass` (required): web interface password (`deluge` by default, change it). There is no username and no reduced permission level.

The web interface must be attached to a daemon (Connection Manager); otherwise the tile reports the error.

```yaml
- id: deluge
  type: deluge
  title: Deluge
  url: http://deluge:8112
  secrets: {pass: deluge_pass}
```

## Fields
On the tile: « Réception » (download rate, curve), « Envoi » (upload rate, curve), « En téléchargement » (downloading), « En partage » (seeding).
In the detail view: incoming connections, connections, DHT nodes, limits, free space, errored, paused, a table of current downloads, a table of most active uploads (with the torrent's ratio).

## Limits
- No global ratio or all-time total: the web interface does not provide them in `web.update_ui`.
- Labels (Label plugin) are not read.
- An expired session is reopened automatically (a single attempt per 3 s cycle).
- Tested only against a fake server based on Deluge's code, not against a real Deluge.
