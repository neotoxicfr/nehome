# Navidrome

Lectures en cours, nombre de morceaux, albums et artistes, état du scan, version. Via l'API Subsonic.

## Configuration

```yaml
- id: navidrome
  type: navidrome
  title: Navidrome
  section: Médias
  href: https://musique.exemple.org/
  url: http://navidrome:4533
  options: {user: alice}          # facultatif : sinon secrets.user
  secrets: {pass: navidrome_pass}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `pass` | mot de passe du compte |
| `user` | identifiant, seulement si l'option `user` est absente |

Un compte ordinaire suffit. L'authentification est celle de la spec Subsonic : jeton salé `md5(mot de passe + sel)`, le mot de passe ne part jamais en clair. Le jeton et le sel voyagent dans l'URL (imposé par la spec) ; nehome les retire des erreurs affichées, mais ils peuvent apparaître dans les journaux d'un proxy devant Navidrome.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `playing`, `tracks`, `albums`, `artists` | Lectures en cours, Morceaux, Albums, Artistes | tuile |
| `scan`, `now_playing`, `version` | état du scan, table des lectures, version | détail |

## Rythme

Lectures en cours toutes les 10 s ; scan chaque minute ; artistes et albums (`getArtists`) toutes les 5 min.

## Limites

« Albums » est la somme des albums des artistes d'album (les compilations comptent chez leur artiste). « Morceaux » est le nombre de fichiers du dernier scan. Non testé sur un vrai Navidrome : réponses simulées d'après la spec Subsonic.
