# Gitea

Dépôts, utilisateurs, organisations et notifications d'une forge Gitea (API `/api/v1`). Relevé toutes les minutes, lecture seule.

## Configuration

- Secret `token` : jeton d'accès personnel (Paramètres > Applications). Droits minimaux : `read:repository`, `read:notification`, et `read:admin` + `read:organization` si le compte est administrateur (pour utilisateurs et organisations).
- Options : aucune. `url` = adresse de l'instance.

```yaml
- id: gitea
  type: gitea
  title: Gitea
  url: http://gitea:3000
  secrets: {token: gitea_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| repos | Dépôts | tuile |
| notifications | Notifications non lues | tuile |
| users | Utilisateurs | tuile |
| orgs | Organisations | détail |
| version | Version | détail |
| recent | Dépôts récemment modifiés (10) | détail |

## Limites

- Utilisateurs et organisations passent par `/admin` : sans jeton administrateur, ces champs sont masqués.
- Les totaux viennent de l'en-tête `X-Total-Count`.
- Réponses reconstituées d'après l'API Swagger ; non éprouvé sur une vraie instance.
