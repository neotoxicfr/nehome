# Minecraft (Java)

État d'un serveur Minecraft Java par le protocole Server List Ping en TCP natif : en ligne, version, joueurs, message du jour, latence. Relevé toutes les 30 s.

## Configuration

- Aucun secret.
- Options : `host` (sinon l'hôte de `url`), `port` (25565 par défaut).

```yaml
- id: minecraft
  type: minecraft
  title: Minecraft
  options: {host: mc.example.org, port: "25565"}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| online | État (En ligne / Hors ligne) | tuile |
| players | Joueurs connectés / max | tuile |
| version | Version | tuile |
| latency | Latence (aller-retour du ping) | détail |
| motd | Message du jour (codes § retirés) | détail |
| player_list | Joueurs de l'échantillon | détail |
| history | Joueurs connectés (historique) | détail |

## Limites

- Un serveur injoignable s'affiche « Hors ligne » (pas une panne de la source).
- Beaucoup de serveurs ne listent pas ou plafonnent l'échantillon de joueurs.
- Testé contre un faux serveur qui parle le protocole Server List Ping.
