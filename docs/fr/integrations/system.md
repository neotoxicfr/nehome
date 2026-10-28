# Système (hôte Linux)

Métriques de la machine sans agent : CPU, charge, mémoire et swap, températures, débit réseau, occupation des disques, durée de marche, nom d'hôte et noyau. Tout est lu dans `/proc`, `/sys` et par `statfs` ; Linux uniquement.

## Configuration

```yaml
- id: nas
  type: system
  title: NAS
  section: Système
  options:
    proc: /host/proc                       # défaut /proc
    sys: /sys                              # défaut /sys
    disks: "Système=/, Données=/host/tank" # libellé=point de montage, ou chemin seul ; défaut /
```

### Dans un conteneur

Sans montage, nehome voit le `/proc` de son conteneur (CPU et mémoire justes, réseau et nom d'hôte du conteneur). Pour l'hôte :

```yaml
services:
  nehome:
    volumes:
      - /proc:/host/proc:ro        # puis options.proc: /host/proc
      - /mnt/tank:/host/tank:ro    # un montage par disque à surveiller (options.disks)
    uts: host                      # facultatif : vrai nom d'hôte au lieu de celui du conteneur
```

- Réseau : nehome lit `<proc>/1/net/dev` (l'espace réseau du processus 1 de l'hôte), sinon `<proc>/net/dev`. Si le débit affiché reste celui du conteneur, passez le service en `network_mode: host`.
- Températures : `/sys/class/hwmon` est déjà visible dans un conteneur Docker ; rien à monter.
- Disques : `statfs` sur un montage en lecture seule donne l'occupation du système de fichiers d'origine. Sans `options.disks`, la barre « / » est celle du stockage Docker.

## Secrets

Aucun.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `cpu`, `ram`, `disks`, `net_down`, `net_up` | CPU (courbe), Mémoire (jauge), Disques (barres), Réseau réception, Réseau envoi (courbes) | tuile |
| `load`, `temp`, `swap`, `uptime`, `host`, `kernel` | charge 1/5/15 min, température la plus haute, swap, durée de marche, hôte, noyau | détail |
| `temps`, `interfaces` | tables : capteurs de température, débit par interface | détail |

Seuils : CPU 80/95 %, mémoire 85/95 %, swap 50/80 %, disques 85/95 %, charge 1×/2× le nombre de cœurs ; températures selon `temp*_max`/`temp*_crit` du capteur, sinon 80/90 °C.

## Rythme

CPU et réseau toutes les 3 s (courbes de 5 min) ; le reste toutes les 30 s.

## Limites

- Interfaces ignorées : `lo`, `veth*`, `docker*`, `br-*`. Une interface qui apparaît, ou dont le compteur recule (redémarrage), compte au tour suivant.
- Températures : hwmon d’abord ; `thermal_zone` seulement si hwmon ne donne rien (les deux décrivent souvent les mêmes capteurs). Valeurs hors de 0-200 °C écartées.
- Un point de montage illisible s'affiche « inaccessible » en rouge.
