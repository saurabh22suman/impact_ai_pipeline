#!/usr/bin/env bash
set -euo pipefail

project_dir="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$project_dir"

if ! find . -name '*.go' -print -quit | grep -q .; then
  exit 0
fi

if ! find . -name '*_test.go' -print -quit | grep -q .; then
  echo "TDD gate failed: at least one Go test file is required." >&2
  exit 2
fi

go test ./...
go test ./... -race
go test ./tests/unit -run='^$' -fuzz=Fuzz -fuzztime=5s

go vet ./...

if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run
fi

if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
fi
