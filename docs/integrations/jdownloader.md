# JDownloader

> Version française : [../fr/integrations/jdownloader.md](../fr/integrations/jdownloader.md)

Speed, state, active packages, remaining data and link grabber, through the MyJDownloader relay.

## Configuration

```yaml
- id: jdownloader
  type: jdownloader
  title: JDownloader
  section: Téléchargement
  options: {device: JDownloader@serveur}
  secrets: {user: myjd_email, pass: myjd_password}
```

`url` is optional (default `https://api.jdownloader.org`).

## Options

| Option | Purpose |
|---|---|
| `device` | name of the JDownloader device as shown in MyJDownloader, **required** (if it is not found, the error lists the connected devices) |

## Secrets

| Role | Content |
|---|---|
| `user` | email of the MyJDownloader account |
| `pass` | its password |

There is no restricted key: these are the account credentials. Keep them in Docker secrets.

## Fields

| ID | Label | Default |
|---|---|---|
| `speed`, `state`, `active`, `remaining`, `grabber` | « Vitesse » (speed), « État » (state), « Paquets en cours » (active packages), « Reste à télécharger » (remaining to download), « Collecteur de liens » (link grabber) | tile |
| `total` | « Liste de téléchargement » (download list) | detail |
| `packages` | « Paquets non terminés » (unfinished packages; table) | detail |

## Refresh rate and limits

Every 30 s. The MyJDownloader protocol requires encrypted POSTs (AES-128-CBC, HMAC-SHA256 signatures); nehome only sends read requests (`query*`, `get*`). A single session is kept for as long as it lives, because MyJDownloader limits connections. The URL, tokens and signature never appear in errors.
