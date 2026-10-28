> Version française : [fr/architecture.md](fr/architecture.md)

# Architecture

nehome is a single Go server: server-side HTML rendering, live updates over SSE, and one small JS file (`internal/web/static/app.js`) that receives the fragments. There is no database; state lives in memory, except `settings.json`.

```
config.yaml -> integ.New -> Source (one goroutine per tile) -> board -> SSE -> browsers
                                                                 ^
                                              settings.json (/settings)
```

## Packages

| Package | Role |
|---|---|
| `internal/config` | reads `config.yaml`, validates the `id`s, reads secrets (one name = one file in `secrets_dir`) |
| `internal/tile` | the contract: `Kind`, `Placement`, `Field`, `Value`, `Source`; French formats (`Num`, `Bytes`, `Rate`, `Dur`...) |
| `internal/integ` | one integration per file, registered in `registry` (type -> constructor + fields) |
| `internal/board` | state of each tile, rendering, broadcast |
| `internal/settings` | `settings.json`: field visibility, placement and order |
| `internal/web` | HTTP routes, templates, security headers |

## Data flow

1. At startup, `integ.New` builds one `Source` per tile. If the constructor fails (missing secret, invalid option), the tile becomes `broken`: it shows the error and still exposes its fields to `/settings`.
2. `board.Start` runs `Source.Run` for each tile. The source calls `emit(values, nil)` to merge values (partial updates allowed) or `emit(nil, err)` to report a failure.
3. On every `emit` or settings change, the tile is rendered to HTML; if the fragment changed, it is sent to subscribers. A field that was never emitted is hidden.
4. `GET /events` (SSE) first sends the state of every tile, then the changed fragments, with a heartbeat every 25 s. The browser replaces the matching tile. `retry: 3000`: automatic reconnection.
5. `GET /`: full page; `GET /tile/{id}`: detail; `GET /settings` and `POST /settings/{id}`: settings.

## Security

- Authentication by proxy header, trusted only from `trusted_proxies`; POSTs require same origin; body limited to 64 KB.
- Strict Content-Security-Policy: `default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'`. No inline script or style: everything comes from `/static/`. Other headers: `X-Content-Type-Options`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, `Cross-Origin-Opener-Policy`, `Permissions-Policy`. HSTS and compression are left to the proxy.
- Integrations are read-only (GET; POST only for a login or when the protocol requires it to read). Secrets never appear in errors: headers rather than URL parameters, otherwise `redact` / `cleanErr`.
- Response bodies are capped (32 MB), with a 15 s timeout per request.

## Writing an integration

One type = one file `internal/integ/<type>.go` plus one line in `registry` (`integ.go`).

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

// Fields works on the zero value: /settings needs it even if the source has not started.
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

Registry: `"monservice": {newMonService, (&monService{}).Fields},`

Ecosystem rules:

- Stable `snake_case` field IDs (they are stored in `settings.json`), French labels.
- 3 to 5 fields on the tile (`OnTile`), the rest in the detail view (`InDetail`): expose everything useful the API provides.
- Polling rate by cost: real time 2 to 5 s, states 30 to 60 s, heavy queries 5 min. A secondary query can run in another goroutine at another pace; its failure clears nothing.
- Main request failing: `emit(nil, err)`. Do not emit a fake empty value: a field that was never emitted is hidden.
- Values formatted with `tile/format.go`; levels `ok`, `warn`, `crit`; tables of at most 15 rows.
- No secret in an error; secrets through explicit roles (`key`, `token`, `user`, `pass`); a non-secret identifier is an option.

### Tests

A file `internal/integ/<type>_test.go` with `httptest.NewServer` returning responses that match the service's official documentation or source code (cite the source in a comment). Dummy values. See `net_test.go` and `arr_test.go` as models. Expected checks:

```
gofmt -l .
go vet ./...
go test -race ./...
```

Finally, add `docs/integrations/<type>.md`: usage, options, secret roles and minimum permissions, YAML example, fields with their default placement, known limits.
