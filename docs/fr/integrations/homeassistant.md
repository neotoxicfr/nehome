# Home Assistant

Nombre d'entités par domaine, entités indisponibles, état du système, et une sélection d'entités de votre choix avec leur état et leur unité.

## Configuration

```yaml
- id: homeassistant
  type: homeassistant
  title: Home Assistant
  section: Infrastructure
  href: https://homeassistant.exemple.org/
  url: http://192.0.2.20:8123
  secrets: {token: ha_token}
  options:
    entities: "sensor.salon_temperature, binary_sensor.porte_entree"
```

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | jeton d'accès longue durée (profil utilisateur > Sécurité) |

Le jeton hérite des droits de l'utilisateur. Créez un utilisateur non administrateur dédié ; nehome ne fait que des GET (`/api/config`, `/api/states`).

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `watched, entities, unavailable` | entités suivies (option `entities`), nombre d'entités, entités indisponibles | tuile |
| `state, version, domains, unavailablelist` | état du système, version, entités par domaine, liste des indisponibles | détail |

## Rythme

Toutes les 30 s.

## Limites

`options.entities` : entity_id séparés par des virgules ; le nom affiché est le `friendly_name`. Seul l'état « unavailable » compte comme indisponible (« unknown » est un état normal pour certaines entités). La liste complète des états est relue à chaque cycle.
