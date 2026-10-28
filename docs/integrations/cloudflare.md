# Cloudflare

> Version française : [../fr/integrations/cloudflare.md](../fr/integrations/cloudflare.md)

Requests, threats, cache ratio, bandwidth over 24 h, and the latest firewall events of a zone.

## Configuration

```yaml
- id: cloudflare
  type: cloudflare
  title: Cloudflare
  section: Réseau
  options: {zone_name: example.org}
  secrets: {token: cloudflare_token}
```

## Options

| Option | Role |
|---|---|
| `zone_name` | zone name, resolved to an ID through the API. **Set it**: the default value hardcoded in the program is the author's own domain. |
| `zone` | zone ID; skips the resolution (and the "Zone Read" permission) |

## Secrets

| Role | Content |
|---|---|
| `token` | Cloudflare API token |

Minimum token permissions, scoped to the zone: Analytics Read and Firewall Read (plus Zone Read if `zone` is not given). The token only travels in a header.

## Fields

| ID | Label | Default |
|---|---|---|
| `requests`, `threats`, `cache`, `bandwidth` | « Requêtes » (requests), « Menaces » (threats), « Part servie par le cache » (share served from cache), « Bande passante » (bandwidth) | tile |
| `curve` | « Requêtes par heure » (requests per hour) | detail |
| `firewall` | Latest firewall events | detail |

## Polling and limits

Every 5 min (heavy GraphQL queries, limited quota). The GraphQL request is a POST, but read-only.
