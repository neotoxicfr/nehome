# Jackett

> Version française : [../fr/integrations/jackett.md](../fr/integrations/jackett.md)

Number of configured indexers and of those in error, with each one's last known error.

## Configuration

```yaml
- id: jackett
  type: jackett
  title: Jackett
  section: Téléchargement
  url: http://jackett:9117
  secrets: {pass: jackett_pass}
```

## Secrets

| Role | Content |
|---|---|
| `pass` | Jackett admin password (cookie login) |

Jackett has a single administrator account: the password grants all rights, but nehome only reads the indexer list.

## Fields

| ID | Label | Default |
|---|---|---|
| `errors`, `count` | « Indexeurs en erreur » (indexers in error), « Indexeurs » (indexers) | tile |
| `list` | Configured indexers (table) | detail |

## Polling and limits

Every minute. Polling tests no indexer and runs no search (each would query a tracker): only the last error Jackett already knows about is displayed. The « Retester les indexeurs en erreur » and « Tester un indexeur » actions call `POST /api/v2.0/indexers/{id}/test`, one indexer at a time.
