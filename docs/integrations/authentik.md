# authentik

> Version française : [../fr/integrations/authentik.md](../fr/integrations/authentik.md)

Active users, applications, version, and failed logins over the last day. Polled every minute.

## Configuration

- Secret `token`: API token (Directory > Tokens and App passwords). A dedicated user with permission to view users, applications and events.
- Options: none.

```yaml
- id: authentik
  type: authentik
  title: authentik
  url: https://auth.example.com
  secrets: {token: authentik_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| users | « Utilisateurs actifs » (active users) | tile |
| failed | « Connexions échouées (24 h) » (failed logins): orange > 0, red >= 10 | tile |
| apps | « Applications » | tile |
| version | « Version » (orange if an update is available) | detail |
| failed_list | Latest failed logins: when, IP, stage | detail |

## Limits

- Credentials entered during failures are never displayed.
- The 24 h count covers the last 100 events ("100+" beyond that).
- Not proven on a real instance.
