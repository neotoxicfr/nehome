# Security Policy

## Supported versions

Only the latest release is supported; fixes ship in the next release.

## Reporting a vulnerability

Open a [private security advisory](https://github.com/neotoxicfr/nehome/security/advisories/new)
rather than a public issue. Include the version, the configuration involved and a way to reproduce.

## Security model

- **Authentication is delegated** to a reverse proxy with forward authentication (Authelia,
  Authentik, oauth2-proxy…). nehome trusts the user header (`Remote-User` by default) only on
  connections coming from `auth.trusted_proxies`, and refuses to start without that list unless
  `auth.disabled` is set explicitly.
- **Editing links** (type, address, keys) from the UI is limited to `auth.admins`. From the UI a
  role can only keep the secret already bound to it; binding another existing secret is a
  `config.yaml` edit. Changing the scheme, host or port of a link, or accepting a self-signed
  certificate, requires typing every key again: an existing key is never sent to a new destination.
- **Secrets never reach the browser.** They are read from Docker secrets or from
  `data_dir/secrets` (mode 0600, written by the UI, never displayed again). Errors shown on tiles
  are stripped of query strings, and HTTP clients refuse redirects to another host, so a key sent
  in a custom header cannot be forwarded.
- **Integrations are read-only**: GET requests, plus POST only where a login or a read-only RPC
  call requires it.
- **Strict Content-Security-Policy** (`default-src 'none'; script-src 'self'`, no inline script or
  style). Live fragments are rendered by `html/template`. Forms are rejected unless they come from
  the same origin.
- **Icons** are PNG files from the dashboard-icons library, fetched only for names listed in its
  index, checked to be PNG and served from nehome's own origin.

## Container hardening

The image is `distroless/static` (no shell, no package manager) running as a non-root user. The
example compose adds `read_only: true`, `cap_drop: [ALL]` and `no-new-privileges:true`. Treat the
data volume as a secret: it holds keys typed in the UI.
