# Docker

État des conteneurs : en marche, sains, en problème, ressources, images, et résumé d'une mise à jour nocturne.

## Configuration

```yaml
- id: docker
  type: docker
  title: Docker
  section: Système
  url: http://socket-proxy:2375
  options: {update_log: /logs/update.log}
```

`url` : API Docker en HTTP, à travers un proxy de socket en lecture seule (par exemple `tecnativa/docker-socket-proxy` avec `CONTAINERS=1`, `IMAGES=1`). Défaut : `http://socket-proxy:2375`. **Ne montez pas `/var/run/docker.sock` dans nehome.**

## Options

| Option | Rôle |
|---|---|
| `update_log` | chemin (dans le conteneur nehome) d'un journal de mise à jour, monté en lecture seule. Sans lui, les champs de mise à jour sont masqués. |

Le journal est celui d'un script `update.sh` propre à l'installation d'origine ; le format attendu n'est pas documenté ailleurs, laissez l'option vide si vous n'en avez pas.

## Secrets

Aucun.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `running`, `healthy`, `trouble` | conteneurs en marche, sains, problèmes | tuile |
| `update` | mise à jour nocturne | tuile |
| `total`, `unhealthy`, `stopped`, `created`, `images`, `usage` | total, unhealthy, arrêtés, jamais démarrés, images, ressources cumulées | détail |
| `problems`, `cpu`, `ram`, `update_detail` | tables : conteneurs à surveiller, CPU et mémoire par conteneur (15 premiers), détail de la mise à jour | détail |

## Rythme et limites

Liste et inspection toutes les minutes (les stats durent environ 1 s par conteneur, 8 en parallèle) ; images toutes les 5 min. Un proxy qui refuse `/images` n'efface pas la tuile. Lecture seule : uniquement des GET.
