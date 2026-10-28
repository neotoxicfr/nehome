# Gitea

> Version française : [../fr/integrations/gitea.md](../fr/integrations/gitea.md)

Repositories, users, organizations and notifications of a Gitea forge (`/api/v1` API). Polled every minute, read-only.

## Configuration

- Secret `token`: personal access token (Settings > Applications). Minimum permissions: `read:repository`, `read:notification`, and `read:admin` + `read:organization` if the account is an administrator (for users and organizations).
- Options: none. `url` = address of the instance.

```yaml
- id: gitea
  type: gitea
  title: Gitea
  url: http://gitea:3000
  secrets: {token: gitea_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| repos | Repositories | tile |
| notifications | Unread notifications | tile |
| users | Users | tile |
| orgs | Organizations | detail |
| version | Version | detail |
| recent | Recently modified repositories (10) | detail |

## Limits

- Users and organizations go through `/admin`: without an administrator token, these fields are hidden.
- Totals come from the `X-Total-Count` header.
- Responses reconstructed from the Swagger API; not proven on a real instance.
