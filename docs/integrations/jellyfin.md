# Jellyfin and Emby

> Version française : [../fr/integrations/jellyfin.md](../fr/integrations/jellyfin.md)

Current streams (including transcodes), library counters, recent activity and version. Same API: `type: jellyfin` or `type: emby`.

## Configuration

```yaml
- id: jellyfin
  type: jellyfin        # or emby
  title: Jellyfin
  section: Médias
  href: https://jellyfin.example.org/
  url: http://jellyfin:8096
  secrets: {key: jellyfin_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | API key (Jellyfin: Dashboard, API Keys; Emby: Advanced, API Keys), sent in the `X-Emby-Token` header |

An API key has administrator rights: nehome only sends GETs. Emby is queried under `/emby`.

## Fields

| ID | Label | Default |
|---|---|---|
| `streams`, `transcodes`, `movies`, `series` | « Lectures en cours » (current streams), « Transcodages » (transcodes), « Films » (movies), « Séries » (series) | tile |
| `episodes`, `music`, `books` | « Épisodes » (episodes), « Musique » (music: tracks, albums, artists), « Livres » (books) | detail |
| `sessions`, `activity`, `version` | playing sessions, recent activity, version (alert if an update or restart is pending) | detail |

## Refresh rate

Sessions every 10 s; counters, version and activity every 5 min.

## Limits

Only sessions with media currently playing are counted. `music` and `books` are hidden if the library has none. Counters are global (not per library). Not tested against a real server: simulated responses based on Jellyfin's OpenAPI.
