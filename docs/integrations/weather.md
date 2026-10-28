# Weather

> Version française : [../fr/integrations/weather.md](../fr/integrations/weather.md)


Temperature, local time, sun, rain, wind, humidity and 3-day forecast, via Open-Meteo (no key).

## Configuration

```yaml
- id: meteo
  type: weather
  title: Météo
  section: Divers
  options: {lat: "48.8566", lon: "2.3522", timezone: Europe/Paris}
```

## Options

| Option | Purpose | Default |
|---|---|---|
| `lat`, `lon` | coordinates in decimal degrees, **required** | |
| `timezone` | IANA time zone | `auto` |

## Secrets

None. The nehome server must be able to reach `api.open-meteo.com` over HTTPS.

## Fields

| ID | Label | Default |
|---|---|---|
| `temp`, `heure`, `soleil`, `pluie` | « Température » (temperature), « Heure locale » (local time), « Lever / coucher » (sunrise / sunset), « Pluie aujourd'hui » (rain today) | tile |
| `vent`, `humidite`, `previsions` | « Vent » (wind), « Humidité » (humidity), « Prévisions sur 3 jours » (3-day forecast) | detail |

## Polling and limits

Forecast every 15 min; local time is recomputed every minute from the UTC offset returned by the API (no time zone database in the image). Open-Meteo is free for non-commercial use.
