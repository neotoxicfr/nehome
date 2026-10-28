# TrueNAS

Tuile pour TrueNAS SCALE : CPU, mémoire, réseau en temps réel, pools, alertes, snapshots et réplications.

## Configuration

```yaml
- id: truenas
  type: truenas
  title: TrueNAS
  section: Système
  href: https://truenas.exemple.org/
  url: https://192.0.2.10:444
  insecure_tls: true        # certificat auto-signé
  secrets: {key: truenas_key}
```

`url` est l'adresse de l'interface web ; nehome s'y connecte en websocket (`/api/current`, JSON-RPC 2.0). Pas d'option.

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API TrueNAS (Identifiants, Clés API) |

Droits minimaux : un compte en lecture seule suffit (rôle « Readonly Admin »). nehome n'appelle que des méthodes de lecture.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `cpu` | CPU | tuile |
| `ram` | Mémoire (hors ARC) | tuile |
| `pools` | Pools | tuile |
| `net_rx` | Réseau réception | tuile |
| `alerts_n` | Nombre d'alertes | tuile |
| `net_tx`, `cpu_hist`, `cputemp`, `arc`, `disk_io`, `load`, `uptime`, `version` | envoi réseau, CPU sur 2 min, température CPU, cache ARC, E/S disques, charge, durée de fonctionnement, version | détail |
| `alerts`, `scrub`, `temps`, `snapshots`, `replication` | tables : alertes, état des pools et dernier scrub, températures des disques, snapshots périodiques, réplications | détail |

## Rythme et limites

Temps réel par l'abonnement `reporting.realtime` (environ 2 s) ; pools, alertes, snapshots et réplications relus toutes les minutes. La connexion se rétablit seule avec un délai croissant (5 s à 1 min). Un scrub de plus de 40 jours passe en alerte.
