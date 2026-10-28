# Netdata

Alarmes actives, processeur et mémoire d'un agent Netdata.

## Configuration

```yaml
- id: netdata
  type: netdata
  title: Netdata
  section: Infrastructure
  href: https://netdata.exemple.org/
  url: http://192.0.2.10:19999
  secrets: {token: netdata_token}   # facultatif
```

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | facultatif : jeton Bearer, utile seulement si un proxy protège l'agent |

Aucune ; l'agent local est en lecture seule sur ces routes.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `alarms, cpu, ram` | alarmes actives (avertissements et critiques), processeur, mémoire | tuile |
| `curve, host, version, children, alarmlist` | historique du processeur, machine, version, nombre de machines suivies, table des alarmes | détail |

## Rythme

Toutes les 10 s.

## Limites

Utilise l'API v1, aussi servie par les agents v2. La mémoire se calcule sur le graphique system.ram (libre, utilisé, cache, tampons).
