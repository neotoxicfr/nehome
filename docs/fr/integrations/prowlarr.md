# Prowlarr

Indexeurs actifs ou indisponibles, recherches et grabs des dernières 24 h, échecs, santé et version.

## Configuration

```yaml
- id: prowlarr
  type: prowlarr
  title: Prowlarr
  section: Médias
  href: https://prowlarr.exemple.org/
  url: http://prowlarr:9696
  secrets: {key: prowlarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Prowlarr, Réglages, Général), envoyée dans l'en-tête `X-Api-Key` |

Pas de clé en lecture seule : nehome n'émet que des GET (API v1).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `indexers`, `queries`, `grabs`, `health` | Indexeurs actifs, Recherches (24 h), Grabs (24 h), Santé | tuile |
| `failed`, `down`, `stats`, `version` | recherches en échec, indexeurs indisponibles (désactivés jusqu'à), statistiques par indexeur, version | détail |

## Rythme

Indexeurs et santé chaque minute ; statistiques et version toutes les 5 min.

## Limites

« Actifs » exclut les indexeurs désactivés à la main ; ceux en pause temporaire après échecs passent en alerte. Statistiques limitées aux 24 dernières heures. Non testé sur un vrai Prowlarr : réponses simulées d'après son code source.
