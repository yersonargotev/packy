#!/usr/bin/env bash

set -euo pipefail

run="${1:?usage: result-state.sh run.json artifact-directory live-pr.json}"
artifacts="${2:?usage: result-state.sh run.json artifact-directory live-pr.json}"
live_pr="${3:-}"
status="$(jq -er .status "$run")"

case "$status" in
  queued|pending|requested|waiting) echo "pendiente"; exit 0 ;;
  in_progress) echo "ejecución iniciada"; exit 0 ;;
  completed) ;;
  *) echo "bloqueada"; exit 0 ;;
esac

find_artifact() {
  find "$artifacts" -type f -name "$1" -print -quit
}

if [[ -n "$(find_artifact no-op.json)" ]]; then
  echo "sin cambios"
elif publication="$(find_artifact publication.json)"; [[ -n "$publication" ]] && jq -e '.decision_ready == true' "$publication" >/dev/null; then
  number="$(jq -er .pr_number "$publication")"
  head="$(jq -er .head_sha "$publication")"
  branch="$(jq -er .branch_name "$publication")"
  if [[ -n "$live_pr" ]] && jq -e --argjson number "$number" --arg head "$head" --arg branch "$branch" '
    .number == $number and .headRefOid == $head and .headRefName == $branch and
    .state == "OPEN" and .isDraft == false
  ' "$live_pr" >/dev/null; then
    echo "decision-ready"
  else
    echo "bloqueada"
  fi
elif [[ -n "$(find_artifact inspection.json)" ]]; then
  echo "pendiente"
elif [[ -n "$(find_artifact operational-artifact.json)" ]]; then
  echo "bloqueada"
else
  echo "bloqueada"
fi
