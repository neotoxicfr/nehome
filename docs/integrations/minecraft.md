# Minecraft (Java)

> Version française : [../fr/integrations/minecraft.md](../fr/integrations/minecraft.md)

Status of a Minecraft Java server through the Server List Ping protocol over native TCP: online, version, players, message of the day, latency. Polled every 30 s.

## Configuration

- No secrets.
- Options: `host` (otherwise the host of `url`), `port` (25565 by default).

```yaml
- id: minecraft
  type: minecraft
  title: Minecraft
  options: {host: mc.example.org, port: "25565"}
```

## Fields

| Field | Label | Placement |
|---|---|---|
| online | « État » (state; En ligne / Hors ligne, i.e. online / offline) | tile |
| players | « Joueurs » (players connected / max) | tile |
| version | « Version » (version) | tile |
| latency | « Latence » (latency; ping round trip) | detail |
| motd | « Message du jour » (message of the day; § codes stripped) | detail |
| player_list | « Joueurs de l'échantillon » (sampled players) | detail |
| history | « Joueurs connectés » (connected players; history) | detail |

## Limits

- An unreachable server shows "Hors ligne" (offline), not a source failure.
- Many servers do not list players or cap the player sample.
- Tested against a fake server speaking the Server List Ping protocol.
