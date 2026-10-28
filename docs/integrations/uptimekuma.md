# Uptime Kuma

> Version française : [../fr/integrations/uptimekuma.md](../fr/integrations/uptimekuma.md)


Monitor status, 24 h availability, average latency, ongoing incident.

## Configuration

```yaml
- id: uptime
  type: uptimekuma
  title: Disponibilité
  section: Supervision
  url: http://uptime-kuma:3001
  options: {slugs: "services,reseau"}
  secrets: {key: uptimekuma_key}      # optional
```

## Options

| Option | Purpose | Default |
|---|---|---|
| `slugs` | public status pages to read, comma-separated | `plex` |

Uptime Kuma 2 has no REST API: nehome reads the public status pages. Monitors absent from those pages are not seen, unless the API key is set.

## Secrets

| Role | Content |
|---|---|
| `key` | Uptime Kuma API key (Settings, API Keys); **optional** |

With the key, `/metrics` (basic authentication: empty username, key as password) adds all monitors. If `secrets.key` is declared, the file must exist.

## Fields

| ID | Label | Default |
|---|---|---|
| `summary`, `uptime`, `latency` | « État » (status), « Disponibilité (24 h) » (availability), « Latence moyenne » (average latency) | tile |
| `incident` | « Incident en cours » (ongoing incident) | detail |
| `monitors` | « Moniteurs » (monitors), failing ones first | detail |
| `metrics` | all monitors (requires the key) | detail |

## Polling

Every 30 s.
