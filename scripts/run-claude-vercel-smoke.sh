#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/run-claude-vercel-smoke.sh \
  --claude-version 2.1.203 --packy-ref <ref-at-HEAD> --evidence-dir <directory>
EOF
}

claude_version=""
packy_ref=""
evidence_dir=""
while (($#)); do
  case "$1" in
    --claude-version) claude_version="${2:-}"; shift 2 ;;
    --packy-ref) packy_ref="${2:-}"; shift 2 ;;
    --evidence-dir) evidence_dir="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done
if [[ "$claude_version" != "2.1.203" ]]; then
  echo "--claude-version must be exactly 2.1.203" >&2
  exit 2
fi
if [[ -z "$packy_ref" || -z "$evidence_dir" ]]; then
  echo "--packy-ref and --evidence-dir are required" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
resolved_ref="$(git -C "$root" rev-parse --verify "${packy_ref}^{commit}")"
head="$(git -C "$root" rev-parse HEAD)"
if [[ "$resolved_ref" != "$head" ]]; then
  echo "Packy ref resolves to $resolved_ref, but checkout HEAD is $head" >&2
  exit 1
fi
evidence_dir="$(mkdir -p "$evidence_dir" && cd "$evidence_dir" && pwd)"
sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-claude-vercel.XXXXXX")"
trap 'chmod -R u+w "$sandbox" 2>/dev/null || true; rm -rf "$sandbox"' EXIT
mkdir -p "$sandbox"/{home,config,cache,tmp,npm,build}
touch "$sandbox/npmrc"
export HOME="$sandbox/home"
export XDG_CONFIG_HOME="$sandbox/config"
export XDG_CACHE_HOME="$sandbox/cache"
export npm_config_cache="$sandbox/cache/npm"
export npm_config_userconfig="$sandbox/npmrc"
export TMPDIR="$sandbox/tmp"
unset ANTHROPIC_API_KEY CLAUDE_CODE_OAUTH_TOKEN AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY \
  GOOGLE_APPLICATION_CREDENTIALS VERCEL_TOKEN

metadata="$(npm view "@anthropic-ai/claude-code@${claude_version}" version dist.integrity --json)"
resolved="$(printf '%s' "$metadata" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).version))')"
integrity="$(printf '%s' "$metadata" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s)["dist.integrity"]))')"
if [[ "$resolved" != "$claude_version" || -z "$integrity" || "$integrity" == "undefined" ]]; then
  echo "npm did not resolve exact Claude ${claude_version} with integrity" >&2
  exit 1
fi
npm install --prefix "$sandbox/npm" --no-audit --no-fund "@anthropic-ai/claude-code@${claude_version}"
claude="$sandbox/npm/node_modules/.bin/claude"
actual="$("$claude" --version)"
[[ "$actual" == *"$claude_version"* ]] || { echo "installed Claude version mismatch: $actual" >&2; exit 1; }
restricted_path="$(dirname "$claude"):$(dirname "$(command -v node)"):/usr/bin:/bin"

( cd "$root" && go build -trimpath -o "$sandbox/build/claudevercelsmoke" ./internal/tools/claudevercelsmoke )
"$sandbox/build/claudevercelsmoke" \
  --claude "$claude" --search-path "$restricted_path" --claude-integrity "$integrity" \
  --packy-repo "$root" --packy-ref "$packy_ref" \
  --evidence "$evidence_dir/vercel-evidence.json"
