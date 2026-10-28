# Netdata

> Version française : [../fr/integrations/netdata.md](../fr/integrations/netdata.md)

Active alarms, CPU and memory of a Netdata agent.

## Configuration

```yaml
- id: netdata
  type: netdata
  title: Netdata
  section: Infrastructure
  href: https://netdata.example.org/
  url: http://192.0.2.10:19999
  secrets: {token: netdata_token}   # optional
```

## Secrets

| Role | Content |
|---|---|
| `token` | optional: Bearer token, only useful if a proxy protects the agent |

No other secrets; the local agent is read-only on these routes.

## Fields

| ID | Content | Default |
|---|---|---|
| `alarms, cpu, ram` | active alarms (warnings and critical), CPU, memory | tile |
| `curve, host, version, children, alarmlist` | CPU history, machine, version, number of monitored machines, alarm table | detail |

## Refresh rate

Every 10 s.

## Limits

Uses API v1, which v2 agents also serve. Memory is computed from the system.ram chart (free, used, cache, buffers).
