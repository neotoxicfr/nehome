# Scrutiny

> Version française : [../fr/integrations/scrutiny.md](../fr/integrations/scrutiny.md)


SMART health of disks (Scrutiny): status, temperatures, power-on hours. Polled every 5 min.

## Configuration

- No secret: Scrutiny has no authentication (protect it at the reverse proxy).
- Options: none.

```yaml
- id: scrutiny
  type: scrutiny
  title: Disques
  url: http://scrutiny:8080
```

## Fields

| Field | Label | Placement |
|---|---|---|
| disks | « Disques » (disks) | tile |
| failed | « Disques en échec » (failed disks) | tile |
| hottest | « Disque le plus chaud » (hottest disk) | tile |
| temps | « Températures » (temperatures; orange 50, red 60 °C) | detail |
| disk_list | « Disques » (disks): model, capacity, SMART status, temperature, hours | detail |

## Limits

- Fixed temperature thresholds, designed for hard drives; they are strict for NVMe.
- The collector runs only once a day by default: values change little.
- Not tested on a real instance.
