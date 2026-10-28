# Speedtest Tracker

> Version française : [../fr/integrations/speedtest.md](../fr/integrations/speedtest.md)


Latest speed test and chart of recent tests (alexjustesen's Speedtest Tracker, API v1). Polled every 5 min.

## Configuration

- Secret `token`: API token (Profile > API tokens) with permission to read results.
- Options: none.

```yaml
- id: speedtest
  type: speedtest
  title: Débit Internet
  url: http://speedtest
  secrets: {token: speedtest_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| download | « Débit descendant » (download speed) | tile |
| upload | « Débit montant » (upload speed) | tile |
| ping | « Latence » (latency) | tile |
| when | « Dernier test » (last test) | detail |
| server | « Serveur de test » (test server), provider | detail |
| download_hist, upload_hist, ping_hist | charts of the last 30 tests | detail |

## Limits

- Speeds are read in bits/s (`download_bits`) or, failing that, bytes/s x 8; displayed in Mbit/s.
- Response shape follows the documentation, not tested on a real instance.
