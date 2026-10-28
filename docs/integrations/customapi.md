# Generic JSON API (customapi)

> Version française : [../fr/integrations/customapi.md](../fr/integrations/customapi.md)

A tile for any API that answers in JSON: you declare the fields to read and their format. Read-only (GET).

## Configuration

```yaml
- id: sauvegarde
  type: customapi
  title: Sauvegardes
  url: https://backup.example.org/api/status
  secrets: {token: backup_token}      # optional
  options:
    interval: 5m                      # default 60s, minimum 5s
    header.Accept-Language: fr        # fixed headers: header.<Name>
  fields:
    - {id: last, label: Dernière sauvegarde, path: jobs.0.finished_at, format: date, warn: 90000, crit: 180000}
    - {id: size, label: Taille, path: jobs.0.bytes, format: bytes}
    - {id: jobs, label: Tâches, path: jobs.#, format: number, placement: detail}
    - {id: disk, label: Disque, path: storage.used_pct, format: percent, warn: 80, crit: 90}
```

(The labels are free text shown as-is in the French UI: last backup, size, jobs, disk.)

## Paths

- Keys separated by dots: `data.items.0.count`; a negative index counts from the end: `items.-1.name`.
- `#` at the end of a path: length of an array or object (`null` counts as 0).
- Empty path: the root (an API that answers with a bare number).
- Keys containing a dot cannot be reached.

## Formats

| Format | Rendering |
|---|---|
| `text` (default) | raw value; boolean shown as « oui »/« non » (yes/no) |
| `number` | French-style number, thousands grouped |
| `bytes`, `rate` | « 1,5 Go », « 2,5 ko/s » |
| `percent` | 0-100 gauge |
| `duration` | seconds -> « 1 j 2 h » (1 d 2 h) |
| `date` | RFC 3339 or Unix epoch (s, ms, µs, ns recognized) -> « 27/10/2026 08:00 », « il y a 3 h » (3 h ago) |

Numbers in strings (`"91.2"`) are accepted. `warn`/`crit` color the value (>= threshold); a `warn` greater than `crit` reverses the direction (lower = worse, e.g. free space). For `date`, thresholds apply to the age in seconds. `placement`: `tile` (default), `detail` or `hidden`.

## Secrets (all optional)

| Role | Effect |
|---|---|
| `token` | `Authorization: Bearer ...` |
| `key` | `X-Api-Key` header (or `options.key_header`) |
| `user`, `pass` | basic authentication (both together) |
| `post_key` | `Authorization: Bearer …` for the `post_url` button only |

Provide a read-only key if the API allows it. Avoid putting a key in the URL: it is stripped from error messages, but stays in clear text in config.yaml.

## Limits

- Reads use GET only (`options.method: POST` is rejected). One exception, an optional button: `options.post_url` (empty POST on click), `options.post_label` (label), with a `post_key` separate from the read key. `post_url` must use the same scheme and host:port as `url`.
- A path not found, a `null` value or a non-numeric value shows "—" with the reason, without affecting other fields. API unreachable or non-JSON response: the whole tile goes into error.
- No generic list or table: one value per field.
