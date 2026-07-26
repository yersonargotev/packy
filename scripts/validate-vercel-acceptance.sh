#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

evidence_dir=""
candidate_sha=""
run_id=""
while (($#)); do
  case "$1" in
    --evidence-dir) evidence_dir="${2:-}"; shift 2 ;;
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --run-id) run_id="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [[ -n "$evidence_dir" ]]; then
  if [[ "$evidence_dir" != /* || -e "$evidence_dir" ||
        ! "$candidate_sha" =~ ^[0-9a-f]{40}$ || ! "$run_id" =~ ^[A-Za-z0-9_.:-]+$ ]]; then
    echo "foundation evidence requires a new absolute directory, exact candidate SHA, and safe run ID" >&2
    exit 2
  fi
  mkdir -p "$evidence_dir"
elif [[ -n "$candidate_sha$run_id" ]]; then
  echo "foundation identity is not accepted without --evidence-dir" >&2
  exit 2
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/packy-vercel-foundation.XXXXXX")"
trap 'rm -rf "$work"' EXIT

# Host-native readiness rows 17-19 are intentionally absent: the PR cohort
# gate binds their independent exact-version artifacts after this foundation.
readonly mappings=(
  "VERCEL-ACCEPTANCE-01|./internal/packsync|TestVercelLegalAdmissionEvidence"
  "VERCEL-ACCEPTANCE-02|./internal/tools/syncpacksource|TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope"
  "VERCEL-ACCEPTANCE-03|./internal/packsync|TestVercelLegalAdmissionEvidence"
  "VERCEL-ACCEPTANCE-04|./internal/vercelacceptance|TestExactSelectedTreesAreCompleteInertAndSealed"
  "VERCEL-ACCEPTANCE-05|./internal/vercelacceptance|TestExactSelectedTreesAreCompleteInertAndSealed"
  "VERCEL-ACCEPTANCE-06|./internal/vercelacceptance|TestExactSelectedTreesAreCompleteInertAndSealed"
  "VERCEL-ACCEPTANCE-07|./internal/vercelacceptance|TestExactSelectedTreesAreCompleteInertAndSealed"
  "VERCEL-ACCEPTANCE-08|./internal/vercelacceptance|TestCanonicalRuntimeContractHasFreshExactCodexPreflight"
  "VERCEL-ACCEPTANCE-09|./internal/codex|TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly"
  "VERCEL-ACCEPTANCE-10|./internal/opencode|TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly"
  "VERCEL-ACCEPTANCE-11|./internal/claudecode|TestVercelFixtureProjectsNineCompleteNativeSkillTreesReversibly"
  "VERCEL-ACCEPTANCE-12|./internal/capabilitypack|TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe"
  "VERCEL-ACCEPTANCE-13|./internal/vercelacceptance|TestCanonicalRuntimeContractHasFreshExactCodexPreflight"
  "VERCEL-ACCEPTANCE-14|./internal/opencodesmoke|TestPreflightEveryVercelModeFailsBeforeHostEffects"
  "VERCEL-ACCEPTANCE-15|./internal/claudesmoke|TestVercelRuntimeEvidenceCoversExactTwentyEightModesSafely"
  "VERCEL-ACCEPTANCE-16|./internal/capabilitypack|TestRuntimeEvidenceIsTriStateDeterministicAndSecretSafe"
  "VERCEL-ACCEPTANCE-20|./internal/vercelacceptance|TestExactSelectedTreesAreCompleteInertAndSealed"
  "VERCEL-ACCEPTANCE-21|./internal/tools/syncpacksource|TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope"
  "VERCEL-ACCEPTANCE-22|./internal/tools/syncpacksource|TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope"
  "VERCEL-ACCEPTANCE-23|./internal/ci|TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated"
  "VERCEL-ACCEPTANCE-24|./internal/vercelacceptance|TestCanonicalCohortReportAndDeterministicRerun"
)

normalize() {
  sed -E '/^ok[[:space:]]/d; s/ \([0-9.]+s\)$/ (duration)/' | LC_ALL=C sort
}

for mapping in "${mappings[@]}"; do
  IFS='|' read -r row package test <<<"$mapping"
  for rerun in first second; do
    raw="$work/$row.$rerun.raw"
    normalized="$work/$row.$rerun.txt"
    if ! go test "$package" -run "^${test}$" -count=1 -v >"$raw" 2>&1; then
      cat "$raw" >&2
      echo "Vercel acceptance foundation failed: $row $package/$test" >&2
      exit 1
    fi
    if [[ "$(grep -Fxc "=== RUN   $test" "$raw")" -ne 1 ||
          "$(grep -Ec "^--- PASS: $test \\([0-9.]+s\\)$" "$raw")" -ne 1 ]]; then
      echo "Vercel acceptance foundation lacks unique RUN/PASS proof: $row $package/$test" >&2
      exit 1
    fi
    normalize <"$raw" >"$normalized"
  done
  if ! cmp -s "$work/$row.first.txt" "$work/$row.second.txt"; then
    echo "Vercel acceptance foundation rerun changed: $row $package/$test" >&2
    exit 1
  fi
  if [[ -n "$evidence_dir" ]]; then
    cp "$work/$row.first.txt" "$evidence_dir/$row.first.txt"
    cp "$work/$row.second.txt" "$evidence_dir/$row.second.txt"
  fi
done

if [[ -n "$evidence_dir" ]]; then
  observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '{"schema_version":1,"matrix_version":"vercel-acceptance-v1","candidate_sha":"%s","fixture_sha256":"%s","run_id":"%s","observed_at":"%s"}\n' \
    "$candidate_sha" \
    "6914589e3899ae238c30a0d87c297ef101c87a01d63e160efc3dcfab27676ab7" \
    "$run_id" \
    "$observed_at" \
    >"$evidence_dir/identity.json"
fi
