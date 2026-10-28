# System (Linux host)

> Version française : [../fr/integrations/system.md](../fr/integrations/system.md)


Agentless machine metrics: CPU, load, memory and swap, temperatures, network throughput, disk usage, uptime, hostname and kernel. Everything is read from `/proc`, `/sys` and via `statfs`; Linux only.

## Configuration

```yaml
- id: nas
  type: system
  title: NAS
  section: Système
  options:
    proc: /host/proc                       # default /proc
    sys: /sys                              # default /sys
    disks: "Système=/, Données=/host/tank" # label=mount point, or path alone; default /
```

### In a container

Without mounts, nehome sees its own container's `/proc` (correct CPU and memory, but the container's network and hostname). For the host:

```yaml
services:
  nehome:
    volumes:
      - /proc:/host/proc:ro        # then options.proc: /host/proc
      - /mnt/tank:/host/tank:ro    # one mount per disk to monitor (options.disks)
    uts: host                      # optional: real hostname instead of the container's
```

- Network: nehome reads `<proc>/1/net/dev` (the host's process 1 network namespace), otherwise `<proc>/net/dev`. If the displayed throughput is still the container's, switch the service to `network_mode: host`.
- Temperatures: `/sys/class/hwmon` is already visible in a Docker container; nothing to mount.
- Disks: `statfs` on a read-only mount reports the usage of the original filesystem. Without `options.disks`, the "/" bar is Docker's storage.

## Secrets

None.

## Fields

| ID | Label | Default |
|---|---|---|
| `cpu`, `ram`, `disks`, `net_down`, `net_up` | « CPU » (chart), « Mémoire » (memory, gauge), « Disques » (disks, bars), « Réseau réception » (network receive), « Réseau envoi » (network send) (charts) | tile |
| `load`, `temp`, `swap`, `uptime`, `host`, `kernel` | 1/5/15 min load, highest temperature, swap, uptime, host, kernel | detail |
| `temps`, `interfaces` | tables: temperature sensors, throughput per interface | detail |

Thresholds: CPU 80/95 %, memory 85/95 %, swap 50/80 %, disks 85/95 %, load 1×/2× the core count; temperatures use the sensor's `temp*_max`/`temp*_crit`, otherwise 80/90 °C.

## Polling

CPU and network every 3 s (5-minute charts); the rest every 30 s.

## Limits

- Ignored interfaces: `lo`, `veth*`, `docker*`, `br-*`. An interface that appears, or whose counter goes backwards (reboot), counts from the next cycle.
- Temperatures: hwmon first; `thermal_zone` only if hwmon yields nothing (both often describe the same sensors). Values outside 0-200 °C are discarded.
- An unreadable mount point is shown as « inaccessible » (inaccessible) in red.
