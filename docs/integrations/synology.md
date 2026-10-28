# Synology DSM

> Version française : [../fr/integrations/synology.md](../fr/integrations/synology.md)


CPU, memory, load, network throughput, volumes, temperature and disk status of a Synology NAS.

## Configuration

```yaml
- id: synology
  type: synology
  title: Synology DSM
  section: Infrastructure
  href: https://synology.example.org/
  url: https://192.0.2.30:5001
  insecure_tls: true
  secrets: {user: dsm_user, pass: dsm_pass}
```

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | local DSM account |

Use a dedicated account without 2FA (two-factor authentication is not supported). The `SYNO.Core.System`, `SYNO.Core.System.Utilization` and `SYNO.Storage.CGI.Storage` APIs generally require an account in the administrators group: nehome only calls read methods, but the account remains powerful. Restrict it by IP in DSM and with a firewall.

## Fields

| ID | Content | Default |
|---|---|---|
| `cpu, ram, volumes, temp` | CPU, memory, volumes (gauges), temperature | tile |
| `model, firmware, uptime, load, net, disktable` | model, DSM version, uptime, load, total network throughput, disk table | detail |

## Polling

Utilization every 10 s; system and storage every 5 min.

## Limits

Credentials and sid are sent by POST, never in the URL. The session is reopened if DSM drops it. The `up_time` format varies across DSM versions (seconds or text): both are handled. Tested against responses matching the API guides, not on a real DSM; `load_info` field names may differ between versions.
