# UPS (NUT)

> Version française : [../fr/integrations/nut.md](../fr/integrations/nut.md)

Status of a UPS read directly from the NUT server (`upsd`): mains or battery, battery charge and runtime, UPS load, voltages.

## Configuration

```yaml
- id: ups
  type: nut
  title: UPS
  section: Supervision
  href: https://ups.example.org/      # optional: PeaNUT, NUT-CGI…
  options:
    host: 192.0.2.10                  # upsd server
    port: "3493"                      # default 3493
    ups: ups                          # UPS name in ups.conf (default "ups")
```

`upsd` must listen on an address nehome can reach (`LISTEN` in `upsd.conf`) and its firewall must let port 3493 through.

## Secrets

None: `LIST VAR` is readable without a NUT account.

## Fields

| ID | Label | Default |
|---|---|---|
| `status` | « État » (on mains, on battery, low battery, overload, replace battery…) | tile |
| `charge`, `runtime` | « Batterie » (gauge, alert below `battery.charge.warning` and `battery.charge.low`), « Autonomie » (alert below `battery.runtime.low`) | tile |
| `load` | « Charge » (gauge, plus estimated power from `ups.realpower.nominal`) | tile |
| `input`, `output`, `battv` | input, output and battery voltages | detail |
| `test`, `model` | last self-test, model | detail |

## Polling and limits

Every 30 s, one TCP connection of 5 s at most (`LIST VAR` then `LOGOUT`). Read-only: no instant commands. The power is only an estimate when the UPS does not measure watts.
