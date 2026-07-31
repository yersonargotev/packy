#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

evidence_dir=""
candidate_sha=""
run_id=""
while (($#)); do
  case "$1" in
    --evidence-dir) evidence_dir="${2:-}"; shift 2 ;;
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [[ -n "$evidence_dir" ]]; then
  if [[ "$evidence_dir" != /* || -e "$evidence_dir" ||
        ! "$candidate_sha" =~ ^[0-9a-f]{40}$ || ! "$run_id" =~ ^[A-Za-z0-9_.:-]+$ ]]; then
    echo "foundation evidence requires a new absolute directory, exact candidate SHA, and safe run ID" >&2
    exit 2
  fi
  if [[ "$(git rev-parse HEAD)" != "$candidate_sha" ]]; then
    echo "foundation candidate SHA does not match tested checkout HEAD" >&2
    exit 1
  fi
  if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
    echo "foundation evidence requires a clean checkout of the candidate SHA" >&2
    exit 1
  fi
  mkdir -p "$evidence_dir"
elif [[ -n "$candidate_sha$run_id" ]]; then
  echo "foundation identity is not accepted without --evidence-dir" >&2
  exit 2
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/packy-vercel-foundation.XXXXXX")"
trap 'rm -rf "$work"' EXIT
capture_dir="$evidence_dir"
if [[ -z "$capture_dir" ]]; then
  capture_dir="$work/evidence"
  candidate_sha="$(git rev-parse HEAD)"
  run_id="local-validation"
  mkdir -p "$capture_dir"
fi

normalize() {
  sed -E '/^go: downloading /d; /^ok[[:space:]]/d; s/ \([0-9.]+s\)$/ (duration)/' | LC_ALL=C sort
}

observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
toolchain_identity="$(go env GOVERSION GOOS GOARCH | paste -sd/ -)"
mkdir -p "$work/seams"
go run ./internal/tools/vercelacceptance --list-foundation >"$work/mappings"
while IFS='|' read -r row kind seam; do
  package="${seam%/*}"
  test="${seam##*/}"
  seam_key="$(printf '%s\0%s\0%s\0%s' "$candidate_sha" "$toolchain_identity" "$package" "$test" | shasum -a 256 | awk '{print $1}')"
  for rerun in first second; do
    normalized="$work/seams/$seam_key.$rerun.txt"
    if [[ ! -f "$normalized" ]]; then
      raw="$work/seams/$seam_key.$rerun.raw"
      if ! go test "$package" -run "^${test}$" -count=1 -v >"$raw" 2>&1; then
        cat "$raw" >&2
        echo "Vercel acceptance foundation failed: $row $kind $package/$test" >&2
        exit 1
      fi
      normalize <"$raw" >"$normalized"
    fi
    {
      printf '@identity\t%s\t%s\t%s\t%s\n' "$candidate_sha" "$run_id" "$observed_at" "$seam"
      printf '@toolchain\t%s\n' "$toolchain_identity"
      cat "$normalized"
    } >"$capture_dir/$row.$kind.$rerun.txt"
  done
  printf 'proof\t%s\t%s\t%s\n' "$row" "$kind" \
    "$(shasum -a 256 "$capture_dir/$row.$kind.first.txt" | awk '{print $1}')" \
    >>"$work/manifest.proofs"
done <"$work/mappings"

go run ./internal/tools/vercelacceptance \
  --foundation-manifest \
  --candidate-sha "$candidate_sha" \
  --run-id "$run_id" \
  --observed-at "$observed_at" \
  <"$work/manifest.proofs" \
  >"$capture_dir/manifest.tsv"
go run ./internal/tools/vercelacceptance \
  --validate-foundation \
  --candidate-sha "$candidate_sha" \
  --run-id "$run_id" \
  --collected-at "$observed_at" \
  --foundation-evidence "$capture_dir"
