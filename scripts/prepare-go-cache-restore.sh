#!/usr/bin/env bash

set -euo pipefail

if (($# != 3)); then
  echo "usage: prepare-go-cache-restore.sh LOOKUP_HIT GOMODCACHE GOCACHE" >&2
  exit 2
fi

lookup_hit="$1"
shift

if [[ -n "$lookup_hit" && "$lookup_hit" != "true" && "$lookup_hit" != "false" ]]; then
  echo "cache action returned an invalid hit value: $lookup_hit" >&2
  exit 1
fi
if [[ "$lookup_hit" != "true" ]]; then
  echo "No exact Go cache exists; preserving the current cache directories."
  exit 0
fi

: "${HOME:?HOME must identify the runner home directory}"
runner_temp="${RUNNER_TEMP:-}"

for cache_path in "$@"; do
  if [[ "$cache_path" != /* ||
    "$cache_path" == "/" ||
    "$cache_path" == *"/../"* ||
    "$cache_path" == *"/./"* ||
    "$cache_path" == "$HOME" ||
    "$cache_path" != "$HOME/"* && (-z "$runner_temp" || "$cache_path" != "$runner_temp/"*) ]]; then
    echo "::error::Refusing unsafe cache path: $cache_path" >&2
    exit 1
  fi

  mkdir -p "$cache_path"
  if [[ ! -d "$cache_path" || -L "$cache_path" ]]; then
    echo "::error::Refusing unsafe cache path: $cache_path" >&2
    exit 1
  fi
  find "$cache_path" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
done

echo "Prepared empty Go cache directories for exact restoration."
