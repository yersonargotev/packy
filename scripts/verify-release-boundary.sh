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
state_output=
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
    --state-output) state_output="${2:-}"; shift 2 ;;
    --mode) mode="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ -n "$boundary" && "$repository" == */* && "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ &&
   "$release_commit" =~ ^[0-9a-f]{40}$ && -x "$verifier" && -n "$ref_output" ]] || {
  echo 'boundary, repository, valid v0 tag, release commit, executable verifier, and ref output are required' >&2
  exit 2
}
if [[ -n "$candidate$provenance$state_output$mode" ]]; then
  [[ -n "$candidate" && -n "$provenance" && -n "$state_output" && "$mode" =~ ^(current|draft|published)$ ]] || {
    echo 'candidate, provenance, state output, and current, draft, or published mode must be supplied together' >&2
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

if [[ -n "$state_output" ]]; then
  owner="${repository%%/*}"
  repository_name="${repository#*/}"
  # GraphQL variables expand server-side.
  # shellcheck disable=SC2016
  expected_candidate="$(jq -r '.id // "<unresolved>"' "$candidate" 2>/dev/null || printf '<unresolved>')"
  if ! release_id="$(gh api graphql \
    -f query='query($owner:String!,$repository:String!,$tag:String!){repository(owner:$owner,name:$repository){release(tagName:$tag){id}}}' \
    -f owner="$owner" -f repository="$repository_name" -f tag="$tag" \
    --jq '.data.repository.release.id // ""')"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=unreadable" >&2
    exit 1
  fi
  if [[ -z "$release_id" ]]; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=missing" >&2
    exit 1
  fi
  observed_release="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary.XXXXXX")"
  observed_body="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-body.XXXXXX")"
  observed_metadata="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-metadata.XXXXXX")"
  trap 'rm -f "$observed_release" "$observed_body" "$observed_metadata"' EXIT
  if ! gh release view "$tag" --repo "$repository" --json tagName,isDraft,body,assets > "$observed_release"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=unreadable" >&2
    exit 1
  fi
  jq -j .body "$observed_release" > "$observed_body"
  awk '/^<!-- packy-release-metadata$/{capture=1;next}/^-->$/{capture=0}capture' \
    "$observed_body" > "$observed_metadata"
  if ! jq --slurpfile metadata "$observed_metadata" \
    --slurpfile candidate "$candidate" \
    --arg repository "$repository" \
    '{candidate_id:($metadata[0].candidate_id // ""),provenance:($metadata[0].provenance // {}),
      version:.tagName,repository:$repository,ref:$candidate[0].ref,
      target_commit:($metadata[0].target_commit // ""),workflow:$candidate[0].workflow,
      workflow_sha:$candidate[0].workflow_sha,
      release_notes_sha256:$candidate[0].release_notes_sha256,draft:.isDraft,
      assets:[.assets[]|select(.name!="attestation.bundle.jsonl")|{name,digest}]}' \
    "$observed_release" > "$state_output"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release metadata=invalid" >&2
    exit 1
  fi
  verify_mode="$mode"
  if [[ "$verify_mode" == current ]]; then
    if [[ "$(jq -r .draft "$state_output")" == true ]]; then verify_mode=draft; else verify_mode=published; fi
  fi
  if ! state_error="$("$verifier" verify-state --candidate "$candidate" --provenance "$provenance" \
    --state "$state_output" --mode "$verify_mode" 2>&1)"; then
    observed_candidate="$(jq -r '.candidate_id // "<missing>"' "$state_output" 2>/dev/null || printf '<unresolved>')"
    observed_target="$(jq -r '.target_commit // "<missing>"' "$state_output" 2>/dev/null || printf '<unresolved>')"
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed candidate_id=$observed_candidate target_commit=$observed_target; $state_error" >&2
    exit 1
  fi
fi
