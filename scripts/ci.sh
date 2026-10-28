#!/usr/bin/env bash
# Checks run by GitHub Actions and before a pull request: format, generated spec, vet, tests, vulnerabilities, static build.
set -euo pipefail
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "gofmt à appliquer :" >&2
	echo "$unformatted" >&2
	exit 1
fi
before=$(cat internal/integ/spec_gen.go)
go run ./cmd/genspec
[ "$before" = "$(cat internal/integ/spec_gen.go)" ] || { echo "spec_gen.go était périmé : il vient d'être régénéré, relancer la CI puis committer" >&2; exit 1; }
go vet ./...
go test -race ./...
govulncheck ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/nehome ./cmd/nehome
echo "ci ok"
