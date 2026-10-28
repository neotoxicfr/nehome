# Gotify

Messages récents et applications d'un serveur de notifications Gotify. Relevé toutes les 30 s.

## Configuration

- Secret `token` : jeton de **client** (Clients > Créer), pas celui d'une application : seul un client peut lire les messages.
- Options : aucune.

```yaml
- id: gotify
  type: gotify
  title: Gotify
  url: http://gotify
  secrets: {token: gotify_client_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| day | Messages (24 h) | tuile |
| last | Dernier message (ancienneté, application) | tuile |
| apps | Applications | tuile |
| high | Priorité haute (>= 8) sur 24 h | détail |
| messages | Messages récents (application, titre, priorité) | détail |
| app_list | Applications déclarées | détail |
| version | Version | détail |

## Limites

- Le corps des messages n'est jamais affiché (il peut être sensible), ni les jetons d'application.
- Le compte 24 h porte sur les 100 derniers messages ("100+" au-delà).
- Non éprouvé sur une vraie instance.
