# Prowlarr

> Version française : [../fr/integrations/prowlarr.md](../fr/integrations/prowlarr.md)

Active or unavailable indexers, searches and grabs over the last 24 h, failures, health and version.

## Configuration

```yaml
- id: prowlarr
  type: prowlarr
  title: Prowlarr
  section: Médias
  href: https://prowlarr.example.org/
  url: http://prowlarr:9696
  secrets: {key: prowlarr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Prowlarr, Settings, General), sent in the `X-Api-Key` header |

No read-only key: nehome only sends GETs (API v1).

## Fields

| ID | Label | Default |
|---|---|---|
| `indexers`, `queries`, `grabs`, `health` | « Indexeurs actifs » (active indexers), « Recherches (24 h) » (searches), « Grabs (24 h) » (grabs), « Santé » (health) | tile |
| `failed`, `down`, `stats`, `version` | failed searches, unavailable indexers (disabled until), per-indexer statistics, version | detail |

## Refresh rate

Indexers and health every minute; statistics and version every 5 min.

## Limits

"Active" excludes indexers disabled by hand; those temporarily paused after failures raise an alert. Statistics are limited to the last 24 hours. Not tested against a real Prowlarr: simulated responses based on its source code.
