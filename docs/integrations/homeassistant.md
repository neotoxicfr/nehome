# Home Assistant

> Version française : [../fr/integrations/homeassistant.md](../fr/integrations/homeassistant.md)

Entity count per domain, unavailable entities, system state, and a selection of entities of your choice with their state and unit.

## Configuration

```yaml
- id: homeassistant
  type: homeassistant
  title: Home Assistant
  section: Infrastructure
  href: https://homeassistant.example.org/
  url: http://192.0.2.20:8123
  secrets: {token: ha_token}
  options:
    entities: "sensor.salon_temperature, binary_sensor.porte_entree"
```

## Secrets

| Role | Content |
|---|---|
| `token` | long-lived access token (user profile > Security) |

The token inherits the user's permissions. Create a dedicated non-administrator user; nehome only makes GET requests (`/api/config`, `/api/states`).

## Fields

| ID | Content | Default |
|---|---|---|
| `watched, entities, unavailable` | followed entities (`entities` option), entity count, unavailable entities | tile |
| `state, version, domains, unavailablelist` | system state, version, entities per domain, list of unavailable ones | detail |

## Polling

Every 30 s.

## Limits

`options.entities`: comma-separated entity_ids; the displayed name is the `friendly_name`. Only the "unavailable" state counts as unavailable ("unknown" is a normal state for some entities). The full list of states is re-read on every cycle.
