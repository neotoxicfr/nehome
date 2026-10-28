# wg-easy

Clients WireGuard : connectés, dernier handshake, trafic. Versions 14 et 15. Relevé toutes les 15 s.

## Configuration

- v14 : secret `pass` seul (mot de passe de l'interface, envoyé en en-tête `Authorization`).
- v15 : secrets `user` et `pass` (authentification Basic).
- Le choix de version suit la présence du rôle `user`. Options : aucune.

```yaml
# v14
- id: wg
  type: wgeasy
  title: WireGuard
  url: http://wg-easy:51821
  secrets: {pass: wg_pass}
# v15 : ajouter user
#  secrets: {user: wg_user, pass: wg_pass}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| online | Clients connectés (handshake < 3 min) | tuile |
| rx | Reçu, cumul | tuile |
| tx | Envoyé, cumul | tuile |
| clients | Nombre de clients | détail |
| client_list | Clients : adresse, état, handshake, trafic | détail |

## Limites

- Les compteurs sont cumulés depuis le démarrage de l'interface WireGuard, pas des débits instantanés.
- Les clés et configurations des clients ne sont jamais lues.
- Non éprouvé sur une vraie instance (schémas v14/v15 d'après le code source).
