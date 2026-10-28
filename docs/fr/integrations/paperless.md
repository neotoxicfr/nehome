# Paperless-ngx

Nombre de documents, boîte de réception, tâches de consommation et stockage. Relevé toutes les minutes.

## Configuration

- Secret `token` : jeton d'API de l'utilisateur (Mon profil > Jeton d'API). Un compte en lecture seule suffit pour les statistiques et les tâches ; `/api/status/` (version, stockage) demande un administrateur.
- Options : aucune.

```yaml
- id: paperless
  type: paperless
  title: Paperless
  url: http://paperless:8000
  secrets: {token: paperless_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| documents | Documents | tuile |
| inbox | Boîte de réception (orange si non vide) | tuile |
| tasks | Tâches en cours, échecs sur 24 h | tuile |
| storage | Stockage | détail |
| classes | Étiquettes / correspondants / types | détail |
| chars | Caractères indexés | détail |
| types | Types de fichiers | détail |
| task_list | Dernières tâches | détail |
| version | Version | détail |

## Limites

- `/api/tasks/` : tableau ou page `{results}` selon la version, les deux sont lus.
- Non éprouvé sur une vraie instance.
