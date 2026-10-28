# ntfy

Messages reçus sur un sujet durant les dernières 24 h : nombre, dernier message, messages de priorité haute.

## Configuration

```yaml
- id: ntfy
  type: ntfy
  title: ntfy
  section: Supervision
  url: https://ntfy.exemple.org
  options: {topic: alertes}
  secrets: {user: ntfy_user, pass: ntfy_pass}   # facultatif
```

## Options

| Option | Rôle | Défaut |
|---|---|---|
| `topic` | sujet à lire | `uptime` |

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte avec droit de **lecture** sur le sujet ; facultatifs si le sujet est lisible anonymement |

Si `secrets.user` est déclaré, `pass` l'est aussi. Donnez au compte un accès `read-only` au seul sujet concerné (`ntfy access`).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `count`, `last`, `urgent` | Messages (24 h), Dernier message, Priorité haute (24 h) | tuile |
| `messages` | Messages des dernières 24 h (table) | détail |

## Rythme et limites

Toutes les 30 s, par `GET …/json?poll=1` (flux JSON ligne à ligne). Un refus du serveur est affiché avec le code HTTP. nehome ne publie jamais de message.
