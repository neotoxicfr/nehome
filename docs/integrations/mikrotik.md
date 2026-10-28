# MikroTik (RouterOS 7)

> Version française : [../fr/integrations/mikrotik.md](../fr/integrations/mikrotik.md)

WAN throughput, CPU, temperature, memory, DHCP leases, Wi-Fi clients, WireGuard peers and the router log.

## Configuration

```yaml
- id: routeur
  type: mikrotik
  title: Routeur
  section: Réseau
  href: https://router.example.org/
  url: http://192.0.2.1:8082     # www service (REST), without /rest
  secrets: {user: mikrotik_user, pass: mikrotik_pass}
  options: {wan: ether1}
```

## Options

| Option | Purpose | Default |
|---|---|---|
| `wan` | interface whose throughput is measured | `ether1` |

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | RouterOS account |

Minimal rights: a read-only group (`read`, `api`, `rest-api`); nehome only uses GETs on `/rest`.

## Fields

| ID | Label | Default |
|---|---|---|
| `down`, `up` | WAN receive / send (« réception » / « envoi ») | tile |
| `cpu`, `temp`, `uptime` | CPU, temperature, uptime | tile |
| `leases`, `ram`, `hdd`, `version`, `wanip`, `ddns`, `ipv6`, `socks`, `psu` | DHCP leases, memory, storage, version, public IP, DDNS, IPv6 prefix, SOCKS, power supply | detail |
| `wifi`, `wg`, `clients`, `logs` | tables: Wi-Fi clients, WireGuard peers, active leases, log (warnings and errors) | detail |

## Refresh rate and limits

Every request writes a "logged in" line in the router log: WAN throughput is read every 5 s, everything else every 60 s. Optional menus that fail (no Wi-Fi, for example) do not blank the tile.
