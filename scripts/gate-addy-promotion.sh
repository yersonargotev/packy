#!/usr/bin/env bash

set -euo pipefail

if (($# < 2 || $# > 5)); then
  echo "usage: $0 <base-ref> <head-ref> [same-run-promotion-evidence | --generate <production-qualification> <output-evidence>]" >&2
  exit 2
fi
generate=false
qualification=
generated_evidence=
if [[ "${3:-}" == "--generate" ]]; then
  (($# == 5)) || { echo "--generate requires qualification and output paths" >&2; exit 2; }
  generate=true
  qualification="$4"
  generated_evidence="$5"
elif (($# > 3)); then
  echo "unexpected arguments" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

base_sha="$(git rev-parse --verify "$1^{commit}")"
head_sha="$(git rev-parse --verify "$2^{commit}")"
promotion_change=false
foundation_change=false
base_has_promotion_gate=false
if git show "$base_sha:.github/workflows/ci.yml" 2>/dev/null | grep -Fq 'addy-promotion-gate:'; then
  base_has_promotion_gate=true
fi

while IFS= read -r path; do
  case "$path" in
    bundle/packs/addy/pack.json | bundle/sources/addy.lock.json | bundle/history/addy/*)
      promotion_change=true
      ;;
    .github/workflows/ci.yml | internal/addyacceptance/* | internal/capabilitypack/* | internal/ci/* | internal/claudecode/* | internal/tools/addypromotiongate/* | scripts/gate-addy-promotion.sh | scripts/validate-addy-acceptance.sh)
      if [[ "$base_has_promotion_gate" == true ]]; then
        foundation_change=true
      fi
      ;;
  esac
done < <(git diff --name-only "$base_sha" "$head_sha" --)

catalog_has_addy() {
  git show "$1:internal/capabilitypack/catalog.go" |
    awk '/^var initialCatalog = / { catalog=1 } catalog { print } catalog && /^}/ { exit }' |
    grep -Eq 'ID:[[:space:]]*"addy"([,}])'
}

if [[ "$promotion_change" == false ]] && ! catalog_has_addy "$base_sha" && catalog_has_addy "$head_sha"; then
  promotion_change=true
fi
if [[ "$promotion_change" == true ]]; then
  foundation_change=false
  if [[ "$generate" == true ]]; then
    [[ -f "$qualification" && ! -L "$qualification" ]] || {
      echo "production qualification must be a regular same-run artifact" >&2
      exit 1
    }
    [[ "$generated_evidence" = /* && ! -e "$generated_evidence" ]] || {
      echo "generated promotion evidence must be a new absolute out-of-tree path" >&2
      exit 1
    }
    work="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/addy-promotion-generation.XXXXXX")"
    trap 'rm -rf "$work"' EXIT
    ./scripts/validate-addy-acceptance.sh >"$work/acceptance.log" 2>&1
    [[ -s "$work/acceptance.log" ]] || { echo "acceptance validation produced no log" >&2; exit 1; }
    ./scripts/gate-governance-drift.sh \
      --boundary promotion \
      --repo "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}" \
      --ref refs/heads/main \
      --commit "${GITHUB_SHA:?GITHUB_SHA is required}" \
      --workflow .github/workflows/ci.yml \
      --output-dir "$work/governance"
  fi
elif [[ "$foundation_change" == true ]]; then
  ./scripts/validate-addy-acceptance.sh
fi

workflow=.github/workflows/ci.yml
if command -v sha256sum >/dev/null 2>&1; then
  workflow_digest="$(sha256sum "$workflow" | awk '{print $1}')"
else
  workflow_digest="$(shasum -a 256 "$workflow" | awk '{print $1}')"
fi

args=(
  --promotion-change="$promotion_change"
  --foundation-change="$foundation_change"
  --repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  --pull-request="${GITHUB_PR_NUMBER:?GITHUB_PR_NUMBER is required}"
  --base-sha="$base_sha"
  --head-sha="$head_sha"
  --evaluated-merge-sha="${GITHUB_SHA:?GITHUB_SHA is required}"
  --workflow="$workflow"
  --workflow-digest="$workflow_digest"
  --run-id="${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
)
if [[ "$promotion_change" == true && "$generate" == true ]]; then
  go run ./internal/tools/addypromotiongate \
    "${args[@]}" \
    --generate \
    --qualification="$qualification" \
    --governance-evaluation="$work/governance/evaluation.json" \
    --governance-gate="$work/governance/gate.json" \
    --acceptance-log="$work/acceptance.log" \
    --output="$generated_evidence"
  args+=(--evidence="$generated_evidence")
fi
if [[ "$promotion_change" == true && -n "${3:-}" ]]; then
  if [[ "$generate" == true ]]; then
    :
  elif [[ -f "$3" && ! -L "$3" ]]; then
    args+=(--evidence="$3")
  else
    echo "promotion evidence must be a regular same-run artifact" >&2
    exit 1
  fi
elif [[ "$promotion_change" == false && -n "${3:-}" && "$generate" == false ]]; then
  echo "candidate evidence is not accepted for a non-promotion change" >&2
  exit 1
fi

go run ./internal/tools/addypromotiongate "${args[@]}"
