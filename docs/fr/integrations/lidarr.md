# Lidarr

File d'attente, éléments manquants, santé, bibliothèque musicale, calendrier des sorties, derniers imports et espace disque.

## Configuration

```yaml
- id: lidarr
  type: lidarr
  title: Lidarr
  section: Médias
  href: https://lidarr.exemple.org/
  url: http://lidarr:8686
  secrets: {key: lidarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Lidarr, Réglages, Général), envoyée dans l'en-tête `X-Api-Key` |

Pas de clé en lecture seule : nehome n'émet que des GET (API v1).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `queue`, `missing`, `health`, `library` | File d'attente, Manquants, Santé, Artistes | tuile |
| `cutoff`, `version` | Cutoff non atteint, Version | détail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | téléchargements en cours, calendrier sur 7 jours, derniers imports, espace disque, tâches planifiées | détail |

## Rythme

File, manquants et imports toutes les 30 s ; calendrier, bibliothèque et disques toutes les 5 min.

## Limites

Les derniers imports sont les pistes importées (événement 3). « Manquants » compte les albums. Non testé sur une vraie instance : réponses simulées d'après le code source.
