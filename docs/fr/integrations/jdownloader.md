# JDownloader

Vitesse, état, paquets en cours, reste à télécharger et collecteur de liens, via le relais MyJDownloader.

## Configuration

```yaml
- id: jdownloader
  type: jdownloader
  title: JDownloader
  section: Téléchargement
  options: {device: JDownloader@serveur}
  secrets: {user: myjd_email, pass: myjd_password}
```

`url` est facultatif (défaut `https://api.jdownloader.org`).

## Options

| Option | Rôle |
|---|---|
| `device` | nom de l'appareil JDownloader tel qu'affiché dans MyJDownloader, **obligatoire** (s'il est introuvable, l'erreur liste les appareils connectés) |

## Secrets

| Rôle | Contenu |
|---|---|
| `user` | courriel du compte MyJDownloader |
| `pass` | son mot de passe |

Il n'existe pas de clé restreinte : ce sont les identifiants du compte. Gardez-les dans des secrets Docker.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `speed`, `state`, `active`, `remaining`, `grabber` | Vitesse, État, Paquets en cours, Reste à télécharger, Collecteur de liens | tuile |
| `total` | Liste de téléchargement | détail |
| `packages` | Paquets non terminés (table) | détail |

## Rythme et limites

Toutes les 30 s. Le protocole MyJDownloader impose des POST chiffrés (AES-128-CBC, signatures HMAC-SHA256) ; nehome n'envoie que des requêtes de lecture (`query*`, `get*`). Une seule session est gardée tant qu'elle vit, car MyJDownloader limite les connexions. L'URL, les jetons et la signature n'apparaissent jamais dans les erreurs.
