# Proxmox VE

Nœuds, VM et conteneurs LXC (en marche ou arrêtés, avec ressources), occupation des stockages et tâches récentes en échec.

## Configuration

```yaml
- id: proxmox
  type: proxmox
  title: Proxmox VE
  section: Infrastructure
  href: https://proxmox.exemple.org/
  url: https://192.0.2.10:8006
  insecure_tls: true
  secrets: {token: proxmox_token}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `token` | jeton d'API complet au format `utilisateur@realm!idjeton=secret` (Datacenter > Permissions > API Tokens) |

Jeton avec « Privilege Separation » et le rôle **PVEAuditor** sur `/` (propagé). Aucun droit d'écriture.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `guests, cpu, ram, failed` | VM et conteneurs en marche (n / total), processeur et mémoire du cluster (nœuds en ligne), tâches en échec | tuile |
| `nodes, vms, storages, failedtasks` | table des nœuds, table des VM/LXC, jauges de stockage, tâches en échec | détail |

## Rythme

/cluster/resources toutes les 30 s ; /cluster/tasks toutes les 5 min.

## Limites

Les modèles (templates) sont ignorés. Un stockage partagé n'est compté qu'une fois. Les tâches en cours ou finies avec avertissement ne sont pas des échecs. Tests sur réponses conformes à l'API viewer, pas sur une instance réelle.
