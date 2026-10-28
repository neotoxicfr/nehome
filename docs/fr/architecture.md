# Architecture

nehome est un serveur Go unique : rendu HTML côté serveur, mises à jour en direct par SSE, une seule petite feuille JS (`internal/web/static/app.js`) qui reçoit les fragments. Aucune base de données ; l'état est en mémoire, sauf `settings.json`.

```
config.yaml -> integ.New -> Source (une goroutine par tuile) -> board -> SSE -> navigateurs
                                                                 ^
                                              settings.json (/settings)
```

## Paquets

| Paquet | Rôle |
|---|---|
| `internal/config` | lit `config.yaml`, valide les `id`, lit les secrets (un nom = un fichier de `secrets_dir`) |
| `internal/tile` | le contrat : `Kind`, `Placement`, `Field`, `Value`, `Source` ; formats français (`Num`, `Bytes`, `Rate`, `Dur`...) |
| `internal/integ` | une intégration par fichier, enregistrée dans `registry` (type -> constructeur + champs) |
| `internal/board` | état de chaque tuile, rendu, diffusion |
| `internal/settings` | `settings.json` : visibilité, emplacement et ordre des champs |
| `internal/web` | routes HTTP, gabarits, en-têtes de sécurité |

## Flux des données

1. Au démarrage, `integ.New` construit une `Source` par tuile. Si le constructeur échoue (secret absent, option invalide), la tuile devient `broken` : elle affiche l'erreur et expose quand même ses champs pour `/settings`.
2. `board.Start` lance `Source.Run` pour chaque tuile. La source appelle `emit(valeurs, nil)` pour fusionner des valeurs (partielles possibles) ou `emit(nil, err)` pour signaler une panne.
3. À chaque `emit` ou changement de réglage, la tuile est rendue en HTML ; si le fragment a changé, il est envoyé aux abonnés. Un champ jamais émis est masqué.
4. `GET /events` (SSE) envoie d'abord l'état de toutes les tuiles, puis les fragments modifiés, avec un battement toutes les 25 s. Le navigateur remplace la tuile correspondante. `retry: 3000` : reconnexion automatique.
5. `GET /` : page complète ; `GET /tile/{id}` : détail ; `GET /settings` et `POST /settings/{id}` : réglages.

## Sécurité

- Authentification par en-tête de proxy, crue seulement depuis `trusted_proxies` ; les POST exigent la même origine ; corps limité à 64 Ko.
- Content-Security-Policy stricte : `default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'`. Pas de script ni de style en ligne : tout vient de `/static/`. Autres en-têtes : `X-Content-Type-Options`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, `Cross-Origin-Opener-Policy`, `Permissions-Policy`. HSTS et compression sont laissés au proxy.
- Les intégrations sont en lecture seule (GET ; POST seulement pour un login ou quand le protocole l'impose pour lire). Les secrets ne figurent jamais dans les erreurs : en-têtes plutôt que paramètres d'URL, sinon `redact` / `cleanErr`.
- Corps de réponse bornés (32 Mo), délai de 15 s par requête.

## Écrire une intégration

Un type = un fichier `internal/integ/<type>.go` plus une ligne dans `registry` (`integ.go`).

```go
type monService struct {
	url, key string
	hc       *http.Client
}

func newMonService(t config.Tile, secret func(role string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &monService{url: strings.TrimRight(t.URL, "/"), key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

// Fields marche sur la valeur zéro : /settings en a besoin même si la source n'a pas démarré.
func (s *monService) Fields() []tile.Field {
	return []tile.Field{
		tile.F("jobs", "Tâches", tile.Stat, tile.OnTile),
		tile.F("detail", "Détail", tile.Table, tile.InDetail),
	}
}

func (s *monService) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 30*time.Second, func() {
		var r struct{ Jobs int }
		err := getJSON(ctx, s.hc, s.url+"/api/jobs", &r, func(req *http.Request) { req.Header.Set("X-Api-Key", s.key) })
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			emit(nil, err)
			return
		}
		emit(tile.Values{"jobs": {Text: tile.Num(float64(r.Jobs))}}, nil)
	})
}
```

Registry : `"monservice": {newMonService, (&monService{}).Fields},`

Règles de l'écosystème :

- Identifiants de champ stables en `snake_case` (ils sont enregistrés dans `settings.json`), libellés en français.
- 3 à 5 champs sur la tuile (`OnTile`), le reste en détail (`InDetail`) : exposer tout ce que l'API donne d'utile.
- Fréquence selon le coût : temps réel 2 à 5 s, états 30 à 60 s, requêtes lourdes 5 min. Une requête secondaire peut tourner dans une autre goroutine à un autre rythme ; sa défaillance n'efface rien.
- Requête principale en échec : `emit(nil, err)`. N'émettez pas de valeur vide factice : un champ jamais émis est masqué.
- Valeurs mises en forme avec `tile/format.go` ; niveaux `ok`, `warn`, `crit` ; tables de 15 lignes au plus.
- Aucun secret dans une erreur ; secrets par rôles explicites (`key`, `token`, `user`, `pass`) ; un identifiant non secret est une option.

### Tests

Un fichier `internal/integ/<type>_test.go` avec `httptest.NewServer` qui renvoie des réponses conformes à la documentation officielle ou au code source du service (citez la source en commentaire). Valeurs factices. Voir `net_test.go` et `arr_test.go` comme modèles. Vérifications attendues :

```
gofmt -l .
go vet ./...
go test -race ./...
```

Ajoutez enfin `docs/integrations/<type>.md` : usage, options, rôles de secrets et droits minimaux, exemple YAML, champs avec placement par défaut, limites connues.
