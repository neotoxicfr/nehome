# Prometheus

> Version française : [../fr/integrations/prometheus.md](../fr/integrations/prometheus.md)

Reads Prometheus metrics in two ways:

- **Prometheus server**: each field is an instant PromQL query (`/api/v1/query`); the value is the first result (scalar, string or first element of the vector).
- **`/metrics` endpoint** (`scrape: true`): nehome reads an exporter's text page directly; each field is a series name, with a simple label selector. Several matching series are summed.

## Configuration

```yaml
- id: prom
  type: prometheus
  title: Prometheus
  url: http://prometheus:9090
  fields:
    - {id: up, label: Cibles en service, path: 'sum(up)', format: number}
    - {id: cpu, label: CPU, path: '100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100', format: percent, warn: 80, crit: 95}

- id: exporter
  type: prometheus
  title: Node exporter
  url: http://192.0.2.10:9100/metrics
  options: {scrape: true, interval: 30s}
  fields:
    - {id: free, label: Libre sur /, path: 'node_filesystem_avail_bytes{mountpoint="/"}', format: bytes}
    - {id: boot, label: Démarré, path: node_boot_time_seconds, format: date}
```

Formats, thresholds, placement, `interval`, `header.<Name>`: same as [customapi](customapi.md).

## Secrets (optional)

`token` (Bearer), `key` (+ `options.key_header`), `user`/`pass` (basic): useful behind an authenticating proxy. Prometheus itself has no per-key permissions; nehome only calls `/api/v1/query` with GET.

## Limits

- Instant queries only; a `matrix` result (`[1h:]` subquery) is rejected. To count or aggregate, use `count()`, `sum()`... in the query.
- Scrape mode: equality-only selector (`=`, no `!=` or `=~`); quoted UTF-8 series names are not supported; text format (Accept `text/plain`), no protobuf.
- An invalid query or one with no result shows "—" along with Prometheus's message; if all of them fail for a network or HTTP reason, the tile goes into error. Queries are sent one after the other: keep their number reasonable.
