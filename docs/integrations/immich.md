# Immich

> Version française : [../fr/integrations/immich.md](../fr/integrations/immich.md)

Photos and videos, space used, server disk, per-user quotas, job queues and version.

## Configuration

```yaml
- id: immich
  type: immich
  title: Immich
  section: Médias
  href: https://photos.example.org/
  url: http://immich-server:2283
  secrets: {key: immich_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Account settings, API keys), `x-api-key` header |

Minimum permissions to tick on the key: `server.statistics`, `server.storage`, `server.about` (or `server.versionCheck`) and `job.read`. Server statistics require an administrator account.

## Fields

| ID | Label | Default |
|---|---|---|
| `photos`, `videos`, `usage`, `disk` | « Photos », « Vidéos » (videos), « Espace utilisé » (space used), « Disque du serveur » (server disk, gauge) | tile |
| `users`, `jobs`, `jobs_table`, `version` | per-user table, running jobs, non-empty job queues, version (alert if a newer one is available) | detail |

## Polling

Job queues every 30 s; statistics, storage and version every 5 min.

## Limits

Queues come from `/api/jobs`, which recent Immich versions may deprecate: if it disappears, only `jobs` and `jobs_table` stay empty. Not tested against a real Immich: simulated responses based on its OpenAPI.
