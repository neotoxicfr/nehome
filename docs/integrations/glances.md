# Glances

> Version française : [../fr/integrations/glances.md](../fr/integrations/glances.md)

CPU, load, memory, swap, disks, network, temperatures and the most demanding processes of a machine.

## Configuration

```yaml
- id: glances
  type: glances
  title: Glances
  section: Infrastructure
  href: https://glances.example.org/
  url: http://192.0.2.10:61208
  secrets: {user: glances_user, pass: glances_pass}   # optional
```

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | optional, together: basic authentication if Glances was started with `--password` |

Read-only; Glances' REST API only exposes GETs.

## Fields

| ID | Content | Default |
|---|---|---|
| `cpu, ram, load, temp` | CPU, memory, load (1 min), maximum temperature | tile |
| `curve, swap, system, uptime, disks, net, sensors, procs` | CPU history, swap, system, uptime, disk gauges, rates per interface, sensors, top processes | detail |

## Polling

CPU, memory, load and network every 5 s; the rest every minute.

## Limits

Run Glances in web server mode (`glances -w`). API v4 first, automatic fallback to v3. One tile per machine. Network rates rely on Glances' `*_rate_per_sec` fields.
