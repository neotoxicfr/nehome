# API JSON générique (customapi)

Une tuile pour n'importe quelle API qui répond en JSON : vous déclarez les champs à lire et leur format. Lecture seule (GET).

## Configuration

```yaml
- id: sauvegarde
  type: customapi
  title: Sauvegardes
  url: https://backup.exemple.org/api/status
  secrets: {token: backup_token}      # facultatif
  options:
    interval: 5m                      # défaut 60s, minimum 5s
    header.Accept-Language: fr        # en-têtes fixes : header.<Nom>
  fields:
    - {id: last, label: Dernière sauvegarde, path: jobs.0.finished_at, format: date, warn: 90000, crit: 180000}
    - {id: size, label: Taille, path: jobs.0.bytes, format: bytes}
    - {id: jobs, label: Tâches, path: jobs.#, format: number, placement: detail}
    - {id: disk, label: Disque, path: storage.used_pct, format: percent, warn: 80, crit: 90}
```

## Chemins

- Clés séparées par des points : `data.items.0.count` ; indice négatif depuis la fin : `items.-1.name`.
- `#` en fin de chemin : longueur d'un tableau ou d'un objet (`null` compte 0).
- Chemin vide : la racine (API qui répond un simple nombre).
- Les clés contenant un point ne sont pas accessibles.

## Formats

| Format | Rendu |
|---|---|
| `text` (défaut) | valeur brute ; booléen « oui »/« non » |
| `number` | nombre à la française, milliers groupés |
| `bytes`, `rate` | « 1,5 Go », « 2,5 ko/s » |
| `percent` | jauge 0-100 |
| `duration` | secondes → « 1 j 2 h » |
| `date` | RFC 3339 ou époque Unix (s, ms, µs, ns reconnus) → « 27/10/2026 08:00 », « il y a 3 h » |

Les nombres en chaîne (`"91.2"`) sont acceptés. `warn`/`crit` colorent la valeur (≥ seuil) ; `warn` supérieur à `crit` inverse le sens (plus bas = pire, ex. espace libre). Pour `date`, les seuils portent sur l'âge en secondes. `placement` : `tile` (défaut), `detail` ou `hidden`.

## Secrets (tous facultatifs)

| Rôle | Effet |
|---|---|
| `token` | `Authorization: Bearer …` |
| `key` | en-tête `X-Api-Key` (ou `options.key_header`) |
| `user`, `pass` | authentification basique (les deux ensemble) |
| `post_key` | `Authorization: Bearer …` du bouton `post_url` seulement |

Donnez une clé en lecture seule si l'API le permet. Évitez de mettre une clé dans l'URL : elle est retirée des messages d'erreur, mais reste en clair dans config.yaml.

## Limites

- Lecture en GET seulement (`options.method: POST` est refusé). Seule exception, un bouton facultatif : `options.post_url` (POST sans corps au clic), `options.post_label` (libellé), clé `post_key` distincte de celle de lecture. `post_url` doit viser le même schéma et le même hôte:port que `url`.
- Un chemin introuvable, une valeur `null` ou non numérique s'affiche « — » avec la raison, sans toucher aux autres champs. API injoignable ou réponse non JSON : toute la tuile en erreur.
- Pas de liste ou table générique : une valeur par champ.
