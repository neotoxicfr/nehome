# Frigate

> Version française : [../fr/integrations/frigate.md](../fr/integrations/frigate.md)

Cameras, frames per second, detections, inference, GPU and recording storage. Polled every 10 s.

## Configuration

- No secret: target the internal port **5000** (unauthenticated). The authenticated port 8971 is not supported.
- Options: none.

```yaml
- id: frigate
  type: frigate
  title: Frigate
  url: http://frigate:5000
```

## Fields

| Field | Label | Placement |
|---|---|---|
| cameras | Cameras online (frames received) | tile |
| detection_fps | Detections per second | tile |
| inference | Inference time of the slowest detector | tile |
| storage | Recordings | tile |
| fps_hist | Detection history | detail |
| camera_list | Cameras: capture, processing, detection, dropped | detail |
| gpu | GPU usage | detail |
| uptime, version | Uptime, version | detail |

## Limits

- Frigate reports storage in MB; it is converted to a decimal approximation.
- Not proven on a real instance.
