#!/usr/bin/env bash

set -euo pipefail

candidate_sha=""
run_id=""
foundation_evidence=""
codex_evidence=""
opencode_evidence=""
claude_evidence=""
output=""
while (($#)); do
  case "$1" in
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    --foundation-evidence) foundation_evidence="${2:-}"; shift 2 ;;
    --codex-evidence) codex_evidence="${2:-}"; shift 2 ;;
    --opencode-evidence) opencode_evidence="${2:-}"; shift 2 ;;
    --claude-evidence) claude_evidence="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ ! "$candidate_sha" =~ ^[0-9a-f]{40}$ || -z "$run_id" ]]; then
  echo "exact candidate SHA and run ID are required" >&2
  exit 2
fi
if [[ "$output" != /* || -e "$output" ]]; then
  echo "output must be a new absolute path" >&2
  exit 2
fi
for result in "${VALIDATE_RESULT:-}" "${CODEX_RESULT:-}" "${OPENCODE_RESULT:-}" "${CLAUDE_RESULT:-}"; do
  if [[ "$result" != "success" ]]; then
    echo "every foundation and host job must succeed before Vercel acceptance" >&2
    exit 1
  fi
done
if [[ "$foundation_evidence" != /* || ! -d "$foundation_evidence" || -L "$foundation_evidence" ]]; then
  echo "foundation evidence must be an absolute non-symlink directory" >&2
  exit 1
fi
for evidence in "$codex_evidence" "$opencode_evidence" "$claude_evidence"; do
  if [[ "$evidence" != /* || ! -f "$evidence" || -L "$evidence" ]]; then
    echo "host evidence must be an absolute regular non-symlink file: ${evidence:-<empty>}" >&2
    exit 1
  fi
done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$(git -C "$root" rev-parse HEAD)" != "$candidate_sha" ]]; then
  echo "candidate SHA does not match checkout HEAD" >&2
  exit 1
fi
if [[ -n "$(git -C "$root" status --porcelain --untracked-files=normal)" ]]; then
  echo "Vercel acceptance requires a clean checkout of the candidate SHA" >&2
  exit 1
fi

sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-vercel-acceptance.XXXXXX")"
trap 'chmod -R u+w "$sandbox" 2>/dev/null || true; rm -rf "$sandbox"' EXIT
mkdir -p "$sandbox/home" "$sandbox/xdg" "$sandbox/bin" "$(dirname "$output")"
(cd "$root" && go build -trimpath -o "$sandbox/bin/vercelacceptance" ./internal/tools/vercelacceptance)

env -i \
  HOME="$sandbox/home" \
  XDG_CONFIG_HOME="$sandbox/xdg" \
  PATH="/usr/bin:/bin" \
  "$sandbox/bin/vercelacceptance" \
  --candidate-sha "$candidate_sha" \
  --run-id "$run_id" \
  --collected-at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --foundation-evidence "$foundation_evidence" \
  --codex-evidence "$codex_evidence" \
  --opencode-evidence "$opencode_evidence" \
  --claude-evidence "$claude_evidence" \
  --output "$output"
