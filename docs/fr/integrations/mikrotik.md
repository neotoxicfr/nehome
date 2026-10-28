# MikroTik (RouterOS 7)

Débit WAN, CPU, température, mémoire, baux DHCP, clients Wi-Fi, pairs WireGuard et journal du routeur.

## Configuration

```yaml
- id: routeur
  type: mikrotik
  title: Routeur
  section: Réseau
  href: https://routeur.exemple.org/
  url: http://192.0.2.1:8082     # service www (REST), sans /rest
  secrets: {user: mikrotik_user, pass: mikrotik_pass}
  options: {wan: ether1}
```

## Options

| Option | Rôle | Défaut |
|---|---|---|
| `wan` | interface dont on mesure le débit | `ether1` |

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte RouterOS |

Droits minimaux : un groupe de lecture seule (`read`, `api`, `rest-api`) ; nehome n'utilise que des GET sur `/rest`.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `down`, `up` | WAN réception / envoi | tuile |
| `cpu`, `temp`, `uptime` | CPU, température, durée de fonctionnement | tuile |
| `leases`, `ram`, `hdd`, `version`, `wanip`, `ddns`, `ipv6`, `socks`, `psu` | baux DHCP, mémoire, stockage, version, IP publique, DDNS, préfixe IPv6, SOCKS, alimentation | détail |
| `wifi`, `wg`, `clients`, `logs` | tables : clients Wi-Fi, pairs WireGuard, baux actifs, journal (avertissements et erreurs) | détail |

## Rythme et limites

Chaque requête écrit une ligne « logged in » dans le journal du routeur : le débit WAN est lu toutes les 5 s, le reste toutes les 60 s. Les menus facultatifs en échec (Wi-Fi absent, par exemple) n'effacent pas la tuile.
