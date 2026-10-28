# Grafana

> Version française : [../fr/integrations/grafana.md](../fr/integrations/grafana.md)

Health, global statistics and current alerts of a Grafana instance. Polled every 30 s.

## Configuration

- Secret `key`: service account token (Administration > Service accounts); the Viewer role is enough for alerts. Statistics (`/api/admin/stats`) require server administrator rights.
- Options: none.

```yaml
- id: grafana
  type: grafana
  title: Grafana
  url: http://grafana:3000
  secrets: {key: grafana_token}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| health | « État » (state; database) | tile |
| alerts | « Alertes en cours » (current alerts) | tile |
| dashboards | « Tableaux de bord » (dashboards) | tile |
| users, datasources, orgs, sessions | Admin statistics | detail |
| alert_list | Firing alerts (15 max) | detail |
| version | Version | detail |

## Limits

- Without server administrator rights, statistics are hidden; health and alerts remain.
- Alerts come from Grafana's built-in Alertmanager (active ones, neither silenced nor inhibited).
- Not proven on a real instance.
