# Healthchecks

Contrôles de tâches planifiées (healthchecks.io ou auto-hébergé), pannes en tête. Relevé toutes les 30 s.

## Configuration

- Secret `key` : clé API **en lecture seule** (Paramètres du projet > Clés API).
- Options : aucune. `url` = `https://healthchecks.io` pour le service hébergé.

```yaml
- id: healthchecks
  type: healthchecks
  title: Healthchecks
  url: https://healthchecks.io
  secrets: {key: hc_readonly_key}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| state | État global | tuile |
| down | En panne | tuile |
| up | En ligne | tuile |
| grace | Délai de grâce | détail |
| paused | En pause | détail |
| checks | Checks, pannes d'abord (15 max) | détail |

## Limites

- Une clé API couvre un seul projet.
- Non éprouvé sur une vraie instance.
