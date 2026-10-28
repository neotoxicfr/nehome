# Plex

> Version française : [../fr/integrations/plex.md](../fr/integrations/plex.md)

Current streams, outgoing bandwidth, movie and show counts, recent additions and libraries.

## Configuration

```yaml
- id: plex
  type: plex
  title: Plex
  section: Médias
  href: https://plex.example.org/
  url: http://plex:32400
  secrets: {token: plex_token}
```

## Secrets

| Role | Content |
|---|---|
| `token` | Plex token (`X-Plex-Token`), sent as a header |

Plex offers no restricted token: the server owner's token grants all rights. nehome only sends GETs.

## Fields

| ID | Label | Default |
|---|---|---|
| `streams`, `bandwidth`, `movies`, `shows` | « Lectures en cours » (current streams), « Débit sortant » (outgoing bandwidth), « Films » (movies), « Séries et animés » (shows and anime) | tile |
| `activities`, `version` | « Activités du serveur » (server activities), « Version » (version) | detail |
| `sessions`, `recent`, `libraries` | tables: stream details (mode, transcoding shown as a warning), recent additions, libraries | detail |

## Refresh rate

Streams every 5 s; additions, activities and version every minute; per-library counts every 5 min. A failure of a secondary request clears nothing.
