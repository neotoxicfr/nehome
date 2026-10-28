# NZBGet

> Version française : [../fr/integrations/nzbget.md](../fr/integrations/nzbget.md)

Monitors the NZBGet Usenet client: download rate as a curve, queue, remaining data, history, free space. Read-only (JSON-RPC methods `status`, `listgroups`, `history`).

## Options and secrets
- `url`: NZBGet address (`http://host:6789`). The `/jsonrpc` path is appended.
- `insecure_tls`: accept a self-signed certificate.
- Secrets `user` and `pass`: optional, control credentials (`ControlUsername` / `ControlPassword`, `nzbget` / `tegbzn6789` by default). The "Restricted" account is enough, otherwise "Control": nehome performs no actions.

```yaml
- id: nzbget
  type: nzbget
  title: NZBGet
  url: http://nzbget:6789
  secrets: {user: nzbget_user, pass: nzbget_pass}
```

## Fields
On the tile: « Réception » (download rate; curve), « En file » (queued), « Reste à télécharger » (remaining to download; with estimated time), « État » (state).
In the detail view: free space, speed limit, downloaded since startup, post-processing, active threads, article cache, uptime, failures in history, and the tables current downloads and recent history (15 entries).

## Limits
- State and queue every 3 s, history every minute (it can be long: "failures" counts the whole history).
- No per-day or per-server volumes (the `servervolumes` method is not read).
- Tested only against a fake server that follows the API documentation, not against a real NZBGet.
