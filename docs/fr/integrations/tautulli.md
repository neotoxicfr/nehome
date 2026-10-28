# Tautulli

Flux Plex en cours, bande passante, lectures par jour, titres et utilisateurs populaires, historique.

## Configuration

```yaml
- id: tautulli
  type: tautulli
  title: Tautulli
  section: Médias
  url: http://tautulli:8181
  secrets: {key: tautulli_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | clé d'API de Tautulli (Paramètres, Interface web, API) |

L'API de Tautulli n'accepte la clé que dans l'adresse (`?apikey=`) : nehome la retire des messages d'erreur, mais elle peut apparaître dans les journaux d'un proxy placé entre les deux. Gardez les deux sur un réseau privé.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `streams`, `bandwidth`, `plays` | Flux en cours, Bande passante, Lectures par jour (14 j) | tuile |
| `peak` | Pic de flux simultanés (7 j) | détail |
| `popular_movies`, `popular_tv`, `top_users`, `history` | tables : films populaires, séries populaires, utilisateurs actifs, historique récent | détail |

## Rythme

Flux toutes les 5 s, historique toutes les minutes, statistiques lourdes (7 jours, lectures par jour) toutes les 5 min.
