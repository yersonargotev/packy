#!/usr/bin/env bash
set -euo pipefail

boundary=
repository=
tag=
release_commit=
verifier=
ref_output=
candidate=
provenance=
state_output=
decision_output=
expected_body=
attestation=
upload_asset=
mode=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --boundary) boundary="${2:-}"; shift 2 ;;
    --repository) repository="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --release-commit) release_commit="${2:-}"; shift 2 ;;
    --verifier) verifier="${2:-}"; shift 2 ;;
    --ref-output) ref_output="${2:-}"; shift 2 ;;
    --candidate) candidate="${2:-}"; shift 2 ;;
    --provenance) provenance="${2:-}"; shift 2 ;;
    --state-output) state_output="${2:-}"; shift 2 ;;
    --decision-output) decision_output="${2:-}"; shift 2 ;;
    --expected-body) expected_body="${2:-}"; shift 2 ;;
    --attestation) attestation="${2:-}"; shift 2 ;;
    --upload-asset) upload_asset="${2:-}"; shift 2 ;;
    --mode) mode="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ -n "$boundary" && "$repository" == */* && "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ &&
   "$release_commit" =~ ^[0-9a-f]{40}$ && -x "$verifier" && -n "$ref_output" ]] || {
  echo 'boundary, repository, valid v0 tag, release commit, executable verifier, and ref output are required' >&2
  exit 2
}
if [[ -n "$candidate$provenance$state_output$decision_output$expected_body$attestation$mode" ]]; then
  [[ -n "$candidate" && -n "$provenance" && -n "$state_output" && -n "$decision_output" &&
     -f "$expected_body" && -f "$attestation" && "$mode" =~ ^(current|draft|published)$ ]] || {
    echo 'candidate, provenance, state and decision outputs, expected body, attestation, and current, draft, or published mode must be supplied together' >&2
    exit 2
  }
fi
[[ "$boundary" != 'asset upload' || -n "$upload_asset" ]] || {
  echo 'asset upload boundary requires the exact upload asset name' >&2
  exit 2
}

