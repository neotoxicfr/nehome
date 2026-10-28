# NZBGet

Suit le client Usenet NZBGet : débit en courbe, file, reste à télécharger, historique, espace libre. Lecture seule (méthodes JSON-RPC `status`, `listgroups`, `history`).

## Options et secrets
- `url` : adresse de NZBGet (`http://hôte:6789`). Le chemin `/jsonrpc` est ajouté.
- `insecure_tls` : certificat auto-signé accepté.
- Secrets `user` et `pass` : facultatifs, identifiants de contrôle (`ControlUsername` / `ControlPassword`, `nzbget` / `tegbzn6789` par défaut). Le compte « Restricted » suffit, sinon « Control » : nehome ne fait aucune action.

```yaml
- id: nzbget
  type: nzbget
  title: NZBGet
  url: http://nzbget:6789
  secrets: {user: nzbget_user, pass: nzbget_pass}
```

## Champs
Sur la tuile : Réception (courbe), En file, Reste à télécharger (avec durée estimée), État.
Dans le détail : Espace libre, Limite de débit, Téléchargé depuis le démarrage, Post-traitements, Fils actifs, Cache d'articles, En marche depuis, Échecs dans l'historique, tables Téléchargements en cours et Historique récent (15 entrées).

## Limites
- État et file toutes les 3 s, historique chaque minute (il peut être long : « Échecs » compte tout l'historique).
- Pas de volumes par jour ou par serveur (méthode `servervolumes` non lue).
- Testé uniquement contre un faux serveur conforme à la documentation de l'API, pas contre un vrai NZBGet.
