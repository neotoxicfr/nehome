# Readarr

File d'attente, éléments manquants, santé, bibliothèque de livres, calendrier des sorties, derniers imports et espace disque.

## Configuration

```yaml
- id: readarr
  type: readarr
  title: Readarr
  section: Médias
  href: https://readarr.exemple.org/
  url: http://readarr:8787
  secrets: {key: readarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Readarr, Réglages, Général), envoyée dans l'en-tête `X-Api-Key` |

Pas de clé en lecture seule : nehome n'émet que des GET (API v1).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `queue`, `missing`, `health`, `library` | File d'attente, Manquants, Santé, Auteurs | tuile |
| `cutoff`, `version` | Cutoff non atteint, Version | détail |
| `downloads`, `calendar`, `history`, `disks`, `tasks` | téléchargements en cours, calendrier sur 7 jours, derniers imports, espace disque, tâches planifiées | détail |

## Rythme

File, manquants et imports toutes les 30 s ; calendrier, bibliothèque et disques toutes les 5 min.

## Limites

Readarr est archivé en amont : l'API v1 reste celle décrite ici, sans garantie d'évolution. Les derniers imports sont les fichiers de livres importés. Non testé sur une vraie instance : réponses simulées d'après le code source.
