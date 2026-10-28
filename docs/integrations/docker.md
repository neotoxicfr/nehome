# Docker

> Version française : [../fr/integrations/docker.md](../fr/integrations/docker.md)

Container state: running, healthy, in trouble, resources, images, and a summary of a nightly update.

## Configuration

```yaml
- id: docker
  type: docker
  title: Docker
  section: Système
  url: http://socket-proxy:2375
  options: {update_log: /logs/update.log}
```

`url`: Docker API over HTTP, through a read-only socket proxy (for example `tecnativa/docker-socket-proxy` with `CONTAINERS=1`, `IMAGES=1`). Default: `http://socket-proxy:2375`. **Do not mount `/var/run/docker.sock` into nehome.**

## Options

| Option | Role |
|---|---|
| `update_log` | path (inside the nehome container) of an update log, mounted read-only. Without it, the update fields are hidden. |

The log is that of an `update.sh` script specific to the original installation; the expected format is not documented elsewhere, so leave the option empty if you have none.

## Secrets

None.

## Fields

| ID | Label | Default |
|---|---|---|
| `running`, `healthy`, `trouble` | running, healthy, problem containers | tile |
| `update` | nightly update | tile |
| `total`, `unhealthy`, `stopped`, `created`, `images`, `usage` | total, unhealthy, stopped, never started, images, cumulative resources | detail |
| `problems`, `cpu`, `ram`, `update_detail` | tables: containers to watch, CPU and memory per container (first 15), update detail | detail |

## Polling and limits

List and inspection every minute (stats take about 1 s per container, 8 in parallel); images every 5 min. A proxy that denies `/images` does not clear the tile. Read-only: GET requests only.
