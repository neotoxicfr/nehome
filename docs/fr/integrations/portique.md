# Portique

Machines de la passerelle [Portique](https://github.com/neotoxicfr/portique) (RDP et SSH dans le navigateur) : joignables ou non, accès, sessions ouvertes, et réveil à distance. Relevé toutes les 30 s.

## Configuration

- Secret `token` : clé de lecture de Portique (`status_key`), envoyée en `Authorization: Bearer`.
- Secret `wake` (facultatif) : clé de réveil (`wake_key`). Sans elle, aucun bouton de réveil.
- `url` : adresse de Portique, sans chemin. Options : aucune.

```yaml
- id: portique
  type: portique
  title: Portique
  url: http://portique:8090
  secrets: {token: portique_key, wake: portique_wake_key}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| online | machines en ligne sur le total, et combien sont en panne (hors ligne sans réveil possible) | tuile |
| sessions | bureaux (RDP) et terminaux (SSH) ouverts | tuile |
| machines | une ligne par machine : état, accès, sessions | tuile |

## Actions

- « Réveiller une machine » : au choix parmi les machines qui ont une adresse MAC de réveil. Portique refuse un second réveil trop rapproché.

## Limites

- « Éteinte » : machine réveillable qui ne répond pas ; ce n'est pas compté comme une panne.
- « Par rebond » : Portique ne joint la machine qu'à travers une autre, il ne sait pas dire si elle répond.
- La réponse ne contient ni adresse, ni MAC, ni compte.
