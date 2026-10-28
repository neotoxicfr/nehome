# OPNsense

Passerelles (état, latence, perte), mémoire, charge, interfaces, disques et mises à jour du pare-feu.

## Configuration

```yaml
- id: opnsense
  type: opnsense
  title: OPNsense
  section: Infrastructure
  href: https://opnsense.exemple.org/
  url: https://192.0.2.1
  insecure_tls: true
  secrets: {key: opn_key, secret: opn_secret}
```

## Secrets

| Rôle | Contenu |
|---|---|
| `key`, `secret` | clé et secret d'API d'un utilisateur (Système > Accès > Utilisateurs > Clés d'API) |

Utilisateur dédié, groupe avec uniquement les privilèges : Diagnostics: System Activity, Diagnostics: Interface (aperçu des interfaces), System: Gateways, System: Firmware (lecture). nehome ne fait que des GET.

## Champs

| ID | Contenu | Défaut |
|---|---|---|
| `gateways, ram, load, firmware` | passerelles en ligne, mémoire, charge, mises à jour | tuile |
| `uptime, version, gwtable, ifaces, disks` | uptime, version, table des passerelles, interfaces, jauges de disques | détail |

## Rythme

Toutes les 30 s ; état du micrologiciel toutes les 10 min.

## Limites

Pas de pourcentage de processeur (l'API ne le donne qu'en flux) : la charge le remplace. L'aperçu des interfaces demande OPNsense 24.1 ou plus. Les chemins `/api/diagnostics/system/*` et `/api/core/firmware/status` suivent la documentation mais n'ont pas été vérifiés sur un pare-feu réel ; les champs absents sont simplement masqués.
