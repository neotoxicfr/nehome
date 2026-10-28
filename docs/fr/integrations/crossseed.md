# cross-seed

Indique seulement si cross-seed est joignable.

## Configuration

```yaml
- id: crossseed
  type: crossseed
  title: cross-seed
  section: Téléchargement
  url: http://cross-seed:2468
```

## Secrets

| Rôle | Usage |
|---|---|
| `key` | facultatif : `apiKey` de cross-seed (en-tête `X-Api-Key`), active les actions « Lancer le RSS » et « Lancer une recherche » (`POST /api/job`) |

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `state` | État | tuile |

## Limites

cross-seed n'expose en lecture que `/api/ping` (sans clé). L'historique des recherches et des injections n'est que dans sa base SQLite, sans API. `/api/job` n'est appelé que par les actions (avec la clé) ; `/api/webhook` et `/api/announce` jamais. Sondage toutes les 30 s.
