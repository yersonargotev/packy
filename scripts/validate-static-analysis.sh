#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.3.1 run ./...
