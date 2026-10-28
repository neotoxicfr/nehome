# UniFi Network

> Version française : [../fr/integrations/unifi.md](../fr/integrations/unifi.md)


Connected clients, devices (access points, switches, gateway), Internet status, WAN throughput and most active clients.

## Configuration

```yaml
- id: unifi
  type: unifi
  title: UniFi Network
  section: Infrastructure
  href: https://unifi.example.org/
  url: https://192.0.2.2
  insecure_tls: true
  secrets: {user: unifi_user, pass: unifi_pass}
  options: {site: default}
```

## Secrets

| Role | Content |
|---|---|
| `user`, `pass` | local controller account |

Use a dedicated local account with the « Lecture seule » (View Only) role and no MFA; an Ubiquiti SSO account does not work. Only GETs are made after login.

## Fields

| ID | Content | Default |
|---|---|---|
| `clients, devices, wan, throughput` | Wi-Fi + wired clients, connected devices, Internet status, WAN throughput | tile |
| `health, devtable, clienttable` | health per subsystem, device table, most active clients | detail |

## Polling

Every 30 s.

## Limits

`options.site` (default `default`); `options.unifi_os` (`true` or `false`) forces the mode, otherwise it is detected: UniFi OS (UDM, Cloud Gateway: /proxy/network) then classic controller. The UniFi API is not officially documented: fields may vary by version, and missing ones are hidden. Tested on reconstructed responses, not on a real controller.
