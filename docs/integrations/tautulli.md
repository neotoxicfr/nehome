# Tautulli

> Version française : [../fr/integrations/tautulli.md](../fr/integrations/tautulli.md)


Current Plex streams, bandwidth, plays per day, popular titles and users, history.

## Configuration

```yaml
- id: tautulli
  type: tautulli
  title: Tautulli
  section: Médias
  url: http://tautulli:8181
  secrets: {key: tautulli_key}
```

## Secrets

| Role | Content |
|---|---|
| `key` | Tautulli API key (Settings, Web Interface, API) |

Tautulli's API only accepts the key in the URL (`?apikey=`): nehome strips it from error messages, but it may appear in the logs of a proxy placed between the two. Keep both on a private network.

## Fields

| ID | Label | Default |
|---|---|---|
| `streams`, `bandwidth`, `plays` | « Flux en cours » (current streams), « Bande passante » (bandwidth), « Lectures par jour » (plays per day, 14 d) | tile |
| `peak` | peak of concurrent streams (7 d) | detail |
| `popular_movies`, `popular_tv`, `top_users`, `history` | tables: popular movies, popular series, active users, recent history | detail |

## Polling

Streams every 5 s, history every minute, heavy statistics (7 days, plays per day) every 5 min.
