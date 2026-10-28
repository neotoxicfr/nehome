# Météo

Température, heure locale, soleil, pluie, vent, humidité et prévisions à 3 jours, via Open-Meteo (sans clé).

## Configuration

```yaml
- id: meteo
  type: weather
  title: Météo
  section: Divers
  options: {lat: "48.8566", lon: "2.3522", timezone: Europe/Paris}
```

## Options

| Option | Rôle | Défaut |
|---|---|---|
| `lat`, `lon` | coordonnées en degrés décimaux, **obligatoires** | |
| `timezone` | fuseau IANA | `auto` |

## Secrets

Aucun. Le serveur nehome doit pouvoir joindre `api.open-meteo.com` en HTTPS.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `temp`, `heure`, `soleil`, `pluie` | Température, Heure locale, Lever / coucher, Pluie aujourd'hui | tuile |
| `vent`, `humidite`, `previsions` | Vent, Humidité, Prévisions sur 3 jours | détail |

## Rythme et limites

Prévisions toutes les 15 min ; l'heure locale est recalculée chaque minute depuis le décalage UTC renvoyé par l'API (aucune base de fuseaux dans l'image). Open-Meteo est gratuit pour un usage non commercial.
