#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --version <vX.Y.Z> [--dist <directory>]" >&2
  exit 2
}

version=""
dist="dist"
while (($#)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "release version must be a tag such as v0.2.0" >&2
  exit 2
}
[[ -d "$dist" ]] || { echo "release directory not found: $dist" >&2; exit 1; }

platforms=(darwin_amd64 darwin_arm64 linux_amd64 linux_arm64)
scratch="$(mktemp -d "${TMPDIR:-/tmp}/packy-release-validation.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT
printf '%s\n' SHA256SUMS > "$scratch/expected"
for platform in "${platforms[@]}"; do
  printf 'packy_%s_%s.tar.gz\n' "$version" "$platform" >> "$scratch/expected"
done
sort -o "$scratch/expected" "$scratch/expected"
find "$dist" -mindepth 1 -maxdepth 1 -exec basename {} \; | sort > "$scratch/actual"
cmp "$scratch/expected" "$scratch/actual" || {
  echo "release directory must contain exactly four archives and SHA256SUMS" >&2
  exit 1
}

[[ -f "$dist/SHA256SUMS" && ! -L "$dist/SHA256SUMS" ]] || {
  echo "SHA256SUMS must be a regular file" >&2
  exit 1
}
if ! awk 'NF != 2 || $1 !~ /^[0-9a-f]{64}$/ { exit 1 }' "$dist/SHA256SUMS"; then
  echo "SHA256SUMS is malformed" >&2
  exit 1
fi
[[ "$(wc -l < "$dist/SHA256SUMS" | tr -d ' ')" == 4 ]] || {
  echo "SHA256SUMS must contain exactly four entries" >&2
  exit 1
}

for platform in "${platforms[@]}"; do
  archive="packy_${version}_${platform}.tar.gz"
  path="$dist/$archive"
  [[ -f "$path" && ! -L "$path" ]] || { echo "$archive must be a regular file" >&2; exit 1; }
  [[ "$(awk -v name="$archive" '$2 == name { count++ } END { print count+0 }' "$dist/SHA256SUMS")" == 1 ]] || {
    echo "SHA256SUMS must contain one entry for $archive" >&2
    exit 1
  }
  if command -v sha256sum >/dev/null 2>&1; then
    actual_digest="$(sha256sum "$path" | awk '{print $1}')"
  else
    actual_digest="$(shasum -a 256 "$path" | awk '{print $1}')"
  fi
  expected_digest="$(awk -v name="$archive" '$2 == name { print $1 }' "$dist/SHA256SUMS")"
  [[ "$actual_digest" == "$expected_digest" ]] || { echo "checksum mismatch for $archive" >&2; exit 1; }
  [[ "$(tar -tzf "$path")" == packy ]] || { echo "$archive must contain only packy" >&2; exit 1; }
done

case "$(uname -s)/$(uname -m)" in
  Darwin/x86_64) host_platform=darwin_amd64 ;;
  Darwin/arm64) host_platform=darwin_arm64 ;;
  Linux/x86_64) host_platform=linux_amd64 ;;
  Linux/aarch64|Linux/arm64) host_platform=linux_arm64 ;;
  *) echo "unsupported validation host: $(uname -s)/$(uname -m)" >&2; exit 1 ;;
esac

install_root="$scratch/install"
mkdir -p "$install_root/bin" "$scratch/extracted"
tar -C "$scratch/extracted" -xzf "$dist/packy_${version}_${host_platform}.tar.gz"
[[ -f "$scratch/extracted/packy" && ! -L "$scratch/extracted/packy" ]] || {
  echo "host archive did not extract one regular packy binary" >&2
  exit 1
}
install -m 0755 "$scratch/extracted/packy" "$install_root/bin/packy"
[[ "$($install_root/bin/packy --version)" == "packy version $version" ]] || {
  echo "installed binary did not report packy version $version" >&2
  exit 1
}

echo "validated checksums, archive contents, installation, and packy version $version"
