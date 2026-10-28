# Gluetun

État du VPN, IP publique, localisation, port transféré, opérateur et DNS. Une tuile par instance.

## Configuration

```yaml
- id: gluetun
  type: gluetun
  title: VPN
  section: Réseau
  url: http://gluetun:8000
```

`url` : serveur de contrôle de Gluetun (v3, port 8000 par défaut).

## Secrets

| Rôle | Usage |
|---|---|
| `key` | facultatif : clé d'un rôle `apikey` du fichier d'authentification du serveur de contrôle, envoyée en `X-API-Key`. Indispensable à l'action « Relancer le VPN » (`PUT /v1/vpn/status`) ; les lectures marchent sans si le rôle par défaut les autorise. |

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `vpn`, `ip`, `location`, `port` | VPN, IP publique, Localisation, Port transféré | tuile |
| `org`, `hostname`, `timezone`, `dns` | Opérateur, Nom d'hôte, Fuseau horaire, DNS du VPN | détail |

## Rythme

Toutes les 30 s. Lecture seule.
