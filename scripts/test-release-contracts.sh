#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --version <vX.Y.Z> --dist <directory> --platform <os_arch> --evidence <directory>" >&2
  exit 2
}

version="" dist="" platform="" evidence=""
while (($#)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    --platform) platform="${2:-}"; shift 2 ;;
    --evidence) evidence="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "$version" && -n "$dist" && -n "$evidence" ]] || usage
case "$(uname -s)/$(uname -m)" in
  Darwin/x86_64) host_platform=darwin_amd64 ;;
  Darwin/arm64) host_platform=darwin_arm64 ;;
  Linux/x86_64) host_platform=linux_amd64 ;;
  Linux/aarch64|Linux/arm64) host_platform=linux_arm64 ;;
  *) echo "unsupported contract runtime" >&2; exit 1 ;;
esac
[[ "$platform" == "$host_platform" ]] || {
  echo "contract target $platform requires a compatible runtime (host is $host_platform)" >&2
  exit 1
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist="$(cd "$dist" && pwd)"
mkdir -p "$evidence"
evidence="$(cd "$evidence" && pwd)"
# Resolve caches before sandboxing the operator's workstation paths.
go_cache="$(go env GOCACHE)"
go_mod_cache="$(go env GOMODCACHE)"
go_path="$(go env GOPATH)"
scratch="$(mktemp -d "${TMPDIR:-/tmp}/packy-release-contracts.XXXXXX")"
trap 'find "$scratch" -depth -delete' EXIT
export HOME="$scratch/home" XDG_CONFIG_HOME="$scratch/xdg" TMPDIR="$scratch/tmp"
export GOCACHE="$go_cache" GOMODCACHE="$go_mod_cache" GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$TMPDIR" "$scratch/extracted"
cd "$root"
./scripts/validate-release-artifacts.sh --version "$version" --dist "$dist" | tee "$evidence/artifacts.txt"
tar -xzf "$dist/packy_${version}_${platform}.tar.gz" -C "$scratch/extracted"
export PACKY_RELEASE_BINARY="$scratch/extracted/packy" PACKY_RELEASE_VERSION="$version"
"$PACKY_RELEASE_BINARY" version --json > "$evidence/identity.json"
cp "$dist/SHA256SUMS" "$evidence/SHA256SUMS"
# Every platform runs the identical cases, using controlled origin/publication
# transports and isolated candidate workspaces, never production credentials.
GOWORK=off GOFLAGS= GOENV=off go test -json -count=1 -timeout=5m ./internal/ci -run '^TestCatalogReleaseExecutable$' | tee "$evidence/contracts.jsonl"
jq -s -e 'any(.[]; .Action == "pass" and .Test == "TestCatalogReleaseExecutable")' "$evidence/contracts.jsonl" >/dev/null
