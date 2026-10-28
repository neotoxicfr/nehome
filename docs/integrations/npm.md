# Nginx Proxy Manager

> Version française : [../fr/integrations/npm.md](../fr/integrations/npm.md)

Active or disabled proxy hosts and certificates close to expiry. Polled every 5 min.

## Configuration

- Secrets `user` (email) and `pass`: a dedicated account with the "view" right on proxy hosts, redirections, streams, 404 hosts and certificates. The token is obtained with `POST /api/tokens`, then renewed on 401.
- Options: none. `url` = the management interface (port 81).

```yaml
- id: npm
  type: npm
  title: Proxy Manager
  url: http://npm:81
  secrets: {user: npm_user, pass: npm_pass}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| proxy | « Hôtes proxy » (proxy hosts; active, disabled) | tile |
| certs | « Certificats » (certificates) | tile |
| expiry | « Prochaine expiration » (next expiry; orange < 21 d, red < 7 d) | tile |
| other | « Redirections / flux / hôtes 404 » (redirections / streams / 404 hosts) | detail |
| cert_list | certificates by expiry date | detail |
| host_list | disabled hosts | detail |
| version | « Version » (version) | detail |

## Limits

- Only GETs are sent, apart from the login.
- Not proven on a real instance.
