# Portainer

Conteneurs en marche, arrêtés et en défaut, environnements, stacks.

## Configuration

```yaml
- id: portainer
  type: portainer
  title: Portainer
  section: Infrastructure
  href: https://portainer.exemple.org/
  url: http://192.0.2.10:9000
  secrets: {key: portainer_key}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key` | jeton d'accès (Mon compte > Access tokens), envoyé en `X-API-Key` |

Un utilisateur standard suffit pour voir les environnements auxquels il a accès ; aucune écriture n'est faite.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `running, unhealthy, endpoints, stacks` | conteneurs en marche, conteneurs en défaut, environnements en ligne, stacks actives | tuile |
| `version, envtable, stacktable` | version, détail par environnement, stacks | détail |

## Rythme

Toutes les 30 s.

## Limites

Les comptes viennent des instantanés d'environnement de Portainer (rafraîchis par Portainer, quelques minutes de retard possibles). Les stacks ne sont pas ventilées par conteneur.
