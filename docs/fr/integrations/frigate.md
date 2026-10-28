# Frigate

Caméras, images par seconde, détections, inférence, GPU et stockage des enregistrements. Relevé toutes les 10 s.

## Configuration

- Aucun secret : cibler le port interne **5000** (sans authentification). Le port 8971 authentifié n'est pas géré.
- Options : aucune.

```yaml
- id: frigate
  type: frigate
  title: Frigate
  url: http://frigate:5000
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| cameras | Caméras en ligne (images reçues) | tuile |
| detection_fps | Détections par seconde | tuile |
| inference | Inférence du détecteur le plus lent | tuile |
| storage | Enregistrements | tuile |
| fps_hist | Historique des détections | détail |
| camera_list | Caméras : capture, traitement, détection, perdues | détail |
| gpu | Usage GPU | détail |
| uptime, version | Disponibilité, version | détail |

## Limites

- Le stockage est exprimé en Mo par Frigate ; converti en approximation décimale.
- Non éprouvé sur une vraie instance.
