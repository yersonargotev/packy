#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

report_output=
report_repository=
report_commit=
report_workflow_digest=
report_run_id=
while (($#)); do
  case "$1" in
    --report-output) report_output="${2:-}"; shift 2 ;;
    --repository) report_repository="${2:-}"; shift 2 ;;
    --commit) report_commit="${2:-}"; shift 2 ;;
    --workflow-digest) report_workflow_digest="${2:-}"; shift 2 ;;
    --run-id) report_run_id="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [[ -n "$report_output" ]]; then
  [[ "$report_output" = /* && ! -e "$report_output" ]] || {
    echo "acceptance report output must be a new absolute path" >&2
    exit 2
  }
  [[ -n "$report_repository" && "$report_commit" =~ ^[0-9a-f]{40}$ &&
     "$report_workflow_digest" =~ ^[0-9a-f]{64}$ && -n "$report_run_id" ]] || {
    echo "acceptance report requires repository, exact commit, workflow digest, and run ID" >&2
    exit 2
  }
elif [[ -n "$report_repository$report_commit$report_workflow_digest$report_run_id" ]]; then
  echo "acceptance report identity is not accepted without --report-output" >&2
  exit 2
fi

declare -a mapping_rows=() mapping_packages=() mapping_tests=()
declare -a promotion_rows=() promotion_packages=() promotion_tests=() packages=()
declare -a exact_packages=() exact_tests=() exact_outputs=()

map_row() {
  local row="${1-}" package="${2-}"
  shift 2 || true
  if [[ ! "$row" =~ ^[0-9]+$ || ! "$package" =~ ^\./internal/[A-Za-z0-9_./-]+$ || "$#" -eq 0 ]]; then
    echo "malformed Addy acceptance mapping: row=${row:-<empty>} package=${package:-<empty>}" >&2
    return 1
  fi
  local test existing
  for test in "$@"; do
    if [[ ! "$test" =~ ^Test[A-Za-z0-9_]+$ ]]; then
      echo "malformed Addy acceptance mapping: row $row has invalid test ${test:-<empty>}" >&2
      return 1
    fi
    mapping_rows+=("$row")
    mapping_packages+=("$package")
    mapping_tests+=("$test")
  done
  for existing in "${packages[@]-}"; do
    [[ "$existing" == "$package" ]] && return 0
  done
  packages+=("$package")
}

map_promotion_row() {
  local row="${1-}" package="${2-}" test="${3-}"
  if [[ ! "$row" =~ ^ADDY-CLAUDE-PROMOTION-ROW-[0-9]{2}$ || ! "$package" =~ ^\./internal/[A-Za-z0-9_./-]+$ || ! "$test" =~ ^Test[A-Za-z0-9_]+$ || "$#" -ne 3 ]]; then
    echo "malformed Addy promotion mapping: row=${row:-<empty>} package=${package:-<empty>} test=${test:-<empty>}" >&2
    return 1
  fi
  promotion_rows+=("$row")
  promotion_packages+=("$package")
  promotion_tests+=("$test")
  local existing
  for existing in "${packages[@]-}"; do
    [[ "$existing" == "$package" ]] && return 0
  done
  packages+=("$package")
}

# Keep these 26 declarations explicit: they are the stable reverse trace from
# the Addy acceptance matrix to the exact top-level tests that prove each row.
map_row 1 ./internal/addyacceptance TestExactUpstreamArchiveInventoryAndSupportRemainInert
map_row 2 ./internal/addyacceptance TestUnsafeArchiveTwinBlocksAndCleansBeforeExecution TestExactUpstreamArchiveInventoryAndSupportRemainInert
map_row 4 ./internal/packsync TestLoadConfigRejectsPathUnsafeSourceIDsAndSharedBindings TestValidatePreconditionsRejectsUnrelatedSourceGenerationWithoutMutation
map_row 6 ./internal/addyacceptance TestCanonicalInventoryAndDeterminism TestOneFactNegativeTwinBlocksCompleteInventory
map_row 7 ./internal/capabilitypack TestDiscoverRejectsInvalidManifestV2Contracts TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp
map_row 8 ./internal/addyacceptance TestExactUpstreamArchiveInventoryAndSupportRemainInert TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent
map_row 9 ./internal/ci TestPackSourceV2SchemasAcceptCanonicalRuntimeContracts TestSynchronizationSchemasAcceptCanonicalRuntimeArtifacts
map_row 10 ./internal/packclassification TestHumanClassificationRequiresInspectionThenBoundEvidenceDispatch
map_row 11 ./internal/addyacceptance TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings
map_row 12 ./internal/addyacceptance TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent
map_row 13 ./internal/addyacceptance TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent
map_row 14 ./internal/capabilitypack TestCompleteAddyCollisionBlocksUntilExactSurfaceAliasReplans
map_row 15 ./internal/capabilitypack TestCompleteAddyCohortStalePreflightAndAtomicFailureRequireFreshRecovery
map_row 16 ./internal/capabilitypack TestCompleteAddyDualSurfaceFailurePreservesAuthorizedOtherSurface TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor
map_row 17 ./internal/capabilitypack TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp
map_row 18 ./internal/capabilitypack TestCompleteAddyAtomicAdapterFailureRecordsAttemptAndRequiresFreshRecoveryPlan
map_row 19 ./internal/capabilitypack TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct
map_row 19 ./internal/cli TestPackStatusJSONRequireEmitsDocumentBeforeGateError TestPackStatusRequireUsableIsIndependentNonInteractiveGate
map_row 20 ./internal/capabilitypack TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct
map_row 21 ./internal/capabilitypack TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp TestUpdateRejectsStaleCatalogAndExactPlanApproval
map_row 22 ./internal/capabilitypack TestCompleteAddyExactOwnershipRemovalBlocksDriftWithoutEffects TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor
map_row 23 ./internal/tools/syncpacksource TestAddyRegistrationTracerProvesExactEndToEndAdmission
map_row 23 ./internal/packsync TestCheckSealsAbsentSourceRegistrationWithoutPersistingIt TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically
map_row 24 ./internal/packsync TestCheckRejectsRegistrationWithExistingSourceOrBindingOwner TestApplyCommitsRegistrationConfigurationLockAndContributionAtomically
map_row 24 ./internal/tools/syncpacksource TestAddyRegistrationTracerProvesExactEndToEndAdmission
map_row 24 ./internal/ci TestPackSourceV2RegistrationSemanticAndNullArrayValidation TestSyncWorkflowIsManualPinnedLeastPrivilegeAndPhaseSeparated

# These identities are the immutable Addy 1.1.0 promotion matrix. Their
# semantics live in internal/addyacceptance; this adapter only provides the
# stable reverse trace to exact top-level tests.
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-01 ./internal/addyacceptance TestAddyPromotionIndependentInputs
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-02 ./internal/addyacceptance TestAddyPromotionIndependentInputs
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-03 ./internal/addyacceptance TestAddyPromotionIndependentInputs
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-04 ./internal/addyacceptance TestAddyPromotionAuthorityFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-05 ./internal/addyacceptance TestAddyPromotionAuthorityFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-06 ./internal/addyacceptance TestAddyPromotionAuthorityFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-07 ./internal/addyacceptance TestAddyPromotionLifecycleFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-08 ./internal/addyacceptance TestAddyPromotionLifecycleFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-09 ./internal/addyacceptance TestAddyPromotionLifecycleFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-10 ./internal/addyacceptance TestAddyPromotionLifecycleFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-11 ./internal/addyacceptance TestAddyPromotionRealHostFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-12 ./internal/addyacceptance TestAddyPromotionRealHostFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-13 ./internal/addyacceptance TestAddyPromotionEvidenceFoundations
map_promotion_row ADDY-CLAUDE-PROMOTION-ROW-14 ./internal/addyacceptance TestAddyPromotionEvidenceFoundations

rows_for_package() {
  local package="$1" result="" i row
  for ((i = 0; i < ${#mapping_rows[@]}; i++)); do
    [[ "${mapping_packages[i]}" == "$package" ]] || continue
    row="${mapping_rows[i]}"
    [[ ", $result, " == *", $row, "* ]] || result="${result:+$result, }$row"
  done
  printf '%s' "$result"
}

rows_for_test() {
  local package="$1" test="$2" result="" i row
  for ((i = 0; i < ${#mapping_rows[@]}; i++)); do
    [[ "${mapping_packages[i]}" == "$package" && "${mapping_tests[i]}" == "$test" ]] || continue
    row="${mapping_rows[i]}"
    [[ ", $result, " == *", $row, "* ]] || result="${result:+$result, }$row"
  done
  printf '%s' "$result"
}

tests_for_package() {
  local package="$1" result="" i test
  for ((i = 0; i < ${#mapping_tests[@]}; i++)); do
    [[ "${mapping_packages[i]}" == "$package" ]] || continue
    test="${mapping_tests[i]}"
    [[ "|$result|" == *"|$test|"* ]] || result="${result:+$result|}$test"
  done
  for ((i = 0; i < ${#promotion_tests[@]}; i++)); do
    [[ "${promotion_packages[i]}" == "$package" ]] || continue
    test="${promotion_tests[i]}"
    [[ "|$result|" == *"|$test|"* ]] || result="${result:+$result|}$test"
  done
  printf '%s' "$result"
}

promotion_rows_for_test() {
  local package="$1" test="$2" result="" i
  for ((i = 0; i < ${#promotion_rows[@]}; i++)); do
    [[ "${promotion_packages[i]}" == "$package" && "${promotion_tests[i]}" == "$test" ]] || continue
    [[ "|$result|" == *"|${promotion_rows[i]}|"* ]] || result="${result:+$result|}${promotion_rows[i]}"
  done
  printf '%s' "$result"
}

evidence_for_exact_test() {
  local package="$1" test="$2" i
  for ((i = 0; i < ${#exact_tests[@]}; i++)); do
    if [[ "${exact_packages[i]}" == "$package" && "${exact_tests[i]}" == "$test" ]]; then
      printf '%s' "${exact_outputs[i]}"
      return 0
    fi
  done
  return 1
}

# Prevalidate the complete mapping before any test execution. Only exact
# top-level names emitted by -list count; go test status text is ignored.
validation_failed=0
for package in "${packages[@]}"; do
  if ! listed="$(go test "$package" -list '^Test[A-Za-z0-9_]*$' 2>&1)"; then
    echo "Addy acceptance package validation failed for $package (rows $(rows_for_package "$package"))" >&2
    printf '%s\n' "$listed" >&2
    validation_failed=1
    continue
  fi
  available="$(printf '%s\n' "$listed" | sed -n '/^Test[A-Za-z0-9_]*$/p')"
  tests="$(tests_for_package "$package")"
  while IFS= read -r test; do
    grep -Fxq "$test" <<<"$available" && continue
    rows="$(rows_for_test "$package" "$test")"
    promotion="$(promotion_rows_for_test "$package" "$test")"
    if [[ -n "$rows" ]]; then
      echo "Addy acceptance mapping references missing exact test $package/$test (rows $rows)" >&2
    else
      echo "Addy promotion mapping references missing exact test $package/$test (promotion ${promotion//|/, })" >&2
    fi
    validation_failed=1
  done < <(tr '|' '\n' <<<"$tests")
done
((validation_failed == 0)) || exit 1

execution_failed=0
for package in "${packages[@]}"; do
  tests="$(tests_for_package "$package")"
  echo "==> Addy acceptance package $package (rows $(rows_for_package "$package"))"
  if output="$(go test "$package" -run "^(${tests})$" -count=1 2>&1)"; then
    printf '%s\n' "$output"
    continue
  fi
  printf '%s\n' "$output" >&2
  failed_tests="$(printf '%s\n' "$output" | sed -n 's/^--- FAIL: \(Test[A-Za-z0-9_]*\) .*/\1/p' | sort -u)"
  if [[ -n "$failed_tests" ]]; then
    while IFS= read -r test; do
      rows="$(rows_for_test "$package" "$test")"
      promotion="$(promotion_rows_for_test "$package" "$test")"
      if [[ -n "$rows" ]]; then
        echo "Addy acceptance test failed: $package/$test (rows $rows)" >&2
      else
        echo "Addy promotion test failed: $package/$test (promotion ${promotion//|/, })" >&2
      fi
    done <<<"$failed_tests"
  else
    echo "Addy acceptance package execution failed for $package (rows $(rows_for_package "$package"))" >&2
  fi
  execution_failed=1
done
((execution_failed == 0)) || exit "$execution_failed"

if [[ -n "$report_output" ]]; then
  for ((i = 0; i < ${#promotion_tests[@]}; i++)); do
    package="${promotion_packages[i]}"
    test="${promotion_tests[i]}"
    if evidence_for_exact_test "$package" "$test" >/dev/null 2>&1; then
      continue
    fi
    if ! exact_output="$(go test "$package" -run "^${test}$" -count=1 -v 2>&1)"; then
      printf '%s\n' "$exact_output" >&2
      echo "Addy promotion exact owning test failed: $package/$test" >&2
      exit 1
    fi
    if [[ "$(grep -Fxc "=== RUN   $test" <<<"$exact_output")" -ne 1 ||
          "$(grep -Ec "^--- PASS: $test \\([0-9.]+s\\)$" <<<"$exact_output")" -ne 1 ]]; then
      echo "Addy promotion exact owning test output is missing its unique RUN/PASS proof: $package/$test" >&2
      exit 1
    fi
    sanitized="$(printf '%s\n' "$exact_output" |
      sed -E '/^ok[[:space:]]/d; s/ \\([0-9.]+s\\)$/ (duration)/')"
    exact_packages+=("$package")
    exact_tests+=("$test")
    exact_outputs+=("$sanitized")
  done

  report_args=(
    --write-acceptance-report
    --output "$report_output"
    --repository "$report_repository"
    --acceptance-commit "$report_commit"
    --workflow-digest "$report_workflow_digest"
    --run-id "$report_run_id"
  )
  for ((i = 0; i < ${#promotion_rows[@]}; i++)); do
    material="${promotion_rows[i]}"$'\t'"${promotion_packages[i]}"$'\t'"${promotion_tests[i]}"$'\t'"$report_repository"$'\t'"$report_commit"$'\t'"$report_workflow_digest"$'\t'"$report_run_id"$'\t'"$(evidence_for_exact_test "${promotion_packages[i]}" "${promotion_tests[i]}")"
    if command -v sha256sum >/dev/null 2>&1; then
      evidence_digest="$(printf '%s' "$material" | sha256sum | awk '{print $1}')"
    else
      evidence_digest="$(printf '%s' "$material" | shasum -a 256 | awk '{print $1}')"
    fi
    report_args+=(--acceptance-row "${promotion_rows[i]}"$'\t'"${promotion_packages[i]}"$'\t'"${promotion_tests[i]}"$'\t'"passed"$'\t'"$evidence_digest")
  done
  go run ./internal/tools/addypromotiongate "${report_args[@]}"
fi
