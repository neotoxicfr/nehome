# SABnzbd

> Version française : [../fr/integrations/sabnzbd.md](../fr/integrations/sabnzbd.md)


Monitors the SABnzbd Usenet client: download rate chart, queue, remaining to download, history, per-server volumes. Read-only (`queue`, `history` and `server_stats` modes).

## Options and secrets
- `url`: SABnzbd address (`http://host:8080`, with the `url_base` prefix if any).
- `insecure_tls`: accept a self-signed certificate.
- Secret `key` (required): the API key (Config, General). Use the **API key**, not the NZB key: the latter does not open these modes. SABnzbd has no per-key permissions. The key travels in the request URL (the only accepted way); nehome strips it from every displayed error.

```yaml
- id: sabnzbd
  type: sabnzbd
  title: SABnzbd
  url: http://sabnzbd:8080
  secrets: {key: sabnzbd_key}
```

## Fields
On the tile: « Réception » (download rate, chart), « En file » (queued), « Reste à télécharger » (remaining to download, with estimated time), « État » (status).
In the detail view: « Espace libre » (free space; temporary folder, completed folder as subtitle), « Limite de débit » (speed limit), « Avertissements » (warnings), « Échecs récents » (recent failures), « Aujourd'hui » / « Cette semaine » / « Ce mois » / « Total depuis l'origine » (today / this week / this month / all-time total), « Version », and the tables « Téléchargements en cours » (active downloads), « Historique récent » (recent history, 15 entries) and « Serveurs Usenet » (Usenet servers).

## Limits
- Queue every 3 s, history every minute, server statistics every 5 min.
- « Échecs récents » only counts the last 15 history entries.
- Tested only against a fake server matching the API documentation, not against a real SABnzbd.
