#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

vercel_evidence_dir=""
candidate_sha=""
run_id=""
while (($#)); do
  case "$1" in
    --vercel-foundation-evidence-dir) vercel_evidence_dir="${2:-}"; shift 2 ;;
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
vercel_args=()
if [[ -n "$vercel_evidence_dir$candidate_sha$run_id" ]]; then
  if [[ -z "$vercel_evidence_dir" || -z "$candidate_sha" || -z "$run_id" ]]; then
    echo "Vercel foundation evidence requires --vercel-foundation-evidence-dir, --candidate-sha, and --run-id together" >&2
    exit 2
  fi
  vercel_args=(
    --evidence-dir "$vercel_evidence_dir"
    --candidate-sha "$candidate_sha"
    --run-id "$run_id"
  )
fi

# Keep this list explicit. A new Packy-owned package must be deliberately added
# here before CI or the synchronization publisher can load or execute it.
readonly packages=(
  ./cmd/packy
  ./internal/addyacceptance
  ./internal/bootstrap
  ./internal/bundletransaction
  ./internal/capabilitypack
  ./internal/ci
  ./internal/cli
  ./internal/claudesmoke
  ./internal/codex
  ./internal/codexsmoke
  ./internal/corelifecycle
  ./internal/engrambin
  ./internal/governanceauth
  ./internal/governancedrift
  ./internal/localprojection
  ./internal/opencode
  ./internal/opencodesmoke
  ./internal/ownedcontainer
  ./internal/packclassification
  ./internal/packsync
  ./internal/packsync/githubsource
  ./internal/packsyncworkflow
  ./internal/prompt
  ./internal/release
  ./internal/setuphealth
  ./internal/skillbundle
  ./internal/tools/addypromotiongate
  ./internal/tools/claudesmoke
  ./internal/tools/claudevercelsmoke
  ./internal/tools/codexsmoke
  ./internal/tools/opencodesmoke
  ./internal/tools/packcontentvalidate
  ./internal/tools/governanceauth
  ./internal/tools/governancedrift
  ./internal/tools/syncpacksource
  ./internal/tools/vercelacceptance
  ./internal/vercelacceptance
  ./internal/version
  ./internal/workstation
)

# Derive formatting paths and the build subset from the one package authority.
# The glob below is intentionally non-recursive. Test-only contract packages
# remain in vet/test/race but have no production archive for `go build` to emit.
go_dirs=()
build_packages=()
race_packages=()
for package in "${packages[@]}"; do
  go_dirs+=("${package#./}")
  case "$package" in
    ./internal/ci | ./internal/release) ;;
    *) build_packages+=("$package") ;;
  esac
  # Release is a test-only subprocess, cross-platform, and package-install
  # integration package. Its child commands are not race-instrumented, so the
  # ordinary exhaustive test phase covers it while the race phase excludes it.
  case "$package" in
    ./internal/release) ;;
    *) race_packages+=("$package") ;;
  esac
done

# Tests that exercise workstation behavior must never inherit the operator's
# real configuration roots. Preserve only Go's caches across the sandbox.
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

# The conditional expansion keeps the no-argument path compatible with the
# system Bash 3.2 used by supported macOS validation.
./scripts/validate-vercel-acceptance.sh ${vercel_args[@]+"${vercel_args[@]}"}
./scripts/validate-addy-acceptance.sh

shopt -s nullglob
go_files=()
for dir in "${go_dirs[@]}"; do
  files=("$root/$dir"/*.go)
  if ((${#files[@]} == 0)); then
    echo "allowlisted Go directory has no Go files: $dir" >&2
    exit 1
  fi
  go_files+=("${files[@]}")
done

echo "==> formatting"
unformatted="$(gofmt -l "${go_files[@]}")"
if [[ -n "$unformatted" ]]; then
  echo "These Packy-owned files are not gofmt-clean:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> build"
go build "${build_packages[@]}"

echo "==> vet"
go vet "${packages[@]}"

echo "==> tests"
go test "${packages[@]}"

echo "==> race"
go test -race -timeout 10m "${race_packages[@]}"
