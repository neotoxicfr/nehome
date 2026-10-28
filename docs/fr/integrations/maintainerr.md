# Maintainerr

Médias que les règles vont supprimer : nombre, espace à libérer, prochaine suppression.

## Configuration

```yaml
- id: maintainerr
  type: maintainerr
  title: Maintainerr
  section: Médias
  url: http://maintainerr:6246
```

## Secrets

Aucun : l'API de Maintainerr n'a pas d'authentification. Ne l'exposez pas hors d'un réseau privé.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `items`, `size`, `next` | Éléments à supprimer, Espace à libérer, Prochaine suppression | tuile |
| `freed`, `version` | Déjà supprimé, Version | détail |
| `collections`, `upcoming` | tables : collections, prochains éléments supprimés | détail |

## Rythme et limites

Toutes les 5 min (les règles tournent une fois par jour). La date de suppression d'un élément est calculée : date d'entrée dans la collection plus `deleteAfterDays`. Seules les 50 premières entrées de chaque collection sont lues.
