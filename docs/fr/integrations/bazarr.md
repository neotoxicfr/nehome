# Bazarr

Sous-titres manquants (épisodes, films), fournisseurs en pause, santé, liaison Sonarr/Radarr et version.

## Configuration

```yaml
- id: bazarr
  type: bazarr
  title: Bazarr
  section: Médias
  href: https://bazarr.exemple.org/
  url: http://bazarr:6767
  secrets: {key: bazarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Bazarr, Réglages, Général), envoyée dans l'en-tête `X-API-KEY` |

Pas de clé en lecture seule : nehome n'émet que des GET.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `episodes`, `movies`, `providers`, `health` | Épisodes et films sans sous-titres, Fournisseurs en pause, Santé | tuile |
| `links`, `provider_list`, `version` | liaison Sonarr/Radarr (SignalR), table des fournisseurs, version | détail |

## Rythme

Badges et santé chaque minute ; version et fournisseurs toutes les 5 min.

## Limites

Les compteurs viennent de `/api/badges` (éléments encore sans sous-titres). Pas d'historique des téléchargements de sous-titres. Non testé sur un vrai Bazarr : réponses simulées d'après son code source.
