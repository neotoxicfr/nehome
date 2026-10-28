# Tracearr

Surveillance des comptes d'un serveur multimédia : flux en cours, lectures du jour, violations, qualité de lecture, plateformes, score de confiance des comptes.

## Configuration

```yaml
- id: tracearr
  type: tracearr
  title: Tracearr
  section: Médias
  url: http://tracearr:3000
  secrets: {key: tracearr_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | jeton de l'API publique v1 (envoyé en `Bearer`), créé dans les réglages de Tracearr |

nehome appelle seulement des routes de lecture de `/api/v1/public`.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `streams`, `today`, `violations`, `plays` | Flux en cours, Lectures aujourd'hui, Violations récentes, Lectures par jour (30 j) | tuile |
| `users`, `server` | Comptes, Serveurs | détail |
| `quality` | Qualité de lecture (30 j, jauges) | détail |
| `details`, `platforms`, `trust` | tables : flux détaillés (codecs, HDR), plateformes, comptes à surveiller | détail |

## Rythme

Flux toutes les 5 s, compteurs toutes les minutes, statistiques sur 30 jours toutes les 5 min.
