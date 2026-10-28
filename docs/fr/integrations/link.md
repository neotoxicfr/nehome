# Lien

Un simple lien vers une application, avec au choix l'état de son conteneur Docker et/ou une sonde HTTP.

## Configuration

```yaml
- id: wiki
  type: link
  title: Wiki
  section: Applications
  href: https://wiki.exemple.org/
  insecure_tls: false
  options:
    container: wiki                         # facultatif
    docker_url: http://socket-proxy:2375    # facultatif
    health_url: http://wiki:3000/health     # facultatif
    post_url: http://wiki:3000/hook         # facultatif
    post_label: Reconstruire                # facultatif
```

Sans option, la tuile n'est qu'un lien.

## Options

| Option | Rôle |
|---|---|
| `container` | nom du conteneur dont on lit l'état (via l'API Docker, lecture seule) |
| `docker_url` | proxy de socket (défaut `http://socket-proxy:2375`) |
| `health_url` | URL `http(s)` sondée par un GET ; invalide, la tuile affiche une erreur |
| `post_url` | URL `http(s)` appelée par un POST sans corps quand on clique le bouton de la tuile (réveil d'un PC, webhook…) |
| `post_label` | libellé de ce bouton (défaut « Déclencher ») |

## Secrets

Aucun. `insecure_tls` s'applique à la sonde.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `state`, `health`, `uptime` | État, Santé HTTP, En marche depuis | tuile |
| `image`, `restarts` | Image, Redémarrages | détail |

## Rythme et limites

Toutes les 30 s. Sonde : erreur réseau ou 5xx en critique, 4xx en avertissement (l'application répond mais refuse la sonde), latence supérieure à 1,5 s en avertissement. Une redirection vers un autre hôte (page de connexion d'un proxy d'authentification) n'est pas suivie et prouve que l'application répond.
