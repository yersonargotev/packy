#!/usr/bin/env bash
set -euo pipefail

repository=""; tag=""; run_id=""; dist=""; metadata=""
while (($#)); do
  case "$1" in
    --repository) repository="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    --metadata) metadata="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repository" && "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ && "$run_id" =~ ^[1-9][0-9]*$ && -n "$dist" && -n "$metadata" ]] || { echo "repository, tag, original run ID, dist, and metadata are required" >&2; exit 2; }
mkdir -p "$dist" "$metadata"
gh run download "$run_id" --repo "$repository" --name "packy-release-$tag" --dir "$dist"
gh run download "$run_id" --repo "$repository" --name "packy-release-metadata-$tag" --dir "$metadata"
