#!/usr/bin/env bash
#
# Local mirror of .github/workflows/ci.yml — runs the same checks CI runs, to
# catch failures before pushing.
#
#   scripts/check.sh         run all checks (read-only; fails if formatting is needed)
#   scripts/check.sh --fix   auto-format (goimports + gofumpt) instead of just checking
set -euo pipefail

# Keep in sync with .github/workflows/ci.yml
GOFUMPT_VERSION="v0.10.0"
GOIMPORTS_VERSION="latest"

cd "$(dirname "$0")/.."

fix=0
[ "${1:-}" = "--fix" ] && fix=1

echo "==> build"
go build ./...

echo "==> vet"
go vet ./...

echo "==> test (race detector)"
go test -race -shuffle=on ./...

if [ "$fix" -eq 1 ]; then
    echo "==> format (write)"
    go run "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}" -w .
    go run "mvdan.cc/gofumpt@${GOFUMPT_VERSION}" -w .
    echo "Formatted."
else
    echo "==> format check"
    unformatted="$(
        go run "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}" -l .
        go run "mvdan.cc/gofumpt@${GOFUMPT_VERSION}" -l .
    )"
    if [ -n "$(printf '%s' "$unformatted" | tr -d '[:space:]')" ]; then
        echo "These files are not formatted (run: scripts/check.sh --fix):" >&2
        printf '%s\n' "$unformatted" | sort -u >&2
        exit 1
    fi
    echo "Formatting OK."
fi

echo "All checks passed."
