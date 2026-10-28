# Cloudflare

Requêtes, menaces, taux de cache, bande passante sur 24 h, et derniers événements du pare-feu d'une zone.

## Configuration

```yaml
- id: cloudflare
  type: cloudflare
  title: Cloudflare
  section: Réseau
  options: {zone_name: exemple.org}
  secrets: {token: cloudflare_token}
```

## Options

| Option | Rôle |
|---|---|
| `zone_name` | nom de la zone, résolu en identifiant par l'API. **À renseigner** : la valeur par défaut codée dans le programme est un domaine de l'auteur. |
| `zone` | identifiant de zone ; évite la résolution (et le droit « Zone Lire ») |

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | jeton d'API Cloudflare |

Droits minimaux du jeton, limité à la zone : Analytics Lire et Pare-feu Lire (plus Zone Lire si `zone` n'est pas donné). Le jeton ne voyage que dans un en-tête.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `requests`, `threats`, `cache`, `bandwidth` | Requêtes, Menaces, Part servie par le cache, Bande passante | tuile |
| `curve` | Requêtes par heure | détail |
| `firewall` | Derniers événements pare-feu | détail |

## Rythme et limites

Toutes les 5 min (requêtes GraphQL lourdes, quota limité). La requête GraphQL est un POST, mais en lecture seule.
