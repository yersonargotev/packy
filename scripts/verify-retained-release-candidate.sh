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
for command in jq find sort; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
[[ -x "$verifier" ]] || { echo "release candidate verifier is missing or not executable" >&2; exit 1; }

scratch="$(mktemp -d "${TMPDIR:-/tmp}/packy-retained-release.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT
printf '[]\n' > "$scratch/subjects.json"
while IFS= read -r path; do
  [[ -f "$path" && ! -L "$path" ]] || { echo "retained candidate entry is not a regular file" >&2; exit 1; }
  name="$(basename "$path")"
  if command -v sha256sum >/dev/null; then
    digest="$(sha256sum "$path" | awk '{print $1}')"
  else
    digest="$(shasum -a 256 "$path" | awk '{print $1}')"
  fi
  jq --arg name "$name" --arg sha256 "$digest" '. + [{name:$name,sha256:$sha256}]' "$scratch/subjects.json" > "$scratch/next.json"
  mv "$scratch/next.json" "$scratch/subjects.json"
done < <(find "$dist" -mindepth 1 -maxdepth 1 -print | sort)

"$verifier" verify-recovery --tag "$tag" --commit "$commit" --run-id "$run_id" \
  --candidate "$metadata/candidate.json" --provenance "$metadata/provenance.json" \
  --publication-plan "$metadata/publication-plan.json" --draft-base "$metadata/draft-base.json" \
  --subjects "$scratch/subjects.json" >/dev/null

echo "retained release candidate verified: tag=$tag commit=$commit original_run_id=$run_id"
