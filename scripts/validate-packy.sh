#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

ci=false
if (($#)); then
  if [[ "$1" != "--ci" || $# -ne 1 ]]; then
    echo "usage: $0 [--ci]" >&2
    exit 2
  fi
  ci=true
fi

# Tests that resolve workstation paths must never inherit the operator's real
# configuration roots. Preserve only Go's caches across the sandbox.
go_cache="${GOCACHE:-$(go env GOCACHE)}"
go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"
go_path="${GOPATH:-$(go env GOPATH)}"
validation_home="${PACKY_VALIDATION_HOME:-}"
validation_config_home="${PACKY_VALIDATION_CONFIG_HOME:-}"
sandbox=""
if [[ -n "$validation_home" || -n "$validation_config_home" ]]; then
  if [[ -z "$validation_home" || -z "$validation_config_home" || "$validation_home" != /* || "$validation_config_home" != /* ]]; then
    echo "PACKY_VALIDATION_HOME and PACKY_VALIDATION_CONFIG_HOME must both be absolute when supplied" >&2
    exit 1
  fi
  export HOME="$validation_home"
  export XDG_CONFIG_HOME="$validation_config_home"
else
  sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-validation.XXXXXX")"
  export HOME="$sandbox/home"
  export XDG_CONFIG_HOME="$sandbox/xdg"
fi
cleanup() { [[ -z "$sandbox" ]] || rm -rf "$sandbox"; }
trap cleanup EXIT
export GOCACHE="$go_cache"
export GOMODCACHE="$go_mod_cache"
export GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

go_files=()
while IFS= read -r file; do
  [[ -f "$file" && ! -L "$file" ]] && go_files+=("$file")
done < <(git ls-files --cached --others --exclude-standard -- '*.go')

echo "==> formatting"
unformatted="$(gofmt -l "${go_files[@]}")"
if [[ -n "$unformatted" ]]; then
  echo "These Go files are not gofmt-clean:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> vet"
go vet ./...

echo "==> tests"
if [[ "$ci" == true ]]; then
  ci_packages=()
  while IFS= read -r package; do
    case "$package" in
      github.com/yersonargotev/packy/internal/capabilitypack) ;;
      *) ci_packages+=("$package") ;;
    esac
  done < <(go list ./...)
  go test "${ci_packages[@]}" &
  tests_pid=$!
  echo "==> race"
  go test -race ./internal/capabilitypack &
  race_pid=$!
  tests_status=0
  race_status=0
  wait "$tests_pid" || tests_status=$?
  wait "$race_pid" || race_status=$?
  ((tests_status == 0 && race_status == 0))
else
  general_packages=()
  while IFS= read -r package; do
    case "$package" in
      github.com/yersonargotev/packy/internal/cli | github.com/yersonargotev/packy/internal/release) ;;
      *) general_packages+=("$package") ;;
    esac
  done < <(go list ./...)
  go test "${general_packages[@]}"
fi
