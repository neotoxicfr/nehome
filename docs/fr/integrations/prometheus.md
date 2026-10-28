# Prometheus

Lit des métriques Prometheus, de deux façons :

- **Serveur Prometheus** : chaque champ est une requête PromQL instantanée (`/api/v1/query`) ; la valeur est le premier résultat (scalaire, chaîne ou premier élément du vecteur).
- **Endpoint `/metrics`** (`scrape: true`) : nehome lit directement la page texte d'un exportateur ; chaque champ est un nom de série, avec un sélecteur d'étiquettes simple. Plusieurs séries correspondantes sont additionnées.

## Configuration

```yaml
- id: prom
  type: prometheus
  title: Prometheus
  url: http://prometheus:9090
  fields:
    - {id: up, label: Cibles en service, path: 'sum(up)', format: number}
    - {id: cpu, label: CPU, path: '100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100', format: percent, warn: 80, crit: 95}

- id: exporter
  type: prometheus
  title: Node exporter
  url: http://192.0.2.10:9100/metrics
  options: {scrape: true, interval: 30s}
  fields:
    - {id: free, label: Libre sur /, path: 'node_filesystem_avail_bytes{mountpoint="/"}', format: bytes}
    - {id: boot, label: Démarré, path: node_boot_time_seconds, format: date}
```

Formats, seuils, placement, `interval`, `header.<Nom>` : comme [customapi](customapi.md).

## Secrets (facultatifs)

`token` (Bearer), `key` (+ `options.key_header`), `user`/`pass` (basique) : utiles derrière un proxy authentifiant. Prometheus lui-même n'a pas de droits par clé ; nehome n'appelle que `/api/v1/query` en GET.

## Limites

- Requêtes instantanées seulement ; un résultat `matrix` (sous-requête `[1h:]`) est refusé. Pour compter ou agréger, utilisez `count()`, `sum()`… dans la requête.
- Mode scrape : sélecteur par égalité seule (`=`, pas de `!=` ni `=~`) ; noms de série UTF-8 entre guillemets non gérés ; format texte (Accept `text/plain`), pas de protobuf.
- Une requête invalide ou sans résultat s'affiche « — » avec le message de Prometheus ; si toutes échouent pour une raison réseau ou HTTP, la tuile passe en erreur. Les requêtes partent l'une après l'autre : gardez-en un nombre raisonnable.
