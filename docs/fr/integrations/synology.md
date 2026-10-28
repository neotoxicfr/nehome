# Synology DSM

Processeur, mémoire, charge, débit réseau, volumes, température et état des disques d'un NAS Synology.

## Configuration

```yaml
- id: synology
  type: synology
  title: Synology DSM
  section: Infrastructure
  href: https://synology.exemple.org/
  url: https://192.0.2.30:5001
  insecure_tls: true
  secrets: {user: dsm_user, pass: dsm_pass}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte local DSM |

Compte dédié, sans 2FA (la double authentification n'est pas prise en charge). Les API `SYNO.Core.System`, `SYNO.Core.System.Utilization` et `SYNO.Storage.CGI.Storage` exigent en général un compte du groupe administrateurs : nehome n'appelle que des méthodes de lecture, mais le compte reste puissant. Restreignez-le par IP dans DSM et par pare-feu.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `cpu, ram, volumes, temp` | processeur, mémoire, volumes (jauges), température | tuile |
| `model, firmware, uptime, load, net, disktable` | modèle, version de DSM, uptime, charge, débit réseau total, table des disques | détail |

## Rythme

Utilisation toutes les 10 s ; système et stockage toutes les 5 min.

## Limites

Identifiants et sid sont envoyés en POST, jamais dans l'URL. La session est rouverte si DSM la perd. Le format de `up_time` varie selon les versions de DSM (secondes ou texte) : les deux sont gérés. Tests sur réponses conformes aux guides d'API, pas sur un vrai DSM ; les noms de champs de load_info peuvent différer selon les versions.
