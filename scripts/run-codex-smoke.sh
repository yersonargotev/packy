#!/usr/bin/env bash
set -euo pipefail
codex_version=""; packy_ref=""; run_id=""; evidence_dir=""
while (($#)); do case "$1" in --codex-version) codex_version="${2:-}";shift 2;;--packy-ref) packy_ref="${2:-}";shift 2;;--run-id) run_id="${2:-}";shift 2;;--evidence-dir) evidence_dir="${2:-}";shift 2;;*) echo "unknown argument: $1" >&2;exit 2;;esac;done
[[ "$codex_version" == "0.145.0" ]] || { echo "--codex-version must be exactly 0.145.0" >&2;exit 2; }
[[ -n "$packy_ref" && -n "$run_id" && -n "$evidence_dir" ]] || { echo "--packy-ref, --run-id, and --evidence-dir are required" >&2;exit 2; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; sha="$(git -C "$root" rev-parse --verify "${packy_ref}^{commit}")"; [[ "$sha" == "$(git -C "$root" rev-parse HEAD)" ]] || { echo "ref must resolve to checkout HEAD" >&2;exit 1; }
evidence_dir="$(mkdir -p "$evidence_dir" && cd "$evidence_dir" && pwd)"; build="$(mktemp -d "${TMPDIR:-/tmp}/packy-codex-smoke.XXXXXX")"; trap 'chmod -R u+w "$build" 2>/dev/null || true; rm -rf "$build"' EXIT
export HOME="$build/acquire-home" XDG_CONFIG_HOME="$build/acquire-config" npm_config_cache="$build/npm-cache" npm_config_userconfig="$build/npmrc"; mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$npm_config_cache"; : >"$npm_config_userconfig"
meta="$(npm view "@openai/codex@$codex_version" version dist.integrity --json)"; resolved="$(printf '%s' "$meta" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).version))')"; integrity="$(printf '%s' "$meta" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s)["dist.integrity"]))')"; [[ "$resolved" == "$codex_version" && -n "$integrity" ]] || exit 1
npm install --prefix "$build/npm" --no-audit --no-fund "@openai/codex@$resolved" >/dev/null
(cd "$root" && go build -trimpath -o "$build/codexsmoke" ./internal/tools/codexsmoke)
node_dir="$(dirname "$(command -v node)")"
restricted_path="$build/npm/node_modules/.bin:$node_dir:/usr/bin:/bin"
env -i PATH="$restricted_path" TMPDIR="${TMPDIR:-/tmp}" "$build/codexsmoke" --codex "$build/npm/node_modules/.bin/codex" --search-path "$restricted_path" --codex-version "$resolved" --codex-integrity "$integrity" --packy-ref "$packy_ref" --packy-sha "$sha" --run-id "$run_id" --evidence "$evidence_dir/evidence.json"
