#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_cache="${GOCACHE:-$(go env GOCACHE)}"
go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"
go_path="${GOPATH:-$(go env GOPATH)}"
sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-content-validation.XXXXXX")"
trap 'rm -rf "$sandbox"' EXIT

export HOME="$sandbox/home"
export XDG_CONFIG_HOME="$sandbox/xdg"
export GOCACHE="$go_cache"
export GOMODCACHE="$go_mod_cache"
export GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

cd "$root"
args=(--repository-root "$root")
if [[ $# -gt 1 ]]; then
  echo "usage: ./scripts/validate-pack-content.sh [pack-name-or-directory]" >&2
  exit 2
fi
if [[ $# -eq 1 ]]; then
  args+=(--pack "$1")
fi
go run ./internal/tools/packcontentvalidate "${args[@]}"
