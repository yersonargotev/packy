#!/usr/bin/env bash

set -euo pipefail

branch="${1:?usage: prepare.sh issue-branch canonical-request.json}"
request="${2:?usage: prepare.sh issue-branch canonical-request.json}"

[[ "$branch" =~ ^(feat|fix|chore)/issue-[0-9]+-[a-z0-9-]+$ ]] ||
  { echo "preparation requires an approved issue branch" >&2; exit 2; }
jq -e '.schema_version == 3 and .operation == "register_bundle"' "$request" >/dev/null ||
  { echo "preparation requires one canonical v3 register_bundle request" >&2; exit 2; }

. "$(dirname "${BASH_SOURCE[0]}")/request.sh"

workflow_inputs "$request" |
  jq '.prepare_only = "true"' |
  gh workflow run .github/workflows/sync-pack-source.yml \
    --repo yersonargotev/packy \
    --ref "$branch" \
    --json
