# Audiobookshelf

Éléments, durée et taille des bibliothèques, écoutes en cours et version.

## Configuration

```yaml
- id: audiobookshelf
  type: audiobookshelf
  title: Audiobookshelf
  section: Médias
  href: https://abs.exemple.org/
  url: http://audiobookshelf:80
  secrets: {token: abs_token}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | jeton d'API d'un compte **administrateur** (Réglages, Utilisateurs), envoyé en `Authorization: Bearer` |

Le compte doit être administrateur : `/api/users/online` est réservé aux admins. nehome n'émet que des GET.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `items`, `streams`, `duration`, `size` | Éléments, Écoutes en cours, Durée totale, Taille | tuile |
| `authors`, `libraries`, `sessions`, `version` | Auteurs, table des bibliothèques, sessions ouvertes, version | détail |

## Rythme

Sessions toutes les 15 s ; bibliothèques (une requête de statistiques par bibliothèque) et version toutes les 5 min.

## Limites

Les statistiques de bibliothèque ne sont pas émises si aucune ne répond. Une session ouverte n'est pas forcément en lecture active. Non testé sur un vrai serveur : réponses simulées d'après la doc de l'API.
