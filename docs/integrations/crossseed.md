# cross-seed

> Version française : [../fr/integrations/crossseed.md](../fr/integrations/crossseed.md)

Only shows whether cross-seed is reachable.

## Configuration

```yaml
- id: crossseed
  type: crossseed
  title: cross-seed
  section: Téléchargement
  url: http://cross-seed:2468
```

## Secrets

| Role | Use |
|---|---|
| `key` | optional: cross-seed's `apiKey` (`X-Api-Key` header), enables the « Lancer le RSS » and « Lancer une recherche » actions (`POST /api/job`) |

## Fields

| ID | Label | Default |
|---|---|---|
| `state` | « État » (state) | tile |

## Limits

cross-seed only exposes `/api/ping` for reading (no key needed). The history of searches and injections is only in its SQLite database, with no API. `/api/job` is only called by the actions (with the key); `/api/webhook` and `/api/announce` never. Polled every 30 s.
