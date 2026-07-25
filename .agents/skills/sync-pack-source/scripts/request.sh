#!/usr/bin/env bash

request_digest() {
  jq -cS . "$1" | shasum -a 256 | cut -d ' ' -f 1
}

workflow_inputs() {
  local request="$1"
  local digest
  digest="$(request_digest "$request")"
  jq --arg request_digest "$digest" '
    if .schema_version == 3 then
      .source_id = .registrations[0].registration.id
      | .selector = "commit"
      | .selector_ref = .registrations[0].registration.selector.ref
    else . end
    | del(.schema_version)
    | with_entries(.value |= if type == "object" or type == "array" then tojson else tostring end)
    | if has("human_evidence") then .human_evidence_json=.human_evidence | del(.human_evidence) else . end
    | if has("registration") then .registration_json=.registration | del(.registration) else . end
    | if has("registrations") then .registrations_json=.registrations | del(.registrations) else . end
    | if has("proposed_manifest") then .proposed_manifest_json=.proposed_manifest | del(.proposed_manifest) else . end
    | .request_digest=$request_digest
  ' "$request"
}
