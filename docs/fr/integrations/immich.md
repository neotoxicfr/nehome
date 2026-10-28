# Immich

Photos et vidéos, espace utilisé, disque du serveur, quotas par utilisateur, files de tâches et version.

## Configuration

```yaml
- id: immich
  type: immich
  title: Immich
  section: Médias
  href: https://photos.exemple.org/
  url: http://immich-server:2283
  secrets: {key: immich_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API (Réglages du compte, Clés d'API), en-tête `x-api-key` |

Droits minimaux à cocher sur la clé : `server.statistics`, `server.storage`, `server.about` (ou `server.versionCheck`) et `job.read`. Les statistiques serveur exigent un compte administrateur.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `photos`, `videos`, `usage`, `disk` | Photos, Vidéos, Espace utilisé, Disque du serveur (jauge) | tuile |
| `users`, `jobs`, `jobs_table`, `version` | table par utilisateur, tâches en cours, files de tâches non vides, version (alerte si plus récente disponible) | détail |

## Rythme

Files de tâches toutes les 30 s ; statistiques, stockage et version toutes les 5 min.

## Limites

Les files viennent de `/api/jobs`, que les versions récentes d'Immich peuvent déprécier : si elle disparaît, seuls `jobs` et `jobs_table` restent vides. Non testé sur un vrai Immich : réponses simulées d'après son OpenAPI.
