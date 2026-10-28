## What and why

<!-- The change, and the scenario that motivated it. -->

## Checks

- [ ] `./scripts/ci.sh` passes (gofmt, `go run ./cmd/genspec`, vet, `go test -race`, govulncheck, build)
- [ ] New integration: read-only, secrets only in headers or redacted, tests against recorded responses, page in `docs/integrations/` and `docs/fr/integrations/`
- [ ] UI change: checked from 320 to 2560 px wide, tiles and detail view, no overlap or overflow
- [ ] No hostname, address or secret of a real installation in code, tests or docs
