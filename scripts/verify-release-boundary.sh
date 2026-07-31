#!/usr/bin/env bash
set -euo pipefail

boundary=
repository=
tag=
release_commit=
verifier=
ref_output=
candidate=
provenance=
state=
mode=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --boundary) boundary="${2:-}"; shift 2 ;;
    --repository) repository="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --release-commit) release_commit="${2:-}"; shift 2 ;;
    --verifier) verifier="${2:-}"; shift 2 ;;
    --ref-output) ref_output="${2:-}"; shift 2 ;;
    --candidate) candidate="${2:-}"; shift 2 ;;
    --provenance) provenance="${2:-}"; shift 2 ;;
    --state) state="${2:-}"; shift 2 ;;
    --mode) mode="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ -n "$boundary" && "$repository" == */* && "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ &&
   "$release_commit" =~ ^[0-9a-f]{40}$ && -x "$verifier" && -n "$ref_output" ]] || {
  echo 'boundary, repository, valid v0 tag, release commit, executable verifier, and ref output are required' >&2
  exit 2
}
if [[ -n "$candidate$provenance$state$mode" ]]; then
  [[ -n "$candidate" && -n "$provenance" && -n "$state" && "$mode" =~ ^(draft|published)$ ]] || {
    echo 'candidate, provenance, state, and draft or published mode must be supplied together' >&2
    exit 2
  }
fi

resolve_ref_commit() {
  local ref="$1" object object_type object_sha hops=0
  object="$(gh api "repos/$repository/git/ref/$ref" --jq '[.object.type,.object.sha]|@tsv')"
  IFS=$'\t' read -r object_type object_sha <<< "$object"
  while [[ "$object_type" == tag && "$hops" -lt 8 ]]; do
    object="$(gh api "repos/$repository/git/tags/$object_sha" --jq '[.object.type,.object.sha]|@tsv')"
    IFS=$'\t' read -r object_type object_sha <<< "$object"
    hops=$((hops + 1))
  done
  if [[ "$object_type" == commit && "$object_sha" =~ ^[0-9a-f]{40}$ ]]; then
    printf '%s\n' "$object_sha"
  else
    printf '<unresolved>'
  fi
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! ref_error="$("$script_dir/verify-release-ref-state.sh" \
  --repository "$repository" --tag "$tag" --release-commit "$release_commit" \
  --verifier "$verifier" --output "$ref_output" 2>&1)"; then
  observed_tag="$(resolve_ref_commit "tags/$tag" || printf '<unresolved>')"
  observed_main="$(resolve_ref_commit heads/main || printf '<unresolved>')"
  observed_ancestry="$(gh api "repos/$repository/compare/$release_commit...$observed_main" --jq .status 2>/dev/null || printf '<unresolved>')"
  echo "$boundary denied: expected release_commit=$release_commit tag_commit=$release_commit in protected main; observed tag_commit=$observed_tag main_commit=$observed_main ancestry=$observed_ancestry; $ref_error" >&2
  exit 1
fi

if [[ -n "$state" ]]; then
  if ! state_error="$("$verifier" verify-state --candidate "$candidate" --provenance "$provenance" \
    --state "$state" --mode "$mode" 2>&1)"; then
    expected_candidate="$(jq -r '.id // "<unresolved>"' "$candidate" 2>/dev/null || printf '<unresolved>')"
    observed_candidate="$(jq -r '.candidate_id // "<missing>"' "$state" 2>/dev/null || printf '<unresolved>')"
    observed_target="$(jq -r '.target_commit // "<missing>"' "$state" 2>/dev/null || printf '<unresolved>')"
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed candidate_id=$observed_candidate target_commit=$observed_target; $state_error" >&2
    exit 1
  fi
fi
