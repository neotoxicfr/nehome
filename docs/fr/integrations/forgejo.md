# Forgejo

Forgejo expose la même API que Gitea : le type `forgejo` utilise exactement la même source. Voir [gitea.md](gitea.md) pour les droits du jeton et la liste des champs.

## Configuration

- Secret `token` : jeton d'accès personnel, mêmes droits que pour Gitea.
- Options : aucune.

```yaml
- id: forgejo
  type: forgejo
  title: Forgejo
  url: http://forgejo:3000
  secrets: {token: forgejo_token}
```

## Champs

Identiques à Gitea (repos, notifications, users, orgs, version, recent).

## Limites

Mêmes limites que Gitea ; non éprouvé sur une vraie instance Forgejo.
