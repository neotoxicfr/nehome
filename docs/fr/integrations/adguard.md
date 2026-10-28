# AdGuard Home

Requêtes DNS sur 24 h, part bloquée, état de la protection, clients et domaines les plus actifs, listes de filtrage. Une tuile par instance.

## Configuration

```yaml
- id: adguard
  type: adguard
  title: AdGuard
  section: Réseau
  href: https://adguard.exemple.org/
  url: http://192.0.2.53:3000
  secrets: {user: adguard_user, pass: adguard_pass}
```

Pour plusieurs instances, répétez la tuile avec un `id`, une `url` et des secrets différents.

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte de l'interface web (authentification basique sur `/control`) |

AdGuard Home n'a pas de rôle en lecture seule : le compte voit tout, nehome ne fait que des GET.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `queries`, `blocked`, `curve`, `protection` | Requêtes (24 h), Bloquées (jauge), Requêtes par heure, Protection | tuile |
| `avg`, `upstream`, `version`, `rules` | temps moyen, serveur amont, version, règles de filtrage | détail |
| `clients`, `domains`, `blockedtop`, `lists` | tables : clients actifs, domaines interrogés, domaines bloqués, listes de filtrage | détail |

## Rythme

Statistiques toutes les minutes ; listes de filtrage toutes les 10 min.
