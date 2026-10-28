# Tracearr

> Version française : [../fr/integrations/tracearr.md](../fr/integrations/tracearr.md)


Monitoring of media server accounts: current streams, plays today, violations, playback quality, platforms, account trust score.

## Configuration

```yaml
- id: tracearr
  type: tracearr
  title: Tracearr
  section: Médias
  url: http://tracearr:3000
  secrets: {key: tracearr_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | public API v1 token (sent as `Bearer`), created in Tracearr's settings |

nehome only calls read routes of `/api/v1/public`.

## Fields

| ID | Label | Default |
|---|---|---|
| `streams`, `today`, `violations`, `plays` | « Flux en cours » (current streams), « Lectures aujourd'hui » (plays today), « Violations récentes » (recent violations), « Lectures par jour » (plays per day, 30 d) | tile |
| `users`, `server` | « Comptes » (accounts), « Serveurs » (servers) | detail |
| `quality` | « Qualité de lecture » (playback quality, 30 d, gauges) | detail |
| `details`, `platforms`, `trust` | tables: detailed streams (codecs, HDR), platforms, accounts to watch | detail |

## Polling

Streams every 5 s, counters every minute, 30-day statistics every 5 min.
