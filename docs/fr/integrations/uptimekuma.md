# Uptime Kuma

État des moniteurs, disponibilité sur 24 h, latence moyenne, incident en cours.

## Configuration

```yaml
- id: uptime
  type: uptimekuma
  title: Disponibilité
  section: Supervision
  url: http://uptime-kuma:3001
  options: {slugs: "services,reseau"}
  secrets: {key: uptimekuma_key}      # facultatif
```

## Options

| Option | Rôle | Défaut |
|---|---|---|
| `slugs` | pages de statut publiques à lire, séparées par des virgules | `plex` |

Uptime Kuma 2 n'a pas d'API REST : nehome lit les pages de statut publiques. Les moniteurs absents de ces pages ne sont pas vus, sauf avec la clé d'API.

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API Uptime Kuma (Paramètres, Clés API) ; **facultatif** |

Avec la clé, `/metrics` (authentification basique : utilisateur vide, clé en mot de passe) ajoute tous les moniteurs. Si `secrets.key` est déclaré, le fichier doit exister.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `summary`, `uptime`, `latency` | État, Disponibilité (24 h), Latence moyenne | tuile |
| `incident` | Incident en cours | détail |
| `monitors` | Moniteurs, ceux en panne en premier | détail |
| `metrics` | Tous les moniteurs (nécessite la clé) | détail |

## Rythme

Toutes les 30 s.
