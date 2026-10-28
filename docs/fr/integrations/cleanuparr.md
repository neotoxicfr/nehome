# Cleanuparr

Strikes (téléchargements bloqués, imports ratés), éléments retirés, prochain nettoyage, santé des clients de téléchargement et tâches planifiées de Cleanuparr.

```yaml
- {id: cleanuparr, type: cleanuparr, title: Cleanuparr, url: "http://cleanuparr:11011", secrets: {key: cleanuparr_key}}
```

| Rôle | Contenu |
|---|---|
| `key` | clé API (Cleanuparr → Compte → Clé API) ; le login par mot de passe exige la 2FA, d'où la clé |

Sur la tuile : téléchargements en strike, retirés, prochain nettoyage, clients de téléchargement. Dans le détail : strikes au total, recherches de remplacement, tâches en échec, liste des strikes, derniers événements, tâches planifiées. Stats et strikes chaque minute, événements et tâches toutes les 5 min. Lecture seule.

## Action

« Lancer une tâche maintenant » : `POST /api/jobs/{type}/trigger`, exécution unique d'une tâche planifiée (le planning ne change pas).
