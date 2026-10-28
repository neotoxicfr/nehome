# TrueNAS

> Version française : [../fr/integrations/truenas.md](../fr/integrations/truenas.md)


Tile for TrueNAS SCALE: CPU, memory, real-time network, pools, alerts, snapshots and replications.

## Configuration

```yaml
- id: truenas
  type: truenas
  title: TrueNAS
  section: Système
  href: https://truenas.example.org/
  url: https://192.0.2.10:444
  insecure_tls: true        # self-signed certificate
  secrets: {key: truenas_key}
```

`url` is the web UI address; nehome connects to it over websocket (`/api/current`, JSON-RPC 2.0). No options.

## Secrets

| Role | Content |
|---|---|
| `key` | TrueNAS API key (Credentials, API Keys) |

Minimum permissions: a read-only account is enough (« Readonly Admin » role). nehome only calls read methods.

## Fields

| ID | Label | Default |
|---|---|---|
| `cpu` | « CPU » | tile |
| `ram` | « Mémoire (hors ARC) » (memory, excluding ARC) | tile |
| `pools` | « Pools » | tile |
| `net_rx` | « Réseau réception » (network receive) | tile |
| `alerts_n` | alert count | tile |
| `net_tx`, `cpu_hist`, `cputemp`, `arc`, `disk_io`, `load`, `uptime`, `version` | network send, CPU over 2 min, CPU temperature, ARC cache, disk I/O, load, uptime, version | detail |
| `alerts`, `scrub`, `temps`, `snapshots`, `replication` | tables: alerts, pool status and last scrub, disk temperatures, periodic snapshots, replications | detail |

## Polling and limits

Real time through the `reporting.realtime` subscription (about 2 s); pools, alerts, snapshots and replications re-read every minute. The connection re-establishes itself with increasing delay (5 s to 1 min). A scrub older than 40 days raises a warning.
