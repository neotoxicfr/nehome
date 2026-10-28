# Readarr

> Version française : [../fr/integrations/readarr.md](../fr/integrations/readarr.md)


Queue, missing items, health, book library, release calendar, latest imports and disk space.

## Configuration

```yaml
- id: readarr
  type: readarr
  title: Readarr
  section: Médias
  href: https://readarr.example.org/
  url: http://readarr:8787
  secrets: {key: readarr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Readarr, Settings, General), sent in the `X-Api-Key` header |

No read-only key: nehome only sends GET requests (API v1).

## Fields

| ID | Label | Default |
|---|---|---|
| `queue`, `missing`, `health`, `library` | « File d'attente » (queue), « Manquants » (missing), « Santé » (health), « Auteurs » (authors) | tile |
| `cutoff`, `version` | « Cutoff non atteint » (cutoff unmet), « Version » | detail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | active downloads, 7-day calendar, latest imports, disk space, scheduled tasks | detail |

## Polling

Queue, missing and imports every 30 s; calendar, library and disks every 5 min.

## Limits

Readarr is archived upstream: API v1 stays as described here, with no guarantee of further changes. The latest imports are the imported book files. Not tested on a real instance: simulated responses based on the source code.
