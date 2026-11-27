# Portique

Machines de la passerelle [Portique](https://github.com/neotoxicfr/portique) (RDP et SSH dans le navigateur) : joignables ou non, accès, sessions ouvertes, et réveil à distance. Relevé toutes les 30 s.

## Configuration

- Secret `token` : clé de lecture de Portique (`status_key`), envoyée en `Authorization: Bearer`.
- Secret `wake` (facultatif) : clé de réveil (`wake_key`). Sans elle, aucun bouton de réveil.
- `url` : adresse de Portique, sans chemin. `href` : son adresse publique ; elle sert aussi aux liens vers les sessions (`<href>/session/<id>`, `<href>/ssh/<id>`). Options : aucune.

```yaml
- id: portique
  type: portique
  title: Portique
  href: https://portique.example.com/
  url: http://portique:8090
  secrets: {token: portique_key, wake: portique_wake_key}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| online | machines en ligne sur le total, et combien sont en panne (hors ligne sans réveil possible) | tuile |
| sessions | bureaux (RDP) et terminaux (SSH) ouverts | tuile |
| machines | une ligne par machine : état, liens « ouvrir » vers le bureau (RDP) et le terminal (SSH) dans Portique, sessions | tuile |

## Actions

- « Réveiller <machine> » : un bouton par machine qui a une adresse MAC de réveil, affiché sur la tuile. Portique refuse un second réveil trop rapproché.

## Limites

- « Éteinte » : machine réveillable qui ne répond pas ; ce n'est pas compté comme une panne.
- « Par rebond » : Portique ne joint la machine qu'à travers une autre, il ne sait pas dire si elle répond.
- La réponse ne contient ni adresse, ni MAC, ni compte.
