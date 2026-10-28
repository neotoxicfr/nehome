# Navidrome

> Version française : [../fr/integrations/navidrome.md](../fr/integrations/navidrome.md)

Current streams, track count, albums and artists, scan state, version. Through the Subsonic API.

## Configuration

```yaml
- id: navidrome
  type: navidrome
  title: Navidrome
  section: Médias
  href: https://music.example.org/
  url: http://navidrome:4533
  options: {user: alice}          # optional: otherwise secrets.user
  secrets: {pass: navidrome_pass}
```

## Secrets

| Role | Content |
|---|---|
| `pass` | account password |
| `user` | username, only if the `user` option is absent |

A regular account is enough. Authentication follows the Subsonic spec: salted token `md5(password + salt)`; the password is never sent in clear text. The token and salt travel in the URL (required by the spec); nehome strips them from displayed errors, but they may appear in the logs of a proxy in front of Navidrome.

## Fields

| ID | Label | Default |
|---|---|---|
| `playing`, `tracks`, `albums`, `artists` | « Lectures en cours » (current streams), « Morceaux » (tracks), « Albums » (albums), « Artistes » (artists) | tile |
| `scan`, `now_playing`, `version` | scan state, table of current streams, version | detail |

## Refresh rate

Current streams every 10 s; scan every minute; artists and albums (`getArtists`) every 5 min.

## Limits

"Albums" is the sum of the albums of the album artists (compilations count under their artist). "Tracks" is the number of files from the last scan. Not tested against a real Navidrome: simulated responses based on the Subsonic spec.
