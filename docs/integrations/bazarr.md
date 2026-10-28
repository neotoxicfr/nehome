# Bazarr

> Version française : [../fr/integrations/bazarr.md](../fr/integrations/bazarr.md)

Missing subtitles (episodes, movies), paused providers, health, Sonarr/Radarr link and version.

## Configuration

```yaml
- id: bazarr
  type: bazarr
  title: Bazarr
  section: Médias
  href: https://bazarr.example.org/
  url: http://bazarr:6767
  secrets: {key: bazarr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Bazarr, Settings, General), sent in the `X-API-KEY` header |

No read-only key: nehome only makes GET requests.

## Fields

| ID | Label | Default |
|---|---|---|
| `episodes`, `movies`, `providers`, `health` | Episodes and movies without subtitles, « Fournisseurs en pause » (paused providers), « Santé » (health) | tile |
| `links`, `provider_list`, `version` | Sonarr/Radarr link (SignalR), providers table, version | detail |

## Polling

Badges and health every minute; version and providers every 5 min.

## Limits

Counters come from `/api/badges` (items still without subtitles). No history of subtitle downloads. Not tested against a real Bazarr: simulated responses based on its source code.
