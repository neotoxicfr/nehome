# Link

> Version française : [../fr/integrations/link.md](../fr/integrations/link.md)

A plain link to an application, optionally with its Docker container state and/or an HTTP probe.

## Configuration

```yaml
- id: wiki
  type: link
  title: Wiki
  section: Applications
  href: https://wiki.example.org/
  insecure_tls: false
  options:
    container: wiki                         # optional
    docker_url: http://socket-proxy:2375    # optional
    health_url: http://wiki:3000/health     # optional
    post_url: http://wiki:3000/hook         # optional
    post_label: Rebuild                     # optional
```

With no options, the tile is just a link.

## Options

| Option | Purpose |
|---|---|
| `container` | name of the container whose state is read (through the Docker API, read-only) |
| `docker_url` | socket proxy (default `http://socket-proxy:2375`) |
| `health_url` | `http(s)` URL probed with a GET; if invalid, the tile shows an error |
| `post_url` | `http(s)` URL called with an empty POST when the tile's button is clicked (wake a PC, webhook…) |
| `post_label` | label of that button (default « Déclencher ») |

## Secrets

None. `insecure_tls` applies to the probe.

## Fields

| ID | Label | Default |
|---|---|---|
| `state`, `health`, `uptime` | « État » (state), « Santé HTTP » (HTTP health), « En marche depuis » (running for) | tile |
| `image`, `restarts` | « Image » (image), « Redémarrages » (restarts) | detail |

## Refresh rate and limits

Every 30 s. Probe: network error or 5xx is critical, 4xx is a warning (the application answers but rejects the probe), latency above 1.5 s is a warning. A redirect to another host (the login page of an authentication proxy) is not followed and counts as proof that the application is responding.
