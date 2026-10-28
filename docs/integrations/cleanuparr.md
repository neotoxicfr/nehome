# Cleanuparr

> Version française : [../fr/integrations/cleanuparr.md](../fr/integrations/cleanuparr.md)

Strikes (stalled downloads, failed imports), removed items, next cleanup, download client health and scheduled jobs from Cleanuparr.

```yaml
- {id: cleanuparr, type: cleanuparr, title: Cleanuparr, url: "http://cleanuparr:11011", secrets: {key: cleanuparr_key}}
```

| Role | Content |
|---|---|
| `key` | API key (Cleanuparr → Account → API key); password login requires 2FA, hence the key |

On the tile: downloads with strikes, removed, next cleanup, download clients. In the detail view: total strikes, replacement searches, failed jobs, strike list, latest events, scheduled jobs. Stats and strikes every minute, events and jobs every 5 min. Read-only.

## Action

« Lancer une tâche maintenant »: `POST /api/jobs/{type}/trigger`, a one-off run of a scheduled job (its schedule is unchanged).
