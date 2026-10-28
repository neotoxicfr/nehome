# AdGuard Home

> Version française : [../fr/integrations/adguard.md](../fr/integrations/adguard.md)

DNS queries over 24 h, blocked share, protection status, most active clients and domains, filter lists. One tile per instance.

## Configuration

```yaml
- id: adguard
  type: adguard
  title: AdGuard
  section: Réseau
  href: https://adguard.example.org/
  url: http://192.0.2.53:3000
  secrets: {user: adguard_user, pass: adguard_pass}
```

For several instances, repeat the tile with a different `id`, `url` and secrets.

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | web interface account (basic authentication on `/control`) |

AdGuard Home has no read-only role: the account sees everything, but nehome only makes GET requests.

## Fields

| ID | Label | Default |
|---|---|---|
| `queries`, `blocked`, `curve`, `protection` | « Requêtes (24 h) » (queries), « Bloquées » (blocked, gauge), « Requêtes par heure » (queries per hour), « Protection » | tile |
| `avg`, `upstream`, `version`, `rules` | average time, upstream server, version, filtering rules | detail |
| `clients`, `domains`, `blockedtop`, `lists` | tables: active clients, queried domains, blocked domains, filter lists | detail |

## Polling

Statistics every minute; filter lists every 10 min.
