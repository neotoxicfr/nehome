# CrowdSec

Alertes des dernières 24 h, bannissements actifs, alertes AppSec, pays et scénarios les plus fréquents.

## Configuration

```yaml
- id: crowdsec
  type: crowdsec
  title: CrowdSec
  section: Sécurité
  url: http://crowdsec:8080
  secrets: {user: crowdsec_machine, pass: crowdsec_password}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user` | identifiant du compte « machine » de la LAPI |
| `pass` | son mot de passe |

Créez un compte dédié : `cscli machines add nehome --password '<mot de passe>'`. Une machine suffit (droits de lecture des alertes).

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `alerts`, `decisions`, `appsec` | Alertes (24 h), Bannissements actifs, Alertes AppSec (24 h) | tuile |
| `curve` | Alertes par heure | détail |
| `countries`, `scenarios`, `recent`, `banned` | tables : pays sources, scénarios, dernières alertes, bannissements en cours | détail |

## Rythme et limites

Toutes les minutes. `/v1/decisions` est réservé aux bouncers (403 pour une machine) : les bannissements actifs sont déduits des alertes (`has_active_decision`). Le jeton de login est renouvelé avant son expiration. L'action « Débannir une IP » (`DELETE /v1/decisions?ip=`, comme `cscli decisions delete`) ne propose que les IP bannies localement.
