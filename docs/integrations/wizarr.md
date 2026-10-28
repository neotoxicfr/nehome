# Wizarr

> Version française : [../fr/integrations/wizarr.md](../fr/integrations/wizarr.md)


Users, pending or expired invitations, 7-day activity, accounts close to expiry.

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

| Role | Content |
|---|---|
| `key` | API key created in Wizarr (API Keys page), sent as `X-API-Key` |

## Fields

| ID | Label | Default |
|---|---|---|
| `users`, `pending` | « Utilisateurs » (users), « Invitations en attente » (pending invitations) | tile |
| `expired`, `invites`, `activity`, `version` | « Invitations expirées » (expired invitations), « Invitations » (invitations), « Activité (7 j) » (activity, 7 d), « Version » | detail |
| `user_list`, `invite_list` | tables: accounts (nearest expiry), recent invitations | detail |

## Polling and limits

Every minute. The schema comes from the container's Swagger documentation and has not been tested with a real key. An invitation code grants access to the server: it is never displayed.
