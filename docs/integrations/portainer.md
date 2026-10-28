# Portainer

> Version française : [../fr/integrations/portainer.md](../fr/integrations/portainer.md)

Running, stopped and unhealthy containers, environments, stacks.

## Configuration

```yaml
- id: portainer
  type: portainer
  title: Portainer
  section: Infrastructure
  href: https://portainer.example.org/
  url: http://192.0.2.10:9000
  secrets: {key: portainer_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | access token (My account > Access tokens), sent as `X-API-Key` |

A standard user is enough to see the environments they have access to; nothing is ever written.

## Fields

| ID | Content | Default |
|---|---|---|
| `running, unhealthy, endpoints, stacks` | running containers, unhealthy containers, environments online, active stacks | tile |
| `version, envtable, stacktable` | version, per-environment detail, stacks | detail |

## Refresh rate

Every 30 s.

## Limits

Counts come from Portainer's environment snapshots (refreshed by Portainer, so a few minutes of lag are possible). Stacks are not broken down by container.
