# Portique

> Version française : [../fr/integrations/portique.md](../fr/integrations/portique.md)


Machines of the [Portique](https://github.com/neotoxicfr/portique) gateway (RDP and SSH in the browser): reachable or not, access, open sessions, and remote wake. Polled every 30 s.

## Configuration

- `token` secret: Portique read key (`status_key`), sent as `Authorization: Bearer`.
- `wake` secret (optional): wake key (`wake_key`). Without it, no wake button.
- `url`: Portique address, no path. Options: none.

```yaml
- id: portique
  type: portique
  title: Portique
  url: http://portique:8090
  secrets: {token: portique_key, wake: portique_wake_key}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| online | machines online out of the total, and how many are down (offline and not wakeable) | tile |
| sessions | open desktops (RDP) and terminals (SSH) | tile |
| machines | one row per machine: state, access, sessions | tile |

## Actions

- "Wake a machine": pick among machines that have a wake MAC address. Portique refuses a second wake too soon after the first.

## Limits

- "Éteinte" (off): a wakeable machine that does not answer; not counted as a fault.
- "Par rebond" (via a jump host): Portique only reaches the machine through another one and cannot tell whether it answers.
- The response contains no address, MAC or account.
