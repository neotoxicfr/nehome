# Pi-hole

> Version française : [../fr/integrations/pihole.md](../fr/integrations/pihole.md)

DNS queries, blocked share, blocking state, blocklist size, most active clients and domains.

## Configuration

```yaml
- id: pihole
  type: pihole
  title: Pi-hole
  section: Infrastructure
  href: https://pihole.example.org/
  url: http://192.0.2.53
  secrets: {pass: pihole_pass}
  # Pi-hole v5:
  # options: {api: "5"}
  # secrets: {token: pihole_token}
```

## Secrets

| Role | Content |
|---|---|
| `pass` | v6: the interface password or, better, an app password |
| `token` | v5 (`options.api: "5"`): API token (Settings > API > Show token) |

Pi-hole has no read-only role; nehome only reads. On v6, a dedicated app password is recommended. The session is reopened if it expires.

## Fields

| ID | Content | Default |
|---|---|---|
| `queries, blocked, blocking, gravity` | queries, blocked share (gauge), blocking state, blocked domains | tile |
| `clients, cached, forwarded, unique, version, topclients, topdomains, topblocked` | active clients, cached replies, forwarded queries, unique domains, version, top tables | detail |

## Refresh rate

Every 30 s.

## Limits

On v5 the token goes in the URL (required by api.php): it is stripped from all error messages, but may appear in the Pi-hole server logs. On v5 there is no active client count, only unique clients. The authentication mode is chosen with `options.api`; it is not detected.
