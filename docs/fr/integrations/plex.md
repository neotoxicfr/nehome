# Plex

Lectures en cours, débit sortant, nombre de films et de séries, ajouts récents et bibliothèques.

## Configuration

```yaml
- id: plex
  type: plex
  title: Plex
  section: Médias
  href: https://plex.exemple.org/
  url: http://plex:32400
  secrets: {token: plex_token}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | jeton Plex (`X-Plex-Token`), envoyé en en-tête |

Plex ne propose pas de jeton restreint : celui du propriétaire du serveur donne tous les droits. nehome n'émet que des GET.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `streams`, `bandwidth`, `movies`, `shows` | Lectures en cours, Débit sortant, Films, Séries et animés | tuile |
| `activities`, `version` | Activités du serveur, Version | détail |
| `sessions`, `recent`, `libraries` | tables : détail des lectures (mode, transcodage en avertissement), ajouts récents, bibliothèques | détail |

## Rythme

Lectures toutes les 5 s ; ajouts, activités et version toutes les minutes ; comptes par bibliothèque toutes les 5 min. Un échec d'une requête secondaire n'efface rien.
