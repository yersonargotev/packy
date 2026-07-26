# Read-only premerge proof

Use this boundary only while delivering an approved issue that changes the v3
Pack Sync workflow or its external classification/publication seams. It is not
a synchronization request, retry, success result, or substitute for a later
protected-`main` dispatch.

## Admission

1. Render and validate the exact canonical v3 `register_bundle` request from
   remote `main` exactly as required by [REQUESTS.md](REQUESTS.md).
2. Push the reviewed issue branch named
   `feat|fix|chore/issue-N-<slug>`.
3. Reobserve remote `main`, the target `sync/<pack-id>` ref, open pull request,
   and production queue. Record their exact pre-run identities.
4. From the reviewed branch bytes, dispatch exactly once:

   ```sh
   .agents/skills/sync-pack-source/scripts/prepare.sh \
     feat/issue-N-slug canonical-request.json
   ```

The helper adds only the transport-only `prepare_only=true` input. It does not
change the canonical request or digest. A preparation run has branch-scoped
concurrency and must never be attached to, retried as, or reported as a
production Pack Sync operation.

## Required evidence

Monitor the exact run without rerunning a semantic failure. Require Inspect,
Classify, Validate, and Prepare to succeed and Publish to be skipped. Download
the exact artifacts and validate their runtime contracts. The preparation
artifact must bind the exact request/plan/base/member/result identities and
state:

- `observations_stable: true`;
- `repository_mutated: false`;
- `decision_ready: false`;
- `upstream_content_executed: false`.

Reobserve the target ref and pull request after the run. Their identities and
comments must be unchanged from the pre-run observations. Ordinary Actions run
artifacts are the only admitted remote writes.

Any model, acquisition, validation, proposal, freshness, ownership, or artifact
failure returns to the issue's implementation-review loop before merge. Only an
independently reviewed final branch head with green local validation, PR CI,
and this exact live proof may be merged.
