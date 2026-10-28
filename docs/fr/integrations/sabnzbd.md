# SABnzbd

Suit le client Usenet SABnzbd : débit en courbe, file, reste à télécharger, historique, volumes par serveur. Lecture seule (modes `queue`, `history`, `server_stats`).

## Options et secrets
- `url` : adresse de SABnzbd (`http://hôte:8080`, avec le préfixe éventuel de `url_base`).
- `insecure_tls` : certificat auto-signé accepté.
- Secret `key` (obligatoire) : la clé API (Configuration, Général). Utilisez la **clé API** et non la clé NZB : la seconde n'ouvre pas ces modes. SABnzbd n'a pas de droits par clé. La clé voyage dans l'adresse de la requête (seul moyen accepté) ; nehome l'expurge de toutes les erreurs affichées.

```yaml
- id: sabnzbd
  type: sabnzbd
  title: SABnzbd
  url: http://sabnzbd:8080
  secrets: {key: sabnzbd_key}
```

## Champs
Sur la tuile : Réception (courbe), En file, Reste à télécharger (avec durée estimée), État.
Dans le détail : Espace libre (dossier temporaire, terminés en sous-titre), Limite de débit, Avertissements, Échecs récents, Aujourd'hui / Cette semaine / Ce mois / Total depuis l'origine, Version, tables Téléchargements en cours, Historique récent (15 entrées) et Serveurs Usenet.

## Limites
- File toutes les 3 s, historique chaque minute, statistiques des serveurs toutes les 5 min.
- « Échecs récents » ne compte que les 15 dernières entrées de l'historique.
- Testé uniquement contre un faux serveur conforme à la documentation de l'API, pas contre un vrai SABnzbd.
