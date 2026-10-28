# Ports TCP

Vérifie que des ports TCP acceptent les connexions (connexion puis fermeture immédiate, rien n'est envoyé) et mesure le temps d'établissement.

## Configuration

```yaml
- id: ports
  type: tcp
  title: Ports
  section: Réseau
  options:
    targets: "SSH=192.0.2.10:22, Postgres=db:5432, IPv6=[2001:db8::1]:443, 192.0.2.1:53"
```

`targets` : liste `nom=hôte:port` séparée par des virgules ; sans nom, l'adresse sert de nom.

## Secrets

Aucun.

## Champs

| ID | Libellé | Défaut |
|---|---|---|
| `down`, `up`, `latency` | En panne (avec les noms), Joignables (« 4 / 5 »), Latence moyenne (et la plus lente) | tuile |
| `targets` | table : nom, adresse, état (ouvert, refusé, délai dépassé, nom inconnu, injoignable), latence | détail |

## Rythme

Toutes les 30 s, cibles en parallèle, 3 s de délai par cible.

## Limites

- La latence inclut la résolution DNS du nom.
- Un port ouvert ne dit rien de la santé du service derrière ; la table affiche 15 cibles au plus (pannes en premier).
