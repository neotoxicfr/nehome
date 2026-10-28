# Gluetun

> Version française : [../fr/integrations/gluetun.md](../fr/integrations/gluetun.md)

VPN status, public IP, location, forwarded port, operator and DNS. One tile per instance.

## Configuration

```yaml
- id: gluetun
  type: gluetun
  title: VPN
  section: Réseau
  url: http://gluetun:8000
```

`url`: Gluetun's control server (v3, port 8000 by default).

## Secrets

| Role | Use |
|---|---|
| `key` | optional: key of an `apikey` role in the control server's auth file, sent as `X-API-Key`. Required by the « Relancer le VPN » action (`PUT /v1/vpn/status`); reads work without it if the default role allows them. |

## Fields

| ID | Label | Default |
|---|---|---|
| `vpn`, `ip`, `location`, `port` | « VPN », « IP publique » (public IP), « Localisation » (location), « Port transféré » (forwarded port) | tile |
| `org`, `hostname`, `timezone`, `dns` | Operator, hostname, time zone, VPN DNS | detail |

## Polling

Every 30 s. Read-only.
