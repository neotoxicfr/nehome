# Maintainerr

> Version française : [../fr/integrations/maintainerr.md](../fr/integrations/maintainerr.md)

Media that the rules are going to delete: count, space to free, next deletion.

## Configuration

```yaml
- id: maintainerr
  type: maintainerr
  title: Maintainerr
  section: Médias
  url: http://maintainerr:6246
```

## Secrets

None: the Maintainerr API has no authentication. Do not expose it outside a private network.

## Fields

| ID | Label | Default |
|---|---|---|
| `items`, `size`, `next` | « Éléments à supprimer » (items to delete), « Espace à libérer » (space to free), « Prochaine suppression » (next deletion) | tile |
| `freed`, `version` | « Déjà supprimé » (already deleted), « Version » (version) | detail |
| `collections`, `upcoming` | tables: collections, next items to be deleted | detail |

## Refresh rate and limits

Every 5 min (the rules run once a day). An item's deletion date is computed: date it entered the collection plus `deleteAfterDays`. Only the first 50 entries of each collection are read.
