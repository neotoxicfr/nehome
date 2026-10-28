# OpenMediaVault

> Version française : [../fr/integrations/openmediavault.md](../fr/integrations/openmediavault.md)

CPU, memory, load, uptime, available updates, filesystems and SMART disk health.

## Configuration

```yaml
- id: openmediavault
  type: openmediavault
  title: OpenMediaVault
  section: Infrastructure
  href: https://openmediavault.example.org/
  url: http://192.0.2.31
  secrets: {user: omv_user, pass: omv_pass}
```

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | OMV web administration account (for example `admin`) |

OMV has no read-only role for its RPC: the account is an administrator. nehome only calls `System.getInformation`, `FileSystemMgmt.enumerateFilesystems` and `Smart.getList`. Do not reuse this account elsewhere.

## Fields

| ID | Content | Default |
|---|---|---|
| `cpu, ram, fs, smart` | CPU, memory, filesystems (gauges), overall SMART health | tile |
| `host, version, uptime, load, updates, disks` | machine, version, uptime, load, updates, disk table | detail |

## Refresh rate

System every 15 s; filesystems and SMART every 5 min.

## Limits

OMV 6 and 7 (/rpc.php). Cookie-based session, automatic reconnection. SMART health only appears if SMART monitoring is enabled in OMV.
