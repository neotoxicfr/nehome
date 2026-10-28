# Jackett

Nombre d'indexeurs configurés et de ceux en erreur, avec la dernière erreur connue de chacun.

## Configuration

```yaml
- id: jackett
  type: jackett
  title: Jackett
  section: Téléchargement
  url: http://jackett:9117
  secrets: {pass: jackett_pass}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `pass` | mot de passe d'administration de Jackett (connexion par cookie) |

Jackett n'a qu'un compte administrateur : le mot de passe donne tous les droits, nehome ne lit que la liste des indexeurs.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `errors`, `count` | Indexeurs en erreur, Indexeurs | tuile |
| `list` | Indexeurs configurés (table) | détail |

## Rythme et limites

Toutes les minutes. Le sondage ne teste aucun indexeur et ne lance aucune recherche (chacun interrogerait un tracker) : seule la dernière erreur déjà connue de Jackett est affichée. Les actions « Retester les indexeurs en erreur » et « Tester un indexeur » appellent `POST /api/v2.0/indexers/{id}/test`, un indexeur à la fois.
