# wg-easy

> Version française : [../fr/integrations/wgeasy.md](../fr/integrations/wgeasy.md)


WireGuard clients: connected, last handshake, traffic. Versions 14 and 15. Polled every 15 s.

## Configuration

- v14: `pass` secret only (interface password, sent in the `Authorization` header).
- v15: `user` and `pass` secrets (Basic authentication).
- The version is chosen by the presence of the `user` role. Options: none.

```yaml
# v14
- id: wg
  type: wgeasy
  title: WireGuard
  url: http://wg-easy:51821
  secrets: {pass: wg_pass}
# v15: add user
#  secrets: {user: wg_user, pass: wg_pass}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| online | connected clients (handshake < 3 min) | tile |
| rx | received, cumulative | tile |
| tx | sent, cumulative | tile |
| clients | client count | detail |
| client_list | clients: address, status, handshake, traffic | detail |

## Limits

- Counters are cumulative since the WireGuard interface started, not instantaneous rates.
- Client keys and configurations are never read.
- Not tested on a real instance (v14/v15 schemas based on the source code).
