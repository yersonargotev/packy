# Read-only activation proof

Use this boundary only after the replacement workflow has merged while the old
workflow remains disabled. It is not a synchronization request, retry, success
result, or substitute for a later protected-`main` dispatch.

## Admission

1. Render and validate the exact canonical v3 `register_bundle` request from
   remote `main` exactly as required by [REQUESTS.md](REQUESTS.md).
2. Preserve the reviewed issue branch named `feat|fix|chore/issue-N-<slug>`
   at the merged implementation identity.
3. Reobserve remote `main`, the target `sync/<pack-id>` ref, open pull request,
   and production queue. Record their exact pre-run identities.
4. Enable the merged replacement only for this bounded proof. From the reviewed
   branch bytes, dispatch exactly once:

   ```sh
   .agents/skills/sync-pack-source/scripts/prepare.sh \
     feat/issue-N-slug canonical-request.json
   ```

The helper adds only the transport-only `prepare_only=true` input. It does not
change the canonical request or digest. A preparation run has branch-scoped
concurrency and must never be attached to, retried as, or reported as a
production Pack Sync operation. No protected-main publication dispatch is
allowed during proof. A failed proof returns the workflow to disabled
immediately.

## Required evidence

Monitor the exact run without rerunning a semantic failure. Require Inspect,
Classify, and Prepare to succeed and Publish to be skipped. Download
the exact artifacts and validate their runtime contracts. The preparation
artifact must bind the exact request/plan/base/member/result identities and
state:

- `observations_stable: true`;
- `repository_mutated: false`;
- `decision_ready: false`;
- `upstream_content_executed: false`.
- `transport.create_body_file: true`;
- `transport.edit_body_file: true`;
- a bounded nonzero `transport.body_bytes` and matching
  `transport.sha256` command/body seal.

Reobserve the target ref and pull request after the run. Their identities and
comments must be unchanged from the pre-run observations. Ordinary Actions run
artifacts are the only admitted remote writes.

Any model, acquisition, validation, proposal, freshness, ownership, transport,
or artifact failure disables the workflow and returns to a corrective issue.
Permanent activation requires the independently reviewed merged bytes, green
PR CI, this exact live proof, and unchanged remote proposal state.
