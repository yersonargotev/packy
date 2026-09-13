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
cleanup() {
  find "$release_json" -delete
  find "$published" -depth -delete
}
trap cleanup EXIT

release_exists=false
if "$gh_bin" api "repos/$repository/releases/tags/$tag" > "$release_json" 2>/dev/null; then
  release_exists=true
fi

if [[ "$release_exists" == false ]]; then
  if ! "$gh_bin" release create "$tag" "$archive" "$checksums" \
    --repo "$repository" \
    --target "$commit" \
    --title "$tag" \
    --notes "Immutable Catalog Snapshot for reviewed commit $commit." \
    --latest=false; then
    "$gh_bin" api "repos/$repository/releases/tags/$tag" > "$release_json"
  fi
fi

"$gh_bin" api "repos/$repository/releases/tags/$tag" > "$release_json"
[[ "$(jq -r .tag_name "$release_json")" == "$tag" ]] || { echo "published Catalog Snapshot tag differs" >&2; exit 1; }
[[ "$(jq -r .target_commitish "$release_json")" == "$commit" ]] || { echo "published Catalog Snapshot commit differs" >&2; exit 1; }
[[ "$(jq -r .draft "$release_json")" == false ]] || { echo "published Catalog Snapshot is still a draft" >&2; exit 1; }

"$gh_bin" release download "$tag" --repo "$repository" --dir "$published"
printf '%s\n' SHA256SUMS catalog-snapshot.tar.gz | sort > "$published/expected-assets"
find "$published" -mindepth 1 -maxdepth 1 ! -name expected-assets ! -name actual-assets -exec basename {} \; | sort > "$published/actual-assets"
cmp "$published/expected-assets" "$published/actual-assets" >/dev/null || {
  echo "published Catalog Snapshot assets differ" >&2
  exit 1
}
for asset in SHA256SUMS catalog-snapshot.tar.gz; do
  cmp "$dist/$asset" "$published/$asset" >/dev/null || {
    echo "published Catalog Snapshot bytes differ for $asset" >&2
    exit 1
  }
done

if [[ "$release_exists" == true ]]; then
  echo "Catalog Snapshot already published unchanged: $tag"
else
  echo "published Catalog Snapshot: $tag"
fi
