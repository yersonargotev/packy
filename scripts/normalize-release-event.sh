#!/usr/bin/env bash

set -euo pipefail
[[ "$GITHUB_REPOSITORY" == yersonargotev/packy ]] || {
  echo 'release workflow is sealed to yersonargotev/packy' >&2; exit 1;
}
resolve_ref_commit() {
  local ref="$1" object object_type object_sha hops=0
  object="$(gh api "repos/$GITHUB_REPOSITORY/git/ref/$ref" --jq '[.object.type,.object.sha]|@tsv')"
  IFS=$'\t' read -r object_type object_sha <<< "$object"
  while [[ "$object_type" == tag && "$hops" -lt 8 ]]; do
    object="$(gh api "repos/$GITHUB_REPOSITORY/git/tags/$object_sha" --jq '[.object.type,.object.sha]|@tsv')"
    IFS=$'\t' read -r object_type object_sha <<< "$object"
    hops=$((hops + 1))
  done
  [[ "$object_type" == commit && "$object_sha" =~ ^[0-9a-f]{40}$ ]] || {
    echo "ref $ref did not peel uniquely to one commit" >&2; return 1;
  }
  printf '%s\n' "$object_sha"
}
requested_mode= tag=
case "$EVENT_NAME" in
  push)
    [[ "$EVENT_REF_TYPE" == tag && "$EVENT_REF" == "refs/tags/$EVENT_REF_NAME" &&
       -z "$INPUT_TAG" && -z "$INPUT_DRY_RUN" ]] || {
      echo 'ambiguous tag-push event payload' >&2; exit 1;
    }
    tag="$EVENT_REF_NAME"
    ;;
  workflow_dispatch)
    [[ "$EVENT_REF" == refs/heads/main && "$EVENT_REF_TYPE" == branch &&
       "$EVENT_REF_NAME" == main && -n "$INPUT_TAG" ]] || {
      echo 'manual release dispatch must run from protected main with one tag' >&2; exit 1;
    }
    case "$INPUT_DRY_RUN" in
      true) requested_mode=dry-run ;;
      false) requested_mode=recovery ;;
      *) echo 'manual dry_run must be exactly true or false' >&2; exit 1 ;;
    esac
    tag="$INPUT_TAG"
    ;;
  *)
    echo "unsupported release event: $EVENT_NAME" >&2
    exit 1
    ;;
esac
[[ "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ ]] || {
  echo 'tag must match the exact valid-v0 namespace v0.x.y' >&2; exit 1;
}

git fetch --force origin main 'refs/tags/*:refs/tags/*'
main_commit="$(resolve_ref_commit heads/main)"
tag_commit="$(resolve_ref_commit "tags/$tag")"
[[ "$(git rev-parse --verify "refs/tags/$tag^{commit}")" == "$tag_commit" ]] || {
  echo 'fetched tag and remote tag target disagree' >&2; exit 1;
}
git merge-base --is-ancestor "$tag_commit" "$main_commit" || {
  echo "$tag target is not in protected-main history" >&2; exit 1;
}
latest_version="$(
  { git tag --list; gh api "repos/$GITHUB_REPOSITORY/releases?per_page=100" --jq '.[].tag_name'; } |
    awk '/^v0\.[0-9]+\.[0-9]+$/' | { grep -vxF "$tag" || true; } | sort -Vu | tail -n1
)"
owner="${GITHUB_REPOSITORY%%/*}"
repository="${GITHUB_REPOSITORY#*/}"
# GraphQL variables expand server-side.
# shellcheck disable=SC2016
release_id="$(gh api graphql \
  -f query='query($owner:String!,$repository:String!,$tag:String!){repository(owner:$owner,name:$repository){release(tagName:$tag){id}}}' \
  -f owner="$owner" -f repository="$repository" -f tag="$tag" \
  --jq '.data.repository.release.id // ""')"
