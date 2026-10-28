# Forgejo

> Version française : [../fr/integrations/forgejo.md](../fr/integrations/forgejo.md)

Forgejo exposes the same API as Gitea: the `forgejo` type uses exactly the same source. See [gitea.md](gitea.md) for token permissions and the field list.

## Configuration

- Secret `token`: personal access token, same permissions as for Gitea.
- Options: none.

```yaml
- id: forgejo
  type: forgejo
  title: Forgejo
  url: http://forgejo:3000
  secrets: {token: forgejo_token}
```

## Fields

Identical to Gitea (repos, notifications, users, orgs, version, recent).

## Limits

Same limits as Gitea; not proven on a real Forgejo instance.
