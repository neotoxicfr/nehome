# Healthchecks

> Version française : [../fr/integrations/healthchecks.md](../fr/integrations/healthchecks.md)

Scheduled-job checks (healthchecks.io or self-hosted), failures first. Polled every 30 s.

## Configuration

- Secret `key`: **read-only** API key (Project settings > API keys).
- Options: none. `url` = `https://healthchecks.io` for the hosted service.

```yaml
- id: healthchecks
  type: healthchecks
  title: Healthchecks
  url: https://healthchecks.io
  secrets: {key: hc_readonly_key}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| state | « État global » (overall state) | tile |
| down | « En panne » (down) | tile |
| up | « En ligne » (up) | tile |
| grace | Grace period | detail |
| paused | Paused | detail |
| checks | Checks, failures first (15 max) | detail |

## Limits

- An API key covers a single project.
- Not proven on a real instance.
