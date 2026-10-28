# Seerr (Overseerr / Jellyseerr)

Demandes de médias en attente, en cours, problèmes ouverts, et liste des demandes à traiter.

## Configuration

```yaml
- id: seerr
  type: seerr
  title: Seerr
  section: Médias
  url: http://seerr:5055
  secrets: {key: seerr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Réglages, Général), envoyée en en-tête `X-Api-Key` |

La clé est celle de l'administrateur ; nehome n'émet que des GET sur `/api/v1`.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `pending`, `processing`, `issues` | Demandes en attente, Demandes en cours, Problèmes ouverts | tuile |
| `total`, `version` | Demandes au total, Version | détail |
| `requests` | Demandes à traiter (table) | détail |

## Rythme et limites

Toutes les minutes. Les titres ne sont pas dans la liste des demandes : nehome les résout par un appel supplémentaire au film ou à la série (avec cache) ; en cas d'échec, un repli lisible s'affiche, sans mise en cache.
