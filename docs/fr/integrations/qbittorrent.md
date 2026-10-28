# qBittorrent

Débits, torrents en cours et en partage, ratio, espace libre, catégories et étiquettes.

## Configuration

```yaml
- id: qbittorrent
  type: qbittorrent
  title: qBittorrent
  section: Téléchargement
  href: https://qbittorrent.exemple.org/
  url: http://qbittorrent:8080
  secrets: {user: qbit_user, pass: qbit_pass}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user` | identifiant de l'interface web |
| `pass` | mot de passe |

Facultatifs : sans `secrets.user`, nehome n'ouvre pas de session (qBittorrent peut exempter un sous-réseau de l'authentification). Si `user` est déclaré, `pass` l'est aussi. Un compte ordinaire de l'interface web suffit. nehome relance le login sur un 403.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `down`, `up` | Réception, Envoi (courbes) | tuile |
| `leech`, `seed` | En téléchargement, En partage | tuile |
| `ratio`, `conn`, `alt`, `limits`, `free`, `total`, `peers`, `errors`, `stalled` | ratio global, état de connexion, limite alternative, limites, espace libre, total depuis l'origine, pairs, en erreur, bloqués | détail |
| `dl`, `topup`, `cats`, `tags` | tables : téléchargements en cours, envois les plus actifs, par catégorie, par étiquette | détail |

## Rythme et limites

Sondage toutes les 2 s sur `sync/maindata` en mode incrémental : le premier appel ramène tous les torrents, les suivants seulement les changements. Lecture seule.
