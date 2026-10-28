# Glances

Processeur, charge, mémoire, swap, disques, réseau, températures et processus les plus gourmands d'une machine.

## Configuration

```yaml
- id: glances
  type: glances
  title: Glances
  section: Infrastructure
  href: https://glances.exemple.org/
  url: http://192.0.2.10:61208
  secrets: {user: glances_user, pass: glances_pass}   # facultatif
```

## Secrets

| Rôle | Contenu |
|---|---|
| `user`, `pass` | facultatifs, ensemble : authentification basique si Glances a été lancé avec `--password` |

Lecture seule ; l'API REST de Glances n'expose que des GET.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `cpu, ram, load, temp` | processeur, mémoire, charge (1 min), température maximale | tuile |
| `curve, swap, system, uptime, disks, net, sensors, procs` | historique du processeur, swap, système, uptime, jauges de disques, débits par interface, capteurs, top processus | détail |

## Rythme

Processeur, mémoire, charge et réseau toutes les 5 s ; le reste toutes les minutes.

## Limites

Lancer Glances en mode serveur web (`glances -w`). API v4 d'abord, repli automatique sur v3. Une tuile par machine. Le débit réseau suppose les champs `*_rate_per_sec` de Glances.
