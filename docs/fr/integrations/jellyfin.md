# Jellyfin et Emby

Lectures en cours (dont transcodages), compteurs de bibliothèque, activité récente et version. Même API : `type: jellyfin` ou `type: emby`.

## Configuration

```yaml
- id: jellyfin
  type: jellyfin        # ou emby
  title: Jellyfin
  section: Médias
  href: https://jellyfin.exemple.org/
  url: http://jellyfin:8096
  secrets: {key: jellyfin_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Jellyfin : Tableau de bord, Clés d'API ; Emby : Avancé, Clés d'API), envoyée dans l'en-tête `X-Emby-Token` |

Une clé d'API a les droits d'administrateur : nehome n'émet que des GET. Emby est interrogé sous `/emby`.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `streams`, `transcodes`, `movies`, `series` | Lectures en cours, Transcodages, Films, Séries | tuile |
| `episodes`, `music`, `books` | Épisodes, Musique (morceaux, albums, artistes), Livres | détail |
| `sessions`, `activity`, `version` | sessions en lecture, activité récente, version (alerte si mise à jour ou redémarrage en attente) | détail |

## Rythme

Sessions toutes les 10 s ; compteurs, version et activité toutes les 5 min.

## Limites

Seules les sessions avec un média en cours comptent. `music` et `books` sont masqués si la bibliothèque n'en a pas. Les compteurs sont globaux (pas par bibliothèque). Non testé sur un vrai serveur : réponses simulées d'après l'OpenAPI de Jellyfin.
