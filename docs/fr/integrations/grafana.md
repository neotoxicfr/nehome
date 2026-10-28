# Grafana

État de santé, statistiques globales et alertes en cours d'une instance Grafana. Relevé toutes les 30 s.

## Configuration

- Secret `key` : jeton de compte de service (Administration > Comptes de service), rôle Viewer suffisant pour les alertes. Les statistiques (`/api/admin/stats`) exigent un droit d'administrateur du serveur.
- Options : aucune.

```yaml
- id: grafana
  type: grafana
  title: Grafana
  url: http://grafana:3000
  secrets: {key: grafana_token}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| health | État (base de données) | tuile |
| alerts | Alertes en cours | tuile |
| dashboards | Tableaux de bord | tuile |
| users, datasources, orgs, sessions | Statistiques admin | détail |
| alert_list | Alertes déclenchées (15 max) | détail |
| version | Version | détail |

## Limites

- Sans droit d'administrateur serveur, les statistiques sont masquées ; la santé et les alertes restent.
- Les alertes viennent de l'Alertmanager intégré de Grafana (actives, ni silencées ni inhibées).
- Non éprouvé sur une vraie instance.
