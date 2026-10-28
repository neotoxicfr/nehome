# Traefik

Nombre de routeurs, services, middlewares, certificats, fournisseurs, points d'entrée, et liste des routeurs ou serveurs en erreur.

## Configuration

```yaml
- id: traefik
  type: traefik
  title: Traefik
  section: Réseau
  url: http://traefik:8080
```

`url` : l'API interne de Traefik (`api.insecure: true` sur un point d'entrée `traefik` non publié, ou un routeur dédié `api@internal`). nehome lit `/api`. Sans authentification, ne l'exposez que sur un réseau Docker privé.

## Secrets

Aucun.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `routers`, `services`, `health`, `version` | Routeurs, Services, État, Version | tuile |
| `middlewares`, `certs`, `providers` | Middlewares, Certificats, Fournisseurs | détail |
| `entrypoints`, `badrouters`, `down` | tables : points d'entrée, routeurs en erreur, serveurs hors service | détail |

## Rythme et limites

Toutes les 30 s. Les métriques Prometheus ne sont pas lues (elles ne sont pas sur le port de l'API).
