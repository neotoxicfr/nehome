# Contributing

## Prerequisites

- **Go 1.27+** (`go.mod` is authoritative)
- **govulncheck** (`go install golang.org/x/vuln/cmd/govulncheck@latest`)
- **A C toolchain** (gcc/clang), only for `go test -race`

```bash
cp config.example.yaml config.yaml    # set auth.disabled: true for a local run
go run ./cmd/nehome -config config.yaml
./scripts/ci.sh                       # what must pass before a pull request
```

## Project conventions

- **Server-rendered, almost no JavaScript.** Tiles are rendered by `html/template` and pushed over
  Server-Sent Events; `static/app.js` only swaps fragments and opens the detail dialog. No build
  step, no npm.
- **The UI is in French** (labels, settings). English documentation lives in `docs/`, French in
  `docs/fr/`; the descriptions shown in the "add a link" form come from `docs/fr/integrations/`.
- **One integration = one file** in `internal/integ/<type>.go`, registered in `integ.go`. Read
  [docs/architecture.md](docs/architecture.md) before writing one. In short:
  - read-only; secrets in headers when the API allows it, otherwise removed from errors;
  - 3 to 5 fields on the tile, everything else in the detail view;
  - poll according to cost (2–5 s for live rates, 30–60 s for state, 5 min for heavy stats);
  - tests with `httptest` and recorded, anonymised responses;
  - run `go run ./cmd/genspec` (the form lists the options and secret roles each type reads);
  - add `docs/integrations/<type>.md` and `docs/fr/integrations/<type>.md`.
- **Layouts are checked** from 320 to 2560 px wide, tiles and detail views: no overlap, no
  horizontal overflow, values and graphics aligned.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/) in English; the
  body explains the scenario.
- **No real installation data** (hostnames, addresses, keys) in code, tests or docs.
