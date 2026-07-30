#!/usr/bin/env bash

set -euo pipefail

if (($# != 2)); then
  echo "usage: verify-go-cache-restore.sh LOOKUP_HIT RESTORE_HIT" >&2
  exit 2
fi

lookup_hit="$1"
restore_hit="$2"
for value in "$lookup_hit" "$restore_hit"; do
  if [[ -n "$value" && "$value" != "true" && "$value" != "false" ]]; then
    echo "cache action returned an invalid hit value: $value" >&2
    exit 1
  fi
done

if [[ "$lookup_hit" == "true" && "$restore_hit" != "true" ]]; then
  echo "::error::The exact Go cache existed but could not be restored; refusing to treat possible cache corruption as a miss." >&2
  exit 1
fi

if [[ "$restore_hit" == "true" ]]; then
  echo "Go cache restored."
else
  echo "Go cache miss; validation will continue without cached content."
fi
