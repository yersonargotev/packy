#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --version <vX.Y.Z> [--out-dir <directory>]" >&2
  exit 2
}

version="${RELEASE_VERSION:-}"
out_dir="${RELEASE_OUT_DIR:-dist}"
while (($#)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --out-dir) out_dir="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "release version must be a tag such as v0.2.0" >&2
  exit 2
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd)"
if find "$out_dir" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  echo "release output directory must be empty: $out_dir" >&2
  exit 1
fi

platforms=(
  darwin/amd64
  darwin/arm64
  linux/amd64
  linux/arm64
)

staging="$(mktemp -d "${TMPDIR:-/tmp}/packy-release-build.XXXXXX")"
trap 'rm -rf "$staging"' EXIT
archives=()
cd "$repo_root"
for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  platform_dir="$staging/${goos}_${goarch}"
  archive="packy_${version}_${goos}_${goarch}.tar.gz"
  mkdir -p "$platform_dir"
  echo "building $archive"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath \
      -ldflags="-s -w -X github.com/yersonargotev/packy/internal/version.Value=${version}" \
      -o "$platform_dir/packy" ./cmd/packy
  COPYFILE_DISABLE=1 tar -C "$platform_dir" -czf "$out_dir/$archive" packy
  archives+=("$archive")
done

(
  cd "$out_dir"
  : > SHA256SUMS
  for archive in "${archives[@]}"; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$archive" >> SHA256SUMS
    else
      shasum -a 256 "$archive" >> SHA256SUMS
    fi
  done
)

echo "wrote four platform archives and SHA256SUMS to $out_dir"
