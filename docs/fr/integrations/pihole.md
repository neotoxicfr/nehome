# Pi-hole

Requêtes DNS, part bloquée, état du blocage, taille de la liste de blocage, clients et domaines les plus actifs.

## Configuration

```yaml
- id: pihole
  type: pihole
  title: Pi-hole
  section: Infrastructure
  href: https://pihole.exemple.org/
  url: http://192.0.2.53
  secrets: {pass: pihole_pass}
  # Pi-hole v5 :
  # options: {api: "5"}
  # secrets: {token: pihole_token}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `pass` | v6 : mot de passe de l'interface ou, mieux, mot de passe d'application |
| `token` | v5 (`options.api: "5"`) : jeton d'API (Réglages > API > Afficher le jeton) |

Pi-hole n'a pas de rôle en lecture seule ; nehome ne fait que lire. En v6, un mot de passe d'application dédié est recommandé. La session est rouverte si elle expire.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `queries, blocked, blocking, gravity` | requêtes, part bloquée (jauge), état du blocage, domaines bloqués | tuile |
| `clients, cached, forwarded, unique, version, topclients, topdomains, topblocked` | clients actifs, réponses en cache, requêtes transmises, domaines uniques, version, tables de tops | détail |

## Rythme

Toutes les 30 s.

## Limites

En v5 le jeton passe dans l'URL (imposé par api.php) : il est retiré de tous les messages d'erreur, mais peut apparaître dans les journaux du serveur Pi-hole. Version v5 : pas de nombre de clients actifs, seulement les clients uniques. Le mode d'authentification se choisit par `options.api`, il n'est pas détecté.
