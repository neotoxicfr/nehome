# Traefik

> Version française : [../fr/integrations/traefik.md](../fr/integrations/traefik.md)


Number of routers, services, middlewares, certificates, providers, entrypoints, and the list of routers or servers in error.

## Configuration

```yaml
- id: traefik
  type: traefik
  title: Traefik
  section: Réseau
  url: http://traefik:8080
```

`url`: Traefik's internal API (`api.insecure: true` on an unpublished `traefik` entrypoint, or a dedicated `api@internal` router). nehome reads `/api`. It has no authentication, so only expose it on a private Docker network.

## Secrets

None.

## Fields

| ID | Label | Default |
|---|---|---|
| `routers`, `services`, `health`, `version` | « Routeurs » (routers), « Services », « État » (status), « Version » | tile |
| `middlewares`, `certs`, `providers` | « Middlewares », « Certificats » (certificates), « Fournisseurs » (providers) | detail |
| `entrypoints`, `badrouters`, `down` | tables: entrypoints, routers in error, servers down | detail |

## Polling and limits

Every 30 s. Prometheus metrics are not read (they are not on the API port).
