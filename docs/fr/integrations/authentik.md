# authentik

Utilisateurs actifs, applications, version et connexions échouées de la dernière journée. Relevé toutes les minutes.

## Configuration

- Secret `token` : jeton d'API (Répertoire > Jetons et mots de passe d'application). Utilisateur dédié avec les droits de voir les utilisateurs, applications et événements.
- Options : aucune.

```yaml
- id: authentik
  type: authentik
  title: authentik
  url: https://auth.example.com
  secrets: {token: authentik_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| users | Utilisateurs actifs | tuile |
| failed | Connexions échouées (24 h) : orange > 0, rouge >= 10 | tuile |
| apps | Applications | tuile |
| version | Version (orange si mise à jour dispo) | détail |
| failed_list | Dernières connexions échouées : quand, IP, étape | détail |

## Limites

- Les identifiants saisis lors des échecs ne sont jamais affichés.
- Le compte 24 h porte sur les 100 derniers événements ("100+" au-delà).
- Non éprouvé sur une vraie instance.
