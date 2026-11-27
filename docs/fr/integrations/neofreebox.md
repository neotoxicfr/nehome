# neofreebox

Freebox vue à travers [neofreebox](https://github.com/neotoxicfr/neofreebox) : état de la connexion, débits en courbe, débit maximal de la ligne, appareils connectés, température, Wi-Fi, appels manqués, capteurs. Relevé toutes les 10 s.

## Configuration

- Secret `token` : jeton d'API de neofreebox ; un jeton en lecture seule suffit (`neofreebox token -name nehome -read-only`).
- `url` : adresse de neofreebox, sans chemin ; la tuile lit `/api/v1/summary`. Options : aucune.

```yaml
- id: freebox
  type: neofreebox
  title: Freebox
  url: http://neofreebox:8080
  secrets: {token: neofreebox_key}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| state | connexion établie ou coupée, et son support (FTTH, xDSL…) | tuile |
| down | débit reçu, en courbe | tuile |
| up | débit envoyé, en courbe | tuile |
| devices | appareils connectés, sur le total connu de la box | tuile |
| temp | température la plus haute (orange dès 80 °C, rouge dès 90 °C) | tuile |
| wifi | Wi-Fi activé ou coupé | détail |
| calls | appels manqués non lus | détail |
| line | débit maximal de la ligne, reçu et envoyé | détail |
| uptime | durée depuis le dernier démarrage de la box | détail |
| box | modèle et micrologiciel | détail |
| sensors | chaque température et chaque ventilateur | détail |

## Limites

- `devices`, `wifi` et `calls` restent vides si l'application neofreebox n'a pas ces droits sur la box.
- La réponse ne contient ni adresse, ni MAC, ni nom d'appareil.
