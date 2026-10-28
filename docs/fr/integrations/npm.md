# Nginx Proxy Manager

Hôtes proxy actifs ou désactivés et certificats proches de l'expiration. Relevé toutes les 5 min.

## Configuration

- Secrets `user` (e-mail) et `pass` : un compte dédié avec le droit « voir » sur hôtes proxy, redirections, flux, hôtes 404 et certificats. Le jeton est obtenu par `POST /api/tokens` puis renouvelé sur 401.
- Options : aucune. `url` = interface de gestion (port 81).

```yaml
- id: npm
  type: npm
  title: Proxy Manager
  url: http://npm:81
  secrets: {user: npm_user, pass: npm_pass}
```

## Champs

| Champ | Libellé | Placement |
|---|---|---|
| proxy | Hôtes proxy (actifs, désactivés) | tuile |
| certs | Certificats | tuile |
| expiry | Prochaine expiration (orange < 21 j, rouge < 7 j) | tuile |
| other | Redirections / flux / hôtes 404 | détail |
| cert_list | Certificats par échéance | détail |
| host_list | Hôtes désactivés | détail |
| version | Version | détail |

## Limites

- Seuls des GET sont émis, hors la connexion.
- Non éprouvé sur une vraie instance.
