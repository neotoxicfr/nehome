# Nextcloud

> Version française : [../fr/integrations/nextcloud.md](../fr/integrations/nextcloud.md)

Active users, files, free space, available updates, and the state of PHP, the database and caches.

## Configuration

```yaml
- id: nextcloud
  type: nextcloud
  title: Nextcloud
  section: Applications
  url: https://nextcloud.example.org
  options: {user: admin}
  secrets: {pass: nextcloud_app_password}
```

The "Informations du serveur" (Server info) app (`serverinfo`) must be enabled.

## Options

| Option | Purpose | Default |
|---|---|---|
| `user` | administrator account (a username, not a secret) | `admin` |
| `skipApps` | `"true"` disables the hourly update check (`updates` and `app_updates` fields hidden) | `false` |

## Secrets

| Role | Content |
|---|---|
| `pass` | app password of the account (Settings, Security), used with basic authentication |

The serverinfo API requires an administrator account. Use an app password, which can be revoked, rather than the account password.

## Fields

| ID | Label | Default |
|---|---|---|
| `active`, `files`, `free`, `updates` | « Utilisateurs actifs » (active users, 5 min), « Fichiers » (files), « Espace libre » (free space), « Mises à jour » (updates) | tile |
| `active_long`, `users`, `shares`, `version`, `php`, `db`, `caches`, `fpm`, `load`, `storages`, `appdata` | long-term active users, accounts, shares, version, PHP, database, caches, PHP-FPM, CPU load, storages, app data | detail |
| `opcache`, `apcu`, `ram` | gauges: OPcache, APCu, memory as seen by PHP | detail |
| `app_updates` | « Applis à mettre à jour » (apps to update; table) | detail |

## Refresh rate

Every minute (a call that skips apps and updates, about 0.1 s); the update check costs about 2 s and runs only once an hour.
