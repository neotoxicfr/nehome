# UniFi Network

Clients connectés, appareils (points d'accès, switchs, passerelle), état d'Internet, débit WAN et clients les plus actifs.

## Configuration

```yaml
- id: unifi
  type: unifi
  title: UniFi Network
  section: Infrastructure
  href: https://unifi.exemple.org/
  url: https://192.0.2.2
  insecure_tls: true
  secrets: {user: unifi_user, pass: unifi_pass}
  options: {site: default}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte local du contrôleur |

Compte local dédié avec le rôle « Lecture seule » (View Only) et sans MFA ; un compte Ubiquiti SSO ne fonctionne pas. Seuls des GET sont faits après le login.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `clients, devices, wan, throughput` | clients Wi-Fi + filaires, appareils connectés, état d'Internet, débit WAN | tuile |
| `health, devtable, clienttable` | santé par sous-système, table des appareils, clients les plus actifs | détail |

## Rythme

Toutes les 30 s.

## Limites

`options.site` (défaut `default`) ; `options.unifi_os` (`true` ou `false`) force le mode, sinon il est détecté : UniFi OS (UDM, Cloud Gateway : /proxy/network) puis contrôleur classique. L'API UniFi n'est pas documentée officiellement : les champs peuvent varier selon la version, ceux qui manquent sont masqués. Tests sur réponses reconstituées, pas sur un contrôleur réel.
