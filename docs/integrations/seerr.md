# Seerr (Overseerr / Jellyseerr)

> Version française : [../fr/integrations/seerr.md](../fr/integrations/seerr.md)


The `overseerr` and `jellyseerr` types are aliases of `seerr`: same API, same behavior.

Pending and in-progress media requests, open issues, and the list of requests to handle.

## Configuration

```yaml
- id: seerr
  type: seerr
  title: Seerr
  section: Médias
  url: http://seerr:5055
  secrets: {key: seerr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Settings, General), sent in the `X-Api-Key` header |

The key is the administrator's; nehome only sends GET requests on `/api/v1`.

## Fields

| ID | Label | Default |
|---|---|---|
| `pending`, `processing`, `issues` | « Demandes en attente » (pending requests), « Demandes en cours » (requests in progress), « Problèmes ouverts » (open issues) | tile |
| `total`, `version` | « Demandes au total » (total requests), « Version » | detail |
| `requests` | « Demandes à traiter » (requests to handle, table) | detail |

## Polling and limits

Every minute. Titles are not in the request list: nehome resolves them with an extra call to the movie or series (cached); on failure, a readable fallback is shown and not cached.
