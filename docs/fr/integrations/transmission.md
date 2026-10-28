# Transmission

Suit le client BitTorrent Transmission (démon ou application) : débits en courbe, torrents actifs, en partage, espace libre. Lecture seule.

## Options et secrets
- `url` : adresse de Transmission (`http://hôte:9091`). Le chemin `/transmission/rpc` est ajouté si absent.
- `insecure_tls` : certificat auto-signé accepté.
- Secrets `user` et `pass` : facultatifs, à donner si `rpc-authentication-required` est actif. Un compte de lecture n'existe pas dans Transmission : le compte RPC a tous les droits, nehome n'envoie que des requêtes de lecture.

```yaml
- id: transmission
  type: transmission
  title: Transmission
  url: http://transmission:9091
  secrets: {user: transmission_user, pass: transmission_pass}
```

## Champs
Sur la tuile : Réception (courbe), Envoi (courbe), En téléchargement, En partage.
Dans le détail : Ratio global, Limite alternative, Limites, Espace libre, Total depuis l'origine, Version, En erreur, En vérification, table Téléchargements en cours, table Envois les plus actifs.

## Limites
- Le ratio global est calculé sur les compteurs cumulés (envoyé / reçu).
- La liste complète des torrents est relue toutes les 3 s ; sur des milliers de torrents, c'est la requête la plus lourde.
- Version, limites et espace libre sont rafraîchis chaque minute. L'espace libre exige Transmission 4.0 ou plus (méthode `free-space`).
- Testé uniquement contre un faux serveur conforme à la spécification RPC, pas contre un vrai Transmission.
