#!/usr/bin/env bash

set -euo pipefail

request="${1:?usage: attach.sh canonical-request.json runs.json request-artifacts}"
runs="${2:?usage: attach.sh canonical-request.json runs.json request-artifacts}"
artifacts="${3:?usage: attach.sh canonical-request.json runs.json request-artifacts}"
. "$(dirname "${BASH_SOURCE[0]}")/request.sh"
source_id="$(jq -er .source_id "$request")"
request_digest="$(request_digest "$request")"
run_name="sync-pack-source / $source_id / $request_digest"

matches="$(jq -c --arg name "$run_name" '[.[] | select(
  .displayTitle == $name and
  (.status == "queued" or .status == "in_progress" or .status == "pending" or
   .status == "requested" or .status == "waiting")
)]' "$runs")"
count="$(jq length <<<"$matches")"
if [[ "$count" -gt 1 ]]; then
  echo "multiple active or pending runs expose the same request identity" >&2
  exit 2
fi
if [[ "$count" -eq 0 ]]; then
  exit 1
fi
status="$(jq -er '.[0].status' <<<"$matches")"
if [[ "$status" == "in_progress" ]]; then
  run_id="$(jq -er '.[0].databaseId' <<<"$matches")"
  owner_request="$artifacts/$run_id-request.json"
  if [[ ! -f "$owner_request" ]] || ! cmp -s <(jq -cS . "$request") <(jq -cS . "$owner_request"); then
    echo "started run request artifact is absent or does not match its run identity" >&2
    exit 2
  fi
fi
jq -er '.[0].url' <<<"$matches"
