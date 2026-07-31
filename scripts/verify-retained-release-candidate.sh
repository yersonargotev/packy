#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Verify retained bytes and publication metadata for one manual release recovery.

Usage: scripts/verify-retained-release-candidate.sh \
  --tag <v0.x.y> --commit <40-hex-sha> --run-id <workflow-run-id> \
  --dist <directory> --metadata <directory> --verifier <releasecandidate>
EOF
}

tag=""
commit=""
run_id=""
dist=""
metadata=""
verifier=""
while (($#)); do
  case "$1" in
    --tag) tag="${2:-}"; shift 2 ;;
    --commit) commit="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    --metadata) metadata="${2:-}"; shift 2 ;;
    --verifier) verifier="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ ]] || { echo "retained candidate tag must be v0.x.y" >&2; exit 2; }
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || { echo "retained candidate commit must be a full lowercase SHA" >&2; exit 2; }
[[ "$run_id" =~ ^[1-9][0-9]*$ ]] || { echo "original retained candidate run ID is required" >&2; exit 2; }
for path in "$dist" "$metadata" "$verifier"; do
  [[ -n "$path" ]] || { echo "retained candidate paths and verifier are required" >&2; exit 2; }
done
for command in jq find sort cmp; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
[[ -x "$verifier" ]] || { echo "release candidate verifier is missing or not executable" >&2; exit 1; }

candidate="$metadata/candidate.json"
provenance="$metadata/provenance.json"
plan="$metadata/publication-plan.json"
draft_base="$metadata/draft-base.json"
for path in "$candidate" "$provenance" "$plan" "$draft_base"; do
  [[ -f "$path" && ! -L "$path" ]] || { echo "retained publication metadata is missing: $(basename "$path")" >&2; exit 1; }
done

"$verifier" verify-provenance --candidate "$candidate" --provenance "$provenance" >/dev/null
jq -e --arg tag "$tag" --arg commit "$commit" --arg run_id "$run_id" \
  --slurpfile candidate "$candidate" --slurpfile provenance "$provenance" --slurpfile plan "$plan" '
  .schema_version == 1 and .candidate_id == $candidate[0].id and
  .provenance == $provenance[0] and .target_commit == $commit and
  .source_run_id == $run_id and .attestation_source_ref == ("refs/tags/" + $tag) and
  .publication_plan == $plan[0] and
  $plan[0].schema_version == 1 and $plan[0].tag == $tag and
  $plan[0].target_commit == $commit and $plan[0].source_run_id == $run_id and
  $plan[0].attestation_source_ref == ("refs/tags/" + $tag) and
  $plan[0].candidate_id == $candidate[0].id and
  $plan[0].candidate_assets == $candidate[0].subjects and
  $candidate[0].version == $tag and $candidate[0].commit == $commit
' "$draft_base" >/dev/null || {
  echo "retained publication metadata diverges from the sealed recovery identity or original run" >&2
  exit 1
}

scratch="$(mktemp -d "${TMPDIR:-/tmp}/packy-retained-release.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT
jq -r '.subjects[].name' "$candidate" | sort > "$scratch/expected"
find "$dist" -mindepth 1 -maxdepth 1 -exec basename {} \; | sort > "$scratch/actual"
cmp -s "$scratch/expected" "$scratch/actual" || {
  echo "retained candidate artifact set is missing, expired, or unexpected" >&2
  exit 1
}
while IFS=$'\t' read -r digest name; do
  [[ "$digest" =~ ^[0-9a-f]{64}$ && -f "$dist/$name" && ! -L "$dist/$name" ]] || {
    echo "retained candidate subject is invalid or missing: $name" >&2
    exit 1
  }
  if command -v sha256sum >/dev/null; then
    actual="$(sha256sum "$dist/$name" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$dist/$name" | awk '{print $1}')"
  fi
  [[ "$actual" == "$digest" ]] || { echo "retained candidate digest diverges for $name" >&2; exit 1; }
done < <(jq -r '.subjects[]|[.sha256,.name]|@tsv' "$candidate")

echo "retained release candidate verified: tag=$tag commit=$commit original_run_id=$run_id"
