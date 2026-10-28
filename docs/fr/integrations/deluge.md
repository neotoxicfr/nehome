# Deluge

Suit Deluge via son interface web (JSON-RPC) : débits en courbe, torrents en téléchargement ou en partage, connexions, espace libre. Lecture seule.

## Options et secrets
- `url` : adresse de l'interface web (`http://hôte:8112`). Le chemin `/json` est ajouté.
- `insecure_tls` : certificat auto-signé accepté.
- Secret `pass` (obligatoire) : mot de passe de l'interface web (par défaut `deluge`, à changer). Il n'y a pas de nom d'utilisateur, ni de droits réduits.

L'interface web doit être reliée à un démon (Gestionnaire de connexions) ; sinon la tuile signale l'erreur.

```yaml
- id: deluge
  type: deluge
  title: Deluge
  url: http://deluge:8112
  secrets: {pass: deluge_pass}
```

## Champs
Sur la tuile : Réception (courbe), Envoi (courbe), En téléchargement, En partage.
Dans le détail : Connexions entrantes, Connexions, Nœuds DHT, Limites, Espace libre, En erreur, En pause, table Téléchargements en cours, table Envois les plus actifs (avec ratio du torrent).

## Limites
- Pas de ratio global ni de total depuis l'origine : l'interface web ne les fournit pas dans `web.update_ui`.
- Les étiquettes (plugin Label) ne sont pas lues.
- Une session expirée est rouverte automatiquement (un seul essai par cycle de 3 s).
- Testé uniquement contre un faux serveur d'après le code de Deluge, pas contre un vrai Deluge.
