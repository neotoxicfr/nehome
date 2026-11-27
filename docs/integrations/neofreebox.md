# neofreebox

> Version française : [../fr/integrations/neofreebox.md](../fr/integrations/neofreebox.md)


Freebox seen through [neofreebox](https://github.com/neotoxicfr/neofreebox): connection state, rates as sparklines against the line maximum, connected devices, temperature, Wi-Fi, missed calls, sensors. Polled every 10 s.

## Configuration

- `token` secret: neofreebox API token; a read-only token is enough (`neofreebox token -name nehome -read-only`).
- `url`: neofreebox address, no path; the tile reads `/api/v1/summary`. Options: none.

```yaml
- id: freebox
  type: neofreebox
  title: Freebox
  url: http://neofreebox:8080
  secrets: {token: neofreebox_key}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| state | connection up or down, and its medium (FTTH, xDSL…) | tile |
| down | download rate sparkline, against the line maximum | tile |
| up | upload rate sparkline, against the line maximum | tile |
| devices | connected devices, out of all devices the box knows | tile |
| temp | highest temperature (orange from 80 °C, red from 90 °C) | tile |
| wifi | Wi-Fi on or off | tile |
| calls | unread missed calls | tile |
| uptime | time since the box last started | detail |
| box | model and firmware | detail |
| sensors | every temperature and fan | detail |

## Limits

- `devices`, `wifi` and `calls` stay empty when the neofreebox app lacks those rights on the box.
- The response contains no address, MAC or device name.
