# ntfy

> Version française : [../fr/integrations/ntfy.md](../fr/integrations/ntfy.md)

Messages received on a topic over the last 24 h: count, last message, high-priority messages.

## Configuration

```yaml
- id: ntfy
  type: ntfy
  title: ntfy
  section: Supervision
  url: https://ntfy.example.org
  options: {topic: alertes}
  secrets: {user: ntfy_user, pass: ntfy_pass}   # optional
```

## Options

| Option | Purpose | Default |
|---|---|---|
| `topic` | topic to read | `uptime` |

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | account with **read** access to the topic; optional if the topic is readable anonymously |

If `secrets.user` is declared, `pass` must be too. Give the account `read-only` access to the one topic concerned (`ntfy access`).

## Fields

| ID | Label | Default |
|---|---|---|
| `count`, `last`, `urgent` | « Messages (24 h) » (messages), « Dernier message » (last message), « Priorité haute (24 h) » (high priority) | tile |
| `messages` | messages from the last 24 h (table) | detail |

## Refresh rate and limits

Every 30 s, with `GET …/json?poll=1` (line-delimited JSON stream). A refusal from the server is shown with its HTTP code. nehome never publishes a message.
