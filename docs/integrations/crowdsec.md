# CrowdSec

> Version française : [../fr/integrations/crowdsec.md](../fr/integrations/crowdsec.md)

Alerts from the last 24 h, active bans, AppSec alerts, most frequent countries and scenarios.

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

| Role | Content |
|---|---|
| `user` | LAPI "machine" account login |
| `pass` | its password |

Create a dedicated account: `cscli machines add nehome --password '<password>'`. A machine is enough (alert read access).

## Fields

| ID | Label | Default |
|---|---|---|
| `alerts`, `decisions`, `appsec` | « Alertes (24 h) » (alerts), « Bannissements actifs » (active bans), « Alertes AppSec (24 h) » | tile |
| `curve` | « Alertes par heure » (alerts per hour) | detail |
| `countries`, `scenarios`, `recent`, `banned` | tables: source countries, scenarios, latest alerts, current bans | detail |

## Polling and limits

Every minute. `/v1/decisions` is reserved for bouncers (403 for a machine): active bans are inferred from alerts (`has_active_decision`). The login token is renewed before it expires. The « Débannir une IP » action (`DELETE /v1/decisions?ip=`, like `cscli decisions delete`) only offers locally banned IPs.
