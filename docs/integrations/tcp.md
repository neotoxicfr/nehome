# TCP ports

> Version française : [../fr/integrations/tcp.md](../fr/integrations/tcp.md)


Checks that TCP ports accept connections (connect then close immediately, nothing is sent) and measures connection setup time.

## Configuration

```yaml
- id: ports
  type: tcp
  title: Ports
  section: Réseau
  options:
    targets: "SSH=192.0.2.10:22, Postgres=db:5432, IPv6=[2001:db8::1]:443, 192.0.2.1:53"
```

`targets`: comma-separated list of `name=host:port`; without a name, the address is used as the name.

## Secrets

None.

## Fields

| ID | Label | Default |
|---|---|---|
| `down`, `up`, `latency` | « En panne » (down, with names), « Joignables » (reachable, "4 / 5"), « Latence moyenne » (average latency, and the slowest) | tile |
| `targets` | table: name, address, status (open, refused, timed out, unknown name, unreachable), latency | detail |

## Polling

Every 30 s, targets in parallel, 3 s timeout per target.

## Limits

- Latency includes the DNS resolution of the name.
- An open port says nothing about the health of the service behind it; the table shows at most 15 targets (failures first).
