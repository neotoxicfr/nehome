# Proxmox VE

> Version française : [../fr/integrations/proxmox.md](../fr/integrations/proxmox.md)

Nodes, VMs and LXC containers (running or stopped, with resources), storage usage and recent failed tasks.

## Configuration

```yaml
- id: proxmox
  type: proxmox
  title: Proxmox VE
  section: Infrastructure
  href: https://proxmox.example.org/
  url: https://192.0.2.10:8006
  insecure_tls: true
  secrets: {token: proxmox_token}
```

## Secrets

| Role | Content |
|---|---|
| `token` | full API token in the format `user@realm!tokenid=secret` (Datacenter > Permissions > API Tokens) |

Token with "Privilege Separation" and the **PVEAuditor** role on `/` (propagated). No write permission.

## Fields

| ID | Content | Default |
|---|---|---|
| `guests, cpu, ram, failed` | running VMs and containers (n / total), cluster CPU and memory (online nodes), failed tasks | tile |
| `nodes, vms, storages, failedtasks` | node table, VM/LXC table, storage gauges, failed tasks | detail |

## Refresh rate

/cluster/resources every 30 s; /cluster/tasks every 5 min.

## Limits

Templates are ignored. A shared storage is counted only once. Tasks that are running or finished with a warning are not failures. Tested against responses that match the viewer API, not against a real instance.
