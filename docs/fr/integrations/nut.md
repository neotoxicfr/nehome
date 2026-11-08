# Onduleur (NUT)

État d'un onduleur lu directement sur le serveur NUT (`upsd`) : secteur ou batterie, charge et autonomie de la batterie, charge de l'onduleur, tensions.

## Configuration

```yaml
- id: onduleur
  type: nut
  title: Onduleur
  section: Supervision
  href: https://ups.exemple.org/      # facultatif : PeaNUT, NUT-CGI…
  options:
    host: 192.0.2.10                  # serveur upsd
    port: "3493"                      # défaut 3493
    ups: ups                          # nom de l'onduleur dans ups.conf (défaut « ups »)
```

`upsd` doit écouter sur une adresse que nehome joint (`LISTEN` dans `upsd.conf`) et son pare-feu doit laisser passer le port 3493.

## Secrets

Aucun : `LIST VAR` se lit sans compte NUT.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `status` | État (sur secteur, sur batterie, batterie faible, surcharge, batterie à remplacer…) | tuile |
| `charge`, `runtime` | Batterie (jauge, alerte sous `battery.charge.warning` et `battery.charge.low`), Autonomie (alerte sous `battery.runtime.low`) | tuile |
| `load` | Charge (jauge, et puissance estimée d'après `ups.realpower.nominal`) | tuile |
| `input`, `output`, `battv` | Tensions d'entrée, de sortie et de la batterie | détail |
| `test`, `model` | Dernier autotest, Modèle | détail |

## Rythme et limites

Toutes les 30 s, une connexion TCP de 5 s au plus (`LIST VAR` puis `LOGOUT`). Lecture seule : aucune commande instantanée. La puissance n'est qu'une estimation quand l'onduleur ne mesure pas les watts.
