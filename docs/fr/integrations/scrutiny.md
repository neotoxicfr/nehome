# Scrutiny

Santé SMART des disques (Scrutiny) : états, températures, heures de fonctionnement. Relevé toutes les 5 min.

## Configuration

- Aucun secret : Scrutiny n'a pas d'authentification (à protéger côté reverse proxy).
- Options : aucune.

```yaml
- id: scrutiny
  type: scrutiny
  title: Disques
  url: http://scrutiny:8080
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| disks | Disques | tuile |
| failed | Disques en échec | tuile |
| hottest | Disque le plus chaud | tuile |
| temps | Températures (orange 50, rouge 60 °C) | détail |
| disk_list | Disques : modèle, capacité, état SMART, température, heures | détail |

## Limites

- Seuils de température fixes, pensés pour des disques durs ; ils sont stricts pour des NVMe.
- Le collecteur ne passe qu'une fois par jour par défaut : les valeurs bougent peu.
- Non éprouvé sur une vraie instance.