release_present=false release_state=absent release_tag= candidate_locator=
printf '{}\n' > "$RUNNER_TEMP/admission-metadata.json"
if [[ -n "$release_id" ]]; then
  gh release view "$tag" --repo "$GITHUB_REPOSITORY" \
    --json tagName,isDraft,body > "$RUNNER_TEMP/admission-release.json"
  release_present=true
  release_tag="$(jq -r .tagName "$RUNNER_TEMP/admission-release.json")"
  release_state="$(jq -r 'if .isDraft then "draft" else "published" end' "$RUNNER_TEMP/admission-release.json")"
  jq -j .body "$RUNNER_TEMP/admission-release.json" |
    awk '/^<!-- packy-release-metadata$/{capture=1;next}/^-->$/{capture=0}capture' \
    > "$RUNNER_TEMP/admission-metadata.json"
  candidate_locator="packy-release-$tag"
fi
tag_in_main=false
if git merge-base --is-ancestor "$tag_commit" "$main_commit"; then tag_in_main=true; fi
jq -n \
  --arg event_name "$EVENT_NAME" --arg event_ref "$EVENT_REF" \
  --arg requested_mode "$requested_mode" --arg repository "$GITHUB_REPOSITORY" \
  --arg tag "$tag" --arg tag_commit "$tag_commit" --arg event_commit "$EVENT_SHA" \
  --arg current_main "$main_commit" --arg latest_version "$latest_version" \
  --argjson tag_in_main "$tag_in_main" --argjson release_present "$release_present" \
  --arg release_state "$release_state" --arg release_tag "$release_tag" \
  --arg candidate_locator "$candidate_locator" \
  --slurpfile release_metadata "$RUNNER_TEMP/admission-metadata.json" \
  '($release_metadata |
    if length==0 then {} elif length==1 then .[0]
    else error("release metadata must contain at most one JSON document") end) as $metadata |
   {event_name:$event_name,event_ref:$event_ref,requested_mode:$requested_mode,
    repository:$repository,tag:$tag,tag_commit:$tag_commit,event_commit:$event_commit,
    current_main:$current_main,latest_version:$latest_version,tag_in_main:$tag_in_main,
    release_present:$release_present,release_state:$release_state,release_tag:$release_tag,
    release_commit:($metadata.target_commit // ""),
    release_schema_version:($metadata.schema_version // 0),
    release_candidate_id:($metadata.candidate_id // ""),
    release_attestation_source_ref:
      ($metadata.attestation_source_ref // $metadata.publication_plan.attestation_source_ref // ""),
    original_run_id:($metadata.source_run_id // $metadata.publication_plan.source_run_id // ""),
    candidate_locator:$candidate_locator}' \
  > "$RUNNER_TEMP/admission-observation.json"
release_candidate_adapter="${RELEASE_CANDIDATE_ADAPTER:-}"
if [[ -n "$release_candidate_adapter" ]]; then
  [[ "$release_candidate_adapter" == /* && -x "$release_candidate_adapter" ]] || {
    echo 'RELEASE_CANDIDATE_ADAPTER must be an absolute executable path' >&2
    exit 1
  }
  release_candidate_command=("$release_candidate_adapter")
else
  release_candidate_command=(go run ./internal/tools/releasecandidate)
fi
"${release_candidate_command[@]}" admit \
  --observation "$RUNNER_TEMP/admission-observation.json" > "$RUNNER_TEMP/admission.json"
git checkout --detach "$(jq -r .release_commit "$RUNNER_TEMP/admission.json")"
{
  echo "mode=$(jq -r .mode "$RUNNER_TEMP/admission.json")"
  echo "tag=$(jq -r .tag "$RUNNER_TEMP/admission.json")"
  echo "commit=$(jq -r .release_commit "$RUNNER_TEMP/admission.json")"
  echo "main_commit=$(jq -r .current_main "$RUNNER_TEMP/admission.json")"
  echo "source_ref=$(jq -r .attestation_source_ref "$RUNNER_TEMP/admission.json")"
  echo "original_run_id=$(jq -r '.original_run_id // ""' "$RUNNER_TEMP/admission.json")"
  echo "release_state=$(jq -r .release_state "$RUNNER_TEMP/admission.json")"
} >> "$GITHUB_OUTPUT"
