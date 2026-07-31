#!/usr/bin/env bash
set -euo pipefail

repository=
tag=
release_commit=
verifier=
output=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repository) repository="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --release-commit) release_commit="${2:-}"; shift 2 ;;
    --verifier) verifier="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ "$repository" == */* && "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ &&
   "$release_commit" =~ ^[0-9a-f]{40}$ && -x "$verifier" && -n "$output" ]] || {
  echo 'repository, valid v0 tag, release commit, executable verifier, and output are required' >&2
  exit 2
}

resolve_ref_commit() {
  local ref="$1" object object_type object_sha hops=0
  object="$(gh api "repos/$repository/git/ref/$ref" --jq '[.object.type,.object.sha]|@tsv')"
  IFS=$'\t' read -r object_type object_sha <<< "$object"
  while [[ "$object_type" == tag && "$hops" -lt 8 ]]; do
    object="$(gh api "repos/$repository/git/tags/$object_sha" --jq '[.object.type,.object.sha]|@tsv')"
    IFS=$'\t' read -r object_type object_sha <<< "$object"
    hops=$((hops + 1))
  done
  [[ "$object_type" == commit && "$object_sha" =~ ^[0-9a-f]{40}$ ]] || {
    echo "ref $ref did not peel uniquely to one commit" >&2
    return 1
  }
  printf '%s\n' "$object_sha"
}

tag_commit="$(resolve_ref_commit "tags/$tag")"
main_commit="$(resolve_ref_commit heads/main)"
ancestry="$(gh api "repos/$repository/compare/$release_commit...$main_commit" --jq .status)"
release_in_main=false
[[ "$ancestry" == ahead || "$ancestry" == identical ]] && release_in_main=true
observation="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-ref-observation.XXXXXX")"
trap 'rm -f "$observation"' EXIT
jq -n --arg tag "$tag" --arg expected_tag_commit "$release_commit" \
  --arg remote_tag_commit "$tag_commit" --arg release_commit "$release_commit" \
  --arg current_main "$main_commit" --argjson release_in_main "$release_in_main" \
  '{tag:$tag,expected_tag_commit:$expected_tag_commit,remote_tag_commit:$remote_tag_commit,
    release_commit:$release_commit,current_main:$current_main,release_in_main:$release_in_main}' \
  > "$observation"
"$verifier" verify-ref-state --observation "$observation" > "$output"
