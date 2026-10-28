# Sonarr

> Version française : [../fr/integrations/sonarr.md](../fr/integrations/sonarr.md)


Queue, missing items, health, library size, calendar, latest imports and disk space.

## Configuration

```yaml
- id: sonarr
  type: sonarr
  title: Sonarr
  section: Médias
  href: https://sonarr.example.org/
  url: http://sonarr:8989
  secrets: {key: sonarr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Sonarr, Settings, General), sent in the `X-Api-Key` header |

There is no read-only key: nehome only sends GET requests (API v3).

## Fields

| ID | Label | Default |
|---|---|---|
| `queue`, `missing`, `health`, `library` | « File d'attente » (queue), « Manquants » (missing), « Santé » (health), « Séries » (series) | tile |
| `cutoff`, `version` | « Cutoff non atteint » (cutoff unmet), « Version » | detail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | tables and gauges: active downloads, 7-day calendar, latest imports, disk space, scheduled tasks | detail |

## Polling

Queue, missing and imports every 30 s; calendar, library (full list, several MB) and disks every 5 min. Each request is independent: one failing does not clear the others.
