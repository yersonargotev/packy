#!/usr/bin/env bash
set -euo pipefail
version=""; packy_ref=""; run_id=""; evidence_dir=""
while (($#)); do case "$1" in --opencode-version) version="${2:-}";shift 2;;--packy-ref) packy_ref="${2:-}";shift 2;;--run-id) run_id="${2:-}";shift 2;;--evidence-dir) evidence_dir="${2:-}";shift 2;;*) echo "unknown argument: $1" >&2;exit 2;;esac;done
[[ "$version" == "1.18.5" ]] || { echo "--opencode-version must be exactly 1.18.5" >&2;exit 2; }
[[ -n "$packy_ref" && -n "$run_id" && -n "$evidence_dir" ]] || { echo "--packy-ref, --run-id, and --evidence-dir are required" >&2;exit 2; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; sha="$(git -C "$root" rev-parse --verify "${packy_ref}^{commit}")"; [[ "$sha" == "$(git -C "$root" rev-parse HEAD)" ]] || { echo "ref must resolve to checkout HEAD" >&2;exit 1; }
evidence_dir="$(mkdir -p "$evidence_dir" && cd "$evidence_dir" && pwd)"; build="$(mktemp -d "${TMPDIR:-/tmp}/packy-opencode-smoke.XXXXXX")"; trap 'chmod -R u+w "$build" 2>/dev/null || true; rm -rf "$build"' EXIT
export HOME="$build/acquire-home" XDG_CONFIG_HOME="$build/acquire-config"; mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$build/host"
case "$(uname -s):$(uname -m)" in
 Linux:x86_64) archive="opencode-linux-x64.tar.gz"; integrity="cd4a2557a3d6550f27cb5c0257ebe8d73388bb34beda8b6121e6428a74c1eae2";;
 Darwin:arm64) archive="opencode-darwin-arm64.zip"; integrity="85f6f9eece174d3bf0c92588086a65284388b891256c8f4102dc317d476ffca6";;
 *) echo "unsupported OpenCode smoke platform: $(uname -s) $(uname -m)" >&2; exit 2;;
esac
curl --fail --location --proto '=https' --tlsv1.2 "https://github.com/anomalyco/opencode/releases/download/v${version}/${archive}" -o "$build/$archive"
printf '%s  %s\n' "$integrity" "$build/$archive" | shasum -a 256 -c -
case "$archive" in *.zip) unzip -q "$build/$archive" -d "$build/host";; *) tar -xzf "$build/$archive" -C "$build/host";; esac
opencode="$(find "$build/host" -type f -name opencode -perm -u+x -print -quit)"; [[ -n "$opencode" ]] || { echo "archive lacks executable opencode" >&2;exit 1; }
(cd "$root" && go build -trimpath -o "$build/opencodesmoke" ./internal/tools/opencodesmoke)
restricted_path="$(dirname "$opencode"):/usr/bin:/bin"
env -i PATH="$restricted_path" TMPDIR="${TMPDIR:-/tmp}" "$build/opencodesmoke" --opencode "$opencode" --search-path "$restricted_path" --opencode-version "$version" --opencode-integrity "$integrity" --packy-ref "$packy_ref" --packy-sha "$sha" --run-id "$run_id" --evidence "$evidence_dir/evidence.json"
