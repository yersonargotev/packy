#!/usr/bin/env bash

set -euo pipefail

repo=
pull_request=
merge_sha=
output_dir=
while (($#)); do
  case "$1" in
    --repo) repo="${2:-}"; shift 2 ;;
    --pull-request) pull_request="${2:-}"; shift 2 ;;
    --merge-sha) merge_sha="${2:-}"; shift 2 ;;
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ &&
   "$pull_request" =~ ^[1-9][0-9]*$ &&
   "$merge_sha" =~ ^[0-9a-f]{40}$ &&
   "$output_dir" = /* && ! -e "$output_dir" ]] || {
  echo "repo, pull-request, exact merge-sha, and a new absolute output-dir are required" >&2
  exit 2
}

GH_BIN="${GH_BIN:-gh}"
attempts="${ADDY_GOVERNANCE_POLL_ATTEMPTS:-60}"
interval="${ADDY_GOVERNANCE_POLL_INTERVAL:-5}"
[[ "$attempts" =~ ^[1-9][0-9]*$ && "$interval" =~ ^[0-9]+$ ]] || {
  echo "poll attempts and interval must be non-negative integers with at least one attempt" >&2
  exit 2
}

artifact_name="addy-governance-pr-$pull_request-$merge_sha"
workflow_id="$("$GH_BIN" api "repos/$repo/actions/workflows/addy-governance.yml" --jq .id)"
[[ "$workflow_id" =~ ^[1-9][0-9]*$ ]] || {
  echo "trusted Addy governance workflow identity is unavailable" >&2
  exit 1
}

parent="$(dirname "$output_dir")"
mkdir -p "$parent"
for ((attempt = 1; attempt <= attempts; attempt++)); do
  artifacts="$("$GH_BIN" api "repos/$repo/actions/artifacts?name=$artifact_name&per_page=100" 2>/dev/null || true)"
  while IFS=$'\t' read -r artifact_id run_id; do
    [[ "$artifact_id" =~ ^[1-9][0-9]*$ && "$run_id" =~ ^[1-9][0-9]*$ ]] || continue
    run="$("$GH_BIN" api "repos/$repo/actions/runs/$run_id" 2>/dev/null || true)"
    jq -e \
      --arg repo "$repo" \
      --argjson workflow_id "$workflow_id" \
      '.event == "pull_request_target" and
       .status == "completed" and
       .conclusion == "success" and
       .workflow_id == $workflow_id and
       .repository.full_name == $repo' \
      <<<"$run" >/dev/null 2>&1 || continue

    stage="$(mktemp -d "$parent/.addy-governance.XXXXXX")"
    if "$GH_BIN" run download "$run_id" \
      --repo "$repo" \
      --name "$artifact_name" \
      --dir "$stage" >/dev/null 2>&1; then
      shopt -s nullglob dotglob
      entries=("$stage"/*)
      names=()
      valid=true
      for entry in "${entries[@]}"; do
        if [[ ! -f "$entry" || -L "$entry" ]]; then
          valid=false
          break
        fi
        names+=("$(basename "$entry")")
      done
      actual="$(printf '%s\n' "${names[@]}" | sort)"
      expected="$(printf '%s\n' blocking-issues.json canonical-issues.json evaluation.json gate.json observation.json)"
      if [[ "$valid" == true && "$actual" == "$expected" ]]; then
        mv "$stage" "$output_dir"
        printf '%s\n' "$output_dir" >&2
        exit 0
      fi
    fi
    rm -rf "$stage"
  done < <(
    jq -r \
      --arg name "$artifact_name" \
      '[.artifacts[]? |
        select(.name == $name and .expired == false and (.workflow_run.id | type) == "number")] |
       sort_by(.updated_at) | reverse[] | [.id, .workflow_run.id] | @tsv' \
      <<<"$artifacts" 2>/dev/null || true
  )
  ((attempt == attempts)) || sleep "$interval"
done

echo "trusted Addy governance evidence was not available for PR $pull_request merge $merge_sha" >&2
exit 1
