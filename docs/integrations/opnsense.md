# OPNsense

> Version française : [../fr/integrations/opnsense.md](../fr/integrations/opnsense.md)

Gateways (state, latency, loss), memory, load, interfaces, disks and firewall updates.

## Configuration

```yaml
- id: opnsense
  type: opnsense
  title: OPNsense
  section: Infrastructure
  href: https://opnsense.example.org/
  url: https://192.0.2.1
  insecure_tls: true
  secrets: {key: opn_key, secret: opn_secret}
```

## Secrets

| Role | Content |
|---|---|
| `key`, `secret` | API key and secret of a user (System > Access > Users > API keys) |

Dedicated user, in a group that has only these privileges: Diagnostics: System Activity, Diagnostics: Interface (interface overview), System: Gateways, System: Firmware (read). nehome only sends GETs.

## Fields

| ID | Content | Default |
|---|---|---|
| `gateways, ram, load, firmware` | gateways online, memory, load, updates | tile |
| `uptime, version, gwtable, ifaces, disks` | uptime, version, gateway table, interfaces, disk gauges | detail |

## Refresh rate

Every 30 s; firmware status every 10 min.

## Limits

No CPU percentage (the API only provides it as a stream): load replaces it. The interface overview needs OPNsense 24.1 or later. The `/api/diagnostics/system/*` and `/api/core/firmware/status` paths follow the documentation but have not been verified on a real firewall; missing fields are simply hidden.
