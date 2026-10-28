# Audiobookshelf

> Version française : [../fr/integrations/audiobookshelf.md](../fr/integrations/audiobookshelf.md)

Library items, duration and size, current listening streams, and version.

## Configuration

```yaml
- id: audiobookshelf
  type: audiobookshelf
  title: Audiobookshelf
  section: Médias
  href: https://abs.example.org/
  url: http://audiobookshelf:80
  secrets: {token: abs_token}
```

## Secrets

| Role | Content |
|---|---|
| `token` | API token of an **administrator** account (Settings, Users), sent as `Authorization: Bearer` |

The account must be an administrator: `/api/users/online` is admin-only. nehome only makes GET requests.

## Fields

| ID | Label | Default |
|---|---|---|
| `items`, `streams`, `duration`, `size` | « Éléments » (items), « Écoutes en cours » (current streams), « Durée totale » (total duration), « Taille » (size) | tile |
| `authors`, `libraries`, `sessions`, `version` | « Auteurs » (authors), libraries table, open sessions, version | detail |

## Polling

Sessions every 15 s; libraries (one statistics request per library) and version every 5 min.

## Limits

Library statistics are not emitted if none of the requests answers. An open session is not necessarily actively playing. Not tested against a real server: simulated responses based on the API docs.
