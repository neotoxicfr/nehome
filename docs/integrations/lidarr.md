# Lidarr

> Version française : [../fr/integrations/lidarr.md](../fr/integrations/lidarr.md)

Queue, missing items, health, music library, release calendar, latest imports and disk space.

## Configuration

```yaml
- id: lidarr
  type: lidarr
  title: Lidarr
  section: Médias
  href: https://lidarr.example.org/
  url: http://lidarr:8686
  secrets: {key: lidarr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Lidarr, Settings, General), sent in the `X-Api-Key` header |

No read-only key: nehome only sends GETs (API v1).

## Fields

| ID | Label | Default |
|---|---|---|
| `queue`, `missing`, `health`, `library` | « File d'attente » (queue), « Manquants » (missing), « Santé » (health), « Artistes » (artists) | tile |
| `cutoff`, `version` | « Cutoff non atteint » (cutoff unmet), « Version » (version) | detail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | current downloads, 7-day calendar, latest imports, disk space, scheduled tasks | detail |

## Refresh rate

Queue, missing items and imports every 30 s; calendar, library and disks every 5 min.

## Limits

Latest imports are imported tracks (event 3). "Missing" counts albums. Not tested against a real instance: simulated responses based on the source code.
