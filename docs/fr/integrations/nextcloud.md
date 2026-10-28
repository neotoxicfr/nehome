# Nextcloud

Utilisateurs actifs, fichiers, espace libre, mises à jour disponibles, et l'état de PHP, de la base et des caches.

## Configuration

```yaml
- id: nextcloud
  type: nextcloud
  title: Nextcloud
  section: Applications
  url: https://nextcloud.exemple.org
  options: {user: admin}
  secrets: {pass: nextcloud_app_password}
```

L'application « Informations du serveur » (`serverinfo`) doit être active.

## Options

| Option | Rôle | Défaut |
|---|---|---|
| `user` | compte administrateur (identifiant, pas un secret) | `admin` |
| `skipApps` | `"true"` désactive le contrôle horaire des mises à jour (champ `updates` et `app_updates` masqués) | `false` |

## Secrets

| Rôle | Contenu |
|---|---|
| `pass` | mot de passe d'application du compte (Paramètres, Sécurité), en authentification basique |

L'API serverinfo exige un compte administrateur. Utilisez un mot de passe d'application, révocable, plutôt que le mot de passe du compte.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `active`, `files`, `free`, `updates` | Utilisateurs actifs (5 min), Fichiers, Espace libre, Mises à jour | tuile |
| `active_long`, `users`, `shares`, `version`, `php`, `db`, `caches`, `fpm`, `load`, `storages`, `appdata` | utilisateurs actifs à long terme, comptes, partages, version, PHP, base de données, caches, PHP-FPM, charge CPU, stockages, données des applis | détail |
| `opcache`, `apcu`, `ram` | jauges OPcache, APCu, mémoire vue par PHP | détail |
| `app_updates` | Applis à mettre à jour (table) | détail |

## Rythme

Toutes les minutes (appel qui saute les applis et les mises à jour, environ 0,1 s) ; le contrôle des mises à jour coûte environ 2 s et n'a lieu qu'une fois par heure.
