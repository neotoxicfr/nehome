# Gotify

> Version française : [../fr/integrations/gotify.md](../fr/integrations/gotify.md)

Recent messages and applications of a Gotify notification server. Polled every 30 s.

## Configuration

- Secret `token`: **client** token (Clients > Create), not an application's: only a client can read messages.
- Options: none.

```yaml
- id: gotify
  type: gotify
  title: Gotify
  url: http://gotify
  secrets: {token: gotify_client_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| day | « Messages (24 h) » | tile |
| last | Last message (age, application) | tile |
| apps | « Applications » | tile |
| high | High priority (>= 8) over 24 h | detail |
| messages | Recent messages (application, title, priority) | detail |
| app_list | Declared applications | detail |
| version | Version | detail |

## Limits

- Message bodies are never displayed (they may be sensitive), nor are application tokens.
- The 24 h count covers the last 100 messages ("100+" beyond that).
- Not proven on a real instance.
