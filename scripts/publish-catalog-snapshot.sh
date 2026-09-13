#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --repository <owner/repo> --commit <full-commit> --dist <directory>" >&2
  exit 2
}

repository=""
commit=""
dist=""
while (($#)); do
  case "$1" in
    --repository) repository="${2:-}"; shift 2 ;;
    --commit) commit="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || usage
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || usage
[[ -d "$dist" ]] || { echo "Catalog Snapshot directory not found: $dist" >&2; exit 1; }

archive="$dist/catalog-snapshot.tar.gz"
checksums="$dist/SHA256SUMS"
[[ -f "$archive" && ! -L "$archive" ]] || { echo "catalog-snapshot.tar.gz must be a regular file" >&2; exit 1; }
[[ -f "$checksums" && ! -L "$checksums" ]] || { echo "SHA256SUMS must be a regular file" >&2; exit 1; }
[[ "$(wc -l < "$checksums" | tr -d ' ')" == 1 ]] || { echo "SHA256SUMS must contain exactly one entry" >&2; exit 1; }
read -r expected_digest expected_name extra < "$checksums"
[[ "$expected_digest" =~ ^[0-9a-f]{64}$ && "$expected_name" == "catalog-snapshot.tar.gz" && -z "${extra:-}" ]] || {
  echo "SHA256SUMS is malformed" >&2
  exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
  actual_digest="$(sha256sum "$archive" | awk '{print $1}')"
else
  actual_digest="$(shasum -a 256 "$archive" | awk '{print $1}')"
fi
[[ "$actual_digest" == "$expected_digest" ]] || { echo "Catalog Snapshot checksum mismatch" >&2; exit 1; }

gh_bin="${GH_BIN:-gh}"
tag="catalog-$commit"
release_json="$(mktemp "${TMPDIR:-/tmp}/packy-catalog-release.XXXXXX")"
published="$(mktemp -d "${TMPDIR:-/tmp}/packy-catalog-published.XXXXXX")"
expected_assets="$(mktemp "${TMPDIR:-/tmp}/packy-catalog-expected.XXXXXX")"
actual_assets="$(mktemp "${TMPDIR:-/tmp}/packy-catalog-actual.XXXXXX")"
cleanup() {
  find "$release_json" "$expected_assets" "$actual_assets" -delete
  find "$published" -depth -delete
}
trap cleanup EXIT

fetch_release() {
  if "$gh_bin" api "repos/$repository/releases/tags/$tag" > "$release_json" 2>/dev/null; then
    return 0
  fi
  "$gh_bin" api --paginate "repos/$repository/releases?per_page=100" \
    --jq ".[] | select(.tag_name == \"$tag\")" > "$release_json"
  [[ -s "$release_json" ]]
}

release_exists=false
if fetch_release; then
  release_exists=true
fi

if [[ "$release_exists" == false ]]; then
  if ! "$gh_bin" release create "$tag" \
    --repo "$repository" \
    --target "$commit" \
    --title "$tag" \
    --notes "Immutable Catalog Snapshot for reviewed commit $commit." \
    --draft \
    --latest=false; then
    fetch_release
  fi
fi

fetch_release
[[ "$(jq -r .tag_name "$release_json")" == "$tag" ]] || { echo "published Catalog Snapshot tag differs" >&2; exit 1; }
[[ "$(jq -r .target_commitish "$release_json")" == "$commit" ]] || { echo "published Catalog Snapshot commit differs" >&2; exit 1; }
draft="$(jq -r .draft "$release_json")"
immutable="$(jq -r .immutable "$release_json")"
[[ "$draft" == true || "$draft" == false ]] || { echo "published Catalog Snapshot draft state is malformed" >&2; exit 1; }
[[ "$immutable" == true || "$immutable" == false ]] || { echo "published Catalog Snapshot immutable state is malformed" >&2; exit 1; }
if [[ "$draft" == false && "$immutable" != true ]]; then
  echo "published Catalog Snapshot is mutable" >&2
  exit 1
fi

published_asset_count="$(jq -r '(.assets // []) | length' "$release_json")"
[[ "$published_asset_count" =~ ^[0-9]+$ ]] || { echo "published Catalog Snapshot asset metadata is malformed" >&2; exit 1; }
if ((published_asset_count > 0)); then
  "$gh_bin" release download "$tag" --repo "$repository" --dir "$published"
fi
printf '%s\n' SHA256SUMS catalog-snapshot.tar.gz | sort > "$expected_assets"
find "$published" -mindepth 1 -maxdepth 1 -exec basename {} \; | sort > "$actual_assets"
while IFS= read -r asset; do
  if ! grep -Fxq "$asset" "$expected_assets"; then
    echo "published Catalog Snapshot contains unexpected asset $asset" >&2
    exit 1
  fi
done < "$actual_assets"
resumed=false
if [[ "$release_exists" == true && "$draft" == true ]]; then
  resumed=true
fi
for asset in SHA256SUMS catalog-snapshot.tar.gz; do
  if [[ -e "$published/$asset" ]]; then
    cmp "$dist/$asset" "$published/$asset" >/dev/null || {
      echo "published Catalog Snapshot bytes differ for $asset" >&2
      exit 1
    }
  else
    [[ "$draft" == true ]] || { echo "immutable Catalog Snapshot is missing $asset" >&2; exit 1; }
    "$gh_bin" release upload "$tag" "$dist/$asset" --repo "$repository"
    resumed=true
  fi
done

find "$published" -mindepth 1 -depth -delete
"$gh_bin" release download "$tag" --repo "$repository" --dir "$published"
find "$published" -mindepth 1 -maxdepth 1 -exec basename {} \; | sort > "$actual_assets"
cmp "$expected_assets" "$actual_assets" >/dev/null || {
  echo "published Catalog Snapshot assets differ" >&2
  exit 1
}
for asset in SHA256SUMS catalog-snapshot.tar.gz; do
  cmp "$dist/$asset" "$published/$asset" >/dev/null || {
    echo "published Catalog Snapshot bytes differ for $asset" >&2
    exit 1
  }
done

if [[ "$draft" == true ]]; then
  "$gh_bin" release edit "$tag" --repo "$repository" --draft=false
  fetch_release
  [[ "$(jq -r .draft "$release_json")" == false ]] || { echo "published Catalog Snapshot is still a draft" >&2; exit 1; }
  [[ "$(jq -r .immutable "$release_json")" == true ]] || { echo "published Catalog Snapshot is not immutable" >&2; exit 1; }
fi

if [[ "$release_exists" == false ]]; then
  echo "published immutable Catalog Snapshot: $tag"
elif [[ "$resumed" == true ]]; then
  echo "completed interrupted Catalog Snapshot publication: $tag"
else
  echo "Catalog Snapshot already published unchanged: $tag"
fi
