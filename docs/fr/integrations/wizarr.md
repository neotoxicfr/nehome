# Wizarr

Utilisateurs, invitations en attente ou expirées, activité sur 7 jours, comptes proches de l'expiration.

## Configuration

```yaml
- id: wizarr
  type: wizarr
  title: Wizarr
  section: Médias
  url: http://wizarr:5690
  secrets: {key: wizarr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API créée dans Wizarr (page Clés API), envoyée en `X-API-Key` |

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `users`, `pending` | Utilisateurs, Invitations en attente | tuile |
| `expired`, `invites`, `activity`, `version` | Invitations expirées, Invitations, Activité (7 j), Version | détail |
| `user_list`, `invite_list` | tables : comptes (expiration la plus proche), invitations récentes | détail |

## Rythme et limites

Toutes les minutes. Le schéma est tiré de la documentation Swagger du conteneur et n'a pas été éprouvé avec une vraie clé. Le code d'une invitation donne accès au serveur : il n'est jamais affiché.
