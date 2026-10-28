# OpenMediaVault

Processeur, mémoire, charge, uptime, mises à jour disponibles, systèmes de fichiers et santé SMART des disques.

## Configuration

```yaml
- id: openmediavault
  type: openmediavault
  title: OpenMediaVault
  section: Infrastructure
  href: https://openmediavault.exemple.org/
  url: http://192.0.2.31
  secrets: {user: omv_user, pass: omv_pass}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | compte d'administration web OMV (par exemple `admin`) |

OMV n'a pas de rôle en lecture seule pour son RPC : le compte est administrateur. nehome n'appelle que `System.getInformation`, `FileSystemMgmt.enumerateFilesystems` et `Smart.getList`. Ne pas exposer ce compte ailleurs.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `cpu, ram, fs, smart` | processeur, mémoire, systèmes de fichiers (jauges), santé SMART globale | tuile |
| `host, version, uptime, load, updates, disks` | machine, version, uptime, charge, mises à jour, table des disques | détail |

## Rythme

Système toutes les 15 s ; systèmes de fichiers et SMART toutes les 5 min.

## Limites

OMV 6 et 7 (/rpc.php). Session par cookie, reconnexion automatique. La santé SMART n'apparaît que si le contrôle SMART est activé dans OMV.
