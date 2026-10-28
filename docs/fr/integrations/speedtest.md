# Speedtest Tracker

Dernier test de débit et courbe des derniers tests (Speedtest Tracker d'alexjustesen, API v1). Relevé toutes les 5 min.

## Configuration

- Secret `token` : jeton d'API (Profil > Jetons d'API) avec le droit de lire les résultats.
- Options : aucune.

```yaml
- id: speedtest
  type: speedtest
  title: Débit Internet
  url: http://speedtest
  secrets: {token: speedtest_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| download | Débit descendant | tuile |
| upload | Débit montant | tuile |
| ping | Latence | tuile |
| when | Dernier test | détail |
| server | Serveur de test, fournisseur | détail |
| download_hist, upload_hist, ping_hist | Courbes des 30 derniers tests | détail |

## Limites

- Débits lus en bits/s (`download_bits`) ou, à défaut, octets/s x 8 ; affichés en Mbit/s.
- Forme de la réponse d'après la documentation, non éprouvée sur une vraie instance.
