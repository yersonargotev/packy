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

canonical_home="$(cd -P -- "$HOME" && pwd -P)"
trusted_roots=("$canonical_home")
if [[ -n "$runner_temp" && -d "$runner_temp" ]]; then
  canonical_runner_temp="$(cd -P -- "$runner_temp" && pwd -P)"
  trusted_roots+=("$canonical_runner_temp")
fi

cache_paths=()
for cache_path in "$@"; do
  if [[ "$cache_path" != /* ||
    "$cache_path" == "/" ||
    "$cache_path" == *"//"* ||
    "$cache_path" =~ (^|/)\.\.?(/|$) ||
    -n "$runner_temp" && "$cache_path" == "$runner_temp" ||
    -L "$cache_path" ]]; then
    echo "::error::Refusing unsafe cache path: $cache_path" >&2
    exit 1
  fi

  existing_path="$cache_path"
  while [[ ! -e "$existing_path" && ! -L "$existing_path" ]]; do
    existing_path="$(dirname "$existing_path")"
  done
  if [[ ! -d "$existing_path" ]]; then
    echo "::error::Refusing unsafe cache path: $cache_path" >&2
    exit 1
  fi
  canonical_existing_path="$(cd -P -- "$existing_path" && pwd -P)"
  canonical_cache_path="$canonical_existing_path${cache_path#"$existing_path"}"
  for trusted_root in "${trusted_roots[@]}"; do
    if [[ "$canonical_cache_path" == "$trusted_root" ]]; then
      echo "::error::Refusing unsafe cache path: $cache_path" >&2
      exit 1
    fi
  done
  trusted=false
  for trusted_root in "${trusted_roots[@]}"; do
    if [[ "$canonical_cache_path" == "$trusted_root/"* ]]; then
      trusted=true
      break
    fi
  done
  if [[ "$trusted" != "true" ]]; then
    echo "::error::Refusing unsafe cache path: $cache_path" >&2
    exit 1
  fi
  cache_paths+=("$canonical_cache_path")
done

for cache_path in "${cache_paths[@]}"; do
  mkdir -p "$cache_path"
  find "$cache_path" -type d -exec chmod u+rwx {} +
  find "$cache_path" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
done

echo "Prepared empty Go cache directories for exact restoration."
