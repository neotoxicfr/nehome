# Sonarr

File d'attente, éléments manquants, santé, taille de la bibliothèque, calendrier, derniers imports et espace disque.

## Configuration

```yaml
- id: sonarr
  type: sonarr
  title: Sonarr
  section: Médias
  href: https://sonarr.exemple.org/
  url: http://sonarr:8989
  secrets: {key: sonarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Sonarr, Réglages, Général), envoyée dans l'en-tête `X-Api-Key` |

Il n'existe pas de clé en lecture seule : nehome n'émet que des GET (API v3).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `queue`, `missing`, `health`, `library` | File d'attente, Manquants, Santé, Séries | tuile |
| `cutoff`, `version` | Cutoff non atteint, Version | détail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | tables et jauges : téléchargements en cours, calendrier sur 7 jours, derniers imports, espace disque, tâches planifiées | détail |

## Rythme

File, manquants et imports toutes les 30 s ; calendrier, bibliothèque (liste complète, plusieurs Mo) et disques toutes les 5 min. Chaque requête est indépendante : une qui échoue n'efface pas les autres.