resolve_ref_commit() {
  local ref="$1" object object_type object_sha hops=0
  object="$(gh api "repos/$repository/git/ref/$ref" --jq '[.object.type,.object.sha]|@tsv')"
  IFS=$'\t' read -r object_type object_sha <<< "$object"
  while [[ "$object_type" == tag && "$hops" -lt 8 ]]; do
    object="$(gh api "repos/$repository/git/tags/$object_sha" --jq '[.object.type,.object.sha]|@tsv')"
    IFS=$'\t' read -r object_type object_sha <<< "$object"
    hops=$((hops + 1))
  done
  if [[ "$object_type" == commit && "$object_sha" =~ ^[0-9a-f]{40}$ ]]; then
    printf '%s\n' "$object_sha"
  else
    printf '<unresolved>'
  fi
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! ref_error="$("$script_dir/verify-release-ref-state.sh" \
  --repository "$repository" --tag "$tag" --release-commit "$release_commit" \
  --verifier "$verifier" --output "$ref_output" 2>&1)"; then
  observed_tag="$(resolve_ref_commit "tags/$tag" || printf '<unresolved>')"
  observed_main="$(resolve_ref_commit heads/main || printf '<unresolved>')"
  observed_ancestry="$(gh api "repos/$repository/compare/$release_commit...$observed_main" --jq .status 2>/dev/null || printf '<unresolved>')"
  echo "$boundary denied: expected release_commit=$release_commit tag_commit=$release_commit in protected main; observed tag_commit=$observed_tag main_commit=$observed_main ancestry=$observed_ancestry; $ref_error" >&2
  exit 1
fi

if [[ -n "$state_output" ]]; then
  owner="${repository%%/*}"
  repository_name="${repository#*/}"
  # GraphQL variables expand server-side.
  # shellcheck disable=SC2016
  expected_candidate="$(jq -r '.id // "<unresolved>"' "$candidate" 2>/dev/null || printf '<unresolved>')"
  if ! release_database_id="$(gh api graphql \
    -f query='query($owner:String!,$repository:String!,$tag:String!){repository(owner:$owner,name:$repository){release(tagName:$tag){databaseId}}}' \
    -f owner="$owner" -f repository="$repository_name" -f tag="$tag" \
    --jq '.data.repository.release.databaseId // ""')"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=unreadable" >&2
    exit 1
  fi
  if [[ -z "$release_database_id" ]]; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=missing" >&2
    exit 1
  fi
  if [[ ! "$release_database_id" =~ ^[0-9]+$ ]]; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release identity=invalid" >&2
    exit 1
  fi
  observed_release="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary.XXXXXX")"
  observed_policy="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-policy.XXXXXX")"
  observed_body="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-body.XXXXXX")"
  observed_metadata="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-metadata.XXXXXX")"
  trap 'rm -f "$observed_release" "$observed_policy" "$observed_body" "$observed_metadata"' EXIT
  if ! gh release view "$tag" --repo "$repository" --json tagName,isDraft,body,assets > "$observed_release"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release=unreadable" >&2
    exit 1
  fi
  if ! gh api "repos/$repository/releases/$release_database_id" --jq 'if type=="object" and (.tag_name|type)=="string" and (.draft|type)=="boolean" and (.prerelease|type)=="boolean" and (.immutable|type)=="boolean" and (.author.login|type)=="string" and ((.published_at==null) or ((.published_at|type)=="string")) then {tag_name,draft,prerelease,immutable,published_at,author:.author.login} else error("unrecognized release policy shape") end' > "$observed_policy"; then
    echo "$boundary denied: expected target_commit=$release_commit bot-authored release policy for tag=$tag; observed release policy=unreadable" >&2
    exit 1
  fi
  jq -j .body "$observed_release" > "$observed_body"
  awk '/^<!-- packy-release-metadata$/{capture=1;next}/^-->$/{capture=0}capture' \
    "$observed_body" > "$observed_metadata"
  if ! cmp "$expected_body" "$observed_body" >/dev/null; then
    expected_body_sha="$(sha256sum "$expected_body" | awk '{print $1}')"
    observed_body_sha="$(sha256sum "$observed_body" | awk '{print $1}')"
    observed_candidate="$(jq -r '.candidate_id // "<missing>"' "$observed_metadata" 2>/dev/null || printf '<invalid>')"
    observed_target="$(jq -r '.target_commit // "<missing>"' "$observed_metadata" 2>/dev/null || printf '<invalid>')"
    echo "$boundary denied: expected target_commit=$release_commit release_body_sha256=$expected_body_sha; observed candidate_id=$observed_candidate target_commit=$observed_target release_body_sha256=$observed_body_sha" >&2
    exit 1
  fi
  if ! jq --slurpfile metadata "$observed_metadata" \
    --slurpfile candidate "$candidate" \
    --arg repository "$repository" \
    '{candidate_id:($metadata[0].candidate_id // ""),provenance:($metadata[0].provenance // {}),
      version:.tagName,repository:$repository,ref:$candidate[0].ref,
      target_commit:($metadata[0].target_commit // ""),workflow:$candidate[0].workflow,
      workflow_sha:$candidate[0].workflow_sha,
      release_notes_sha256:$candidate[0].release_notes_sha256,draft:.isDraft,
      assets:[.assets[]|select(.name!="attestation.bundle.jsonl")|{name,digest}]}' \
    "$observed_release" > "$state_output"; then
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed release metadata=invalid" >&2
    exit 1
  fi
  verify_mode="$mode"
  if [[ "$verify_mode" == current ]]; then
    if [[ "$(jq -r .draft "$state_output")" == true ]]; then verify_mode=draft; else verify_mode=published; fi
  fi
  expected_draft="$(jq -r .isDraft "$observed_release")"
  published_timestamp_valid=true
  if [[ "$verify_mode" == published ]] &&
     ! jq -e '.published_at | type=="string" and (fromdateiso8601? != null)' "$observed_policy" >/dev/null; then
    published_timestamp_valid=false
  fi
  if ! jq -e --arg tag "$tag" --argjson draft "$expected_draft" '
    .tag_name==$tag and .draft==$draft and .prerelease==false and
    .author=="github-actions[bot]"
  ' "$observed_policy" >/dev/null ||
     [[ "$verify_mode" == published && "$(jq -r .immutable "$observed_policy")" != true ]] ||
     [[ "$published_timestamp_valid" != true ]]; then
    observed_author="$(jq -r '.author // "<missing>"' "$observed_policy" 2>/dev/null || printf '<invalid>')"
    observed_immutable="$(jq -r '.immutable // "<missing>"' "$observed_policy" 2>/dev/null || printf '<invalid>')"
    observed_published_at="$(jq -r '.published_at // "<missing>"' "$observed_policy" 2>/dev/null || printf '<invalid>')"
    echo "$boundary denied: expected target_commit=$release_commit tag=$tag bot-authored non-prerelease release and immutable RFC3339-dated published state; observed author=$observed_author immutable=$observed_immutable published_at=$observed_published_at mode=$verify_mode" >&2
    exit 1
  fi
  if ! state_error="$("$verifier" verify-state --candidate "$candidate" --provenance "$provenance" \
    --state "$state_output" --mode "$verify_mode" > "$decision_output" 2>&1)"; then
    observed_candidate="$(jq -r '.candidate_id // "<missing>"' "$state_output" 2>/dev/null || printf '<unresolved>')"
    observed_target="$(jq -r '.target_commit // "<missing>"' "$state_output" 2>/dev/null || printf '<unresolved>')"
    echo "$boundary denied: expected candidate_id=$expected_candidate target_commit=$release_commit; observed candidate_id=$observed_candidate target_commit=$observed_target; $state_error" >&2
    exit 1
  fi
  attestation_name="$(basename "$attestation")"
  expected_assets="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-assets.XXXXXX")"
  observed_assets="$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/release-boundary-observed-assets.XXXXXX")"
  jq -r '.subjects[].name' "$candidate" > "$expected_assets"
  printf '%s\n' "$attestation_name" >> "$expected_assets"
  sort -u -o "$expected_assets" "$expected_assets"
  jq -r '.assets[].name' "$observed_release" | sort > "$observed_assets"
  duplicates="$(uniq -d "$observed_assets")"
  extras="$(comm -13 "$expected_assets" "$observed_assets")"
  if [[ -n "$duplicates$extras" ]]; then
    echo "$boundary denied: expected unique sealed asset names; observed duplicates=${duplicates:-none} extras=${extras:-none}" >&2
    exit 1
  fi
  observed_attestation_digest="$(jq -r --arg name "$attestation_name" '.assets[]|select(.name==$name)|.digest' "$observed_release")"
  expected_attestation_digest="sha256:$(sha256sum "$attestation" | awk '{print $1}')"
  if [[ -n "$observed_attestation_digest" && "$observed_attestation_digest" != "$expected_attestation_digest" ]]; then
    echo "$boundary denied: expected $attestation_name digest=$expected_attestation_digest; observed digest=$observed_attestation_digest" >&2
    exit 1
  fi
  decision="$(jq -r .decision "$decision_output")"
  missing_count="$(jq -r '.missing_assets|length' "$decision_output")"
  case "$boundary" in
    'asset upload')
      if [[ "$upload_asset" == "$attestation_name" ]]; then
        [[ -z "$observed_attestation_digest" ]] || {
          echo "$boundary denied: expected upload asset $upload_asset to be missing; observed exact asset already present" >&2
          exit 1
        }
      else
        jq -e --arg name "$upload_asset" '.missing_assets|any(.name==$name)' "$decision_output" >/dev/null || {
          echo "$boundary denied: expected upload asset $upload_asset in fresh missing subset; observed missing subset excludes it" >&2
          exit 1
        }
      fi
      ;;
    publication)
      if [[ "$mode" != current ]]; then
        [[ "$missing_count" == 0 && "$observed_attestation_digest" == "$expected_attestation_digest" &&
           "$(wc -l < "$observed_assets" | tr -d ' ')" == "$(wc -l < "$expected_assets" | tr -d ' ')" &&
           ( "$verify_mode" == draft && "$decision" == publish-draft ||
             "$verify_mode" == published && "$decision" == continue-published ) ]] || {
          echo "$boundary denied: expected complete draft publication or exact published continuation; observed mode=$verify_mode decision=$decision missing_assets=$missing_count attestation_digest=${observed_attestation_digest:-missing}" >&2
          exit 1
        }
      fi
      ;;
    'Homebrew mutation')
      [[ "$verify_mode" == published && "$decision" == continue-published && "$missing_count" == 0 &&
         "$observed_attestation_digest" == "$expected_attestation_digest" &&
         "$(wc -l < "$observed_assets" | tr -d ' ')" == "$(wc -l < "$expected_assets" | tr -d ' ')" ]] || {
        echo "$boundary denied: expected exact published release set; observed decision=$decision missing_assets=$missing_count attestation_digest=${observed_attestation_digest:-missing}" >&2
        exit 1
      }
      ;;
  esac
fi
