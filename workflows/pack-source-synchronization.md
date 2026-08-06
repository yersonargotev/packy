# Pack source synchronization

## Status and scope

This is the canonical operational contract for the private maintainer workflow
`.github/workflows/sync-pack-source.yml`. It exposes one manual-first operation;
it is not a public Packy command, a distributed binary, a scheduled refresh, or
authorization to merge. The implementation workflow may create or update only
the owned synchronization branch and pull request described below. It never
opens an issue, enables auto-merge, merges, or falls back from AI to human
classification.

The private `internal/tools/syncpacksource` adapter sequences domain behavior.
It must not reproduce candidate admission, compatibility floors, plan sealing,
exact version selection, evidence validation, Apply, or Recover. Those remain
owned by `internal/packsync` and `internal/packclassification` under ADR 0007
and ADR 0008.

The complete immutable schema suites are checked in under
`schemas/pack-source/v1.0.0/`, `schemas/pack-source/v2.0.0/`,
`schemas/pack-source/v2.1.0/`, and
`schemas/pack-source/v3.0.0/`. Each consists
of the dispatch, validation, no-op, operational-artifact, and publication
schema files. Version 1 remains the synchronization contract; version 2 adds
sealed source registration and source-scoped provenance; its v2.1 suite adds
explicit existing-source reconfiguration without changing instance
`schema_version: 2`. Version 3 is the
distinct Pack-scoped `register_bundle` contract for atomic initial admission of
two or more exact-commit sources and cannot consume or emit v1/v2 artifacts.
Repository validation
registers every schema locally by its canonical GitHub Pages ID. Packy runtime
validates domain values directly and never resolves schemas over the network.

## Canonical dispatch

Every request conforms to the dispatch schema for its declared version. A
version 1 request synchronizes one configured `source_id`. A version 2 request
also names an explicit `synchronize`, `register`, or `reconfigure` operation. Registration is
allowed only when the source is absent and seals the complete strict source
configuration plus its canonical SHA-256. Every v1/v2 request carries an
explicit candidate selector. Every request carries an explicit `ai` or `human`
classification mode and an operator reason. There are no automatic triggers.

Reconfiguration is allowed only for an existing source and seals one complete
replacement `SourceConfig` plus one complete canonical current-version Pack
manifest. It preserves source/provider/repository/selector/Pack identity,
requires exact bidirectional binding-to-manifest equality, and applies the
configuration, classified manifest, changed-selection evidence, immutable
history, source lock, and selected bytes as one bundle generation. It cannot
transfer ownership or infer omitted bindings.

The workflow transport additionally requires `request_digest`, the lowercase
SHA-256 of the sorted compact canonical request JSON including its trailing
newline. This derived value is not a request field or synchronization authority.
It is exposed with `source_id` in the run name so the repository-local
maintainer skill can identify an identical pending run without exposing the
reason or human evidence. Inspect recomputes and verifies it before admitting
the request; a started run's `request.json` remains the owner-produced proof.

The boolean `prepare_only` input is also transport-only. It is never part of a
canonical synchronization request or its digest. `false` admits only protected
`main` publication flow; `true` admits only a non-main approved issue branch,
only for v3 `register_bundle`, and never enters production concurrency or
publication authority.

A version 3 request names one absent `pack_id`, at least two complete source
registrations ordered by source ID, and `registration_bundle_sha256`. Each
member binds only that Pack, uses an exact full-commit selector, and carries a
durable digest-bound redistributable legal disposition. It has no member
selector or independently reusable source subrequest. The request also carries
the canonical initial `proposed_manifest`, its exact digest, and its proposed
version; no Pack manifest is pre-created outside the atomic result. Every later
v3 artifact
preserves the ordered source/member identity, legal evidence, plan, base,
resulting configuration/manifests, source-lock set, result tree, and
secret/upstream-byte attestations.

`latest-stable` has no selector reference. `prerelease` carries one exact
published prerelease tag. `commit` carries one full lowercase commit SHA. An
AI request cannot carry human evidence and never changes modes automatically.

Human classification is inspection-first and evidence-second:

1. The first dispatch explicitly selects `human` without evidence. Inspect
   emits the canonical sealed plan and bound inspection identity, then stops.
2. The operator inspects that artifact. A second `human` dispatch selects the
   exact candidate commit and supplies canonical evidence bound to the exact
   inspection plan ID and base SHA. Missing or stale bindings block.

For v3, every member is already exact-commit in both dispatches, so the second
human dispatch repeats the complete identical ordered registration and proposed
generation seals instead of carrying a single selector. Its
`human_evidence` is one complete Pack-level composite classification bound to
the exact plan, Pack, and base; it grants no member-level authority.

## Concurrency and freshness

The concurrency group is `sync-pack-source-<source-id>` for v1/v2 and
`sync-pack-source-<pack-id>` for v3, with
`cancel-in-progress: false`. GitHub therefore leaves the one active run alone,
admits at most one pending run for that source, and a newer request replaces
only the older pending request. No run resumes another run's plan. Every run
that actually starts begins at Inspect and executes a fresh canonical Check.

The concurrency key is serialization, not freshness proof. Inspect seals the
candidate, base, plan ID, provenance, configuration and selection observation.
Publish must reobserve them immediately before its first write.
Preparation-only runs use a separate branch-scoped concurrency group and can
neither block nor supersede a production operation for the same Pack.

## Phases and permission boundary

The workflow starts with `permissions: {}`. Every external action is pinned by
one full commit SHA. Admit rejects transport above the ADR 0019 bounds before
checkout, acquisition, or model use. Every job has a timeout, Go dependency
caching is enabled, and the normal critical-path timeout budget is ten minutes.
Ordinary pull-request CI, not this operational workflow, runs the exhaustive
`validate-packy.sh` repository suite.

### Inspect — `contents: read`

Inspect checks out without persisted credentials and invokes:

```text
go run ./internal/tools/syncpacksource --phase inspect ...
```

The private adapter supplies the job-scoped `GITHUB_TOKEN` to an authenticated
read-only client that adds authorization only for the exact GitHub API origin;
redirected archive origins and deterministic domain modules never receive it.

The adapter validates the dispatch, creates isolated acquisition directories,
and calls canonical `packsync.Check` or complete-set
`packsync.CheckComposite`. Its output is a sealed, immutable
inspection artifact. It contains identities, reasons, changes, blockers and
digests, not copied upstream resources or credentials.

For a composite request, Check validates the complete prospective result with
the narrow Pack-content authority. An exact single-source Check-level no-op
emits the matching v1/v2
`pack-source-noop.schema.json` contract from Inspect and stops before
classification or publication permissions. Initial v3 composite
registration has no no-op or partial-convergence state.

### Classify — `contents: read`, `models: read`

Classify downloads that exact inspection artifact and invokes:

```text
go run ./internal/tools/syncpacksource --phase classify ...
```

It passes the sealed plan to `packclassification`. AI mode retries only model
transport failures according to the retry policy below. Human mode accepts
only the separately dispatched, inspection-bound evidence. V3 emits one
Pack-level composite classification plus an artifact that seals the exact
evidence digest and complete plan identity; neither member evidence nor a
member subplan carries authority. The classifier has no publication authority
and never writes a branch or pull request.

### Publish — `contents: write`, `pull-requests: write`

Publish is gated by Inspect and Classify. It downloads only their sealed artifacts
and invokes:

```text
go run ./internal/tools/syncpacksource --phase publish ...
```

Before the first Git or GitHub write, the adapter uses an isolated checkout to
reacquire the exact candidate or complete ordered member set, calls canonical
Apply or ApplyComposite (and Recover if canonical
transaction evidence requires it), renders the diff, runs the complete
Pack-content validation authority on the staged result, evaluates ownership,
and freshly reobserves
the repository and GitHub state. Only a proposal whose exact identity passes
all gates can reach the write operation. A first PR is created as a blocked
draft, reobserved, converted to ready, finalized with decision-ready metadata,
and reobserved again before readiness is recorded. The operational path never
runs the exhaustive repository suite; its final Apply owns the staged-content
validation and exact sealed result-tree comparison.

A normal operation performs no more than two independent Pack-content
validations. Composite Inspect validates its complete prospective result and
the final Apply validates the staged result. A single-source operation needs
only the final staged validation. A matching sealed base hash makes another
validation of the unchanged current bundle redundant.

All phases set sandboxed `HOME` and `XDG_CONFIG_HOME`. Acquisitions, staged
checkouts, generated state and filesystem writes remain under runner-owned
temporary or checkout paths.

### Premerge preparation — `contents: read`, `pull-requests: read`

A preparation-only branch dispatch uses the issue branch's reviewed workflow
and adapter bytes but performs every repository operation in a disposable
checkout of the current protected `main`. It runs the exact v3 request through
Inspect, real GitHub Models Classify, and a distinct Prepare job. Prepare
independently reacquires every exact member, Applies, runs the narrow
Pack-content validation on the staged result, seals and verifies the result tree,
constructs the local proposal commit and managed metadata, revalidates
provenance, and requires two identical read-only observations of live
publication state.

Prepare stops before the `PublicationGateway.Publish` and `Finalize` mutation
methods. Its artifact records stable observation, proposal/tree identity,
complete validation gates, `repository_mutated: false`, and
`decision_ready: false`; it is proof that the pre-publication path executed, not
a synchronization result or review authority. The branch job cannot push,
create, edit, ready, or comment on a pull request, or share production
concurrency. The write-capable Publish job remains protected-main-only.

Prepare also invokes the exact final create/edit argument builders without
running them. It requires private `--body-file` transport for both operations
and records the bounded body length and SHA-256. This proves the final GitHub
adapter without using `gh pr create --dry-run`, which may still push.

## Retries and failures

A single operation has at most three attempts. Attempt delays use bounded
exponential backoff. A valid server `Retry-After` delay or future
`X-RateLimit-Reset` deadline replaces the computed delay when it is longer.
GitHub HTTP 403 is a rate-limit failure only when safe response metadata proves
it (`Retry-After` is valid or `X-RateLimit-Remaining` is zero); otherwise it is
a terminal access failure with access-specific recovery. Only transport,
rate-limit and service-unavailable failures classified as transient retry.

A failed lease-protected branch push is reconciled once by reading the remote
ref. If the target head is not already present, the run stops; it never repeats
the write without a fresh Check and full publication revalidation.

Provenance, integrity, classification, validation, ownership and divergence
failures are terminal. Invalid evidence and AI unavailability after the bound
attempts remain blocked. Retrying never changes candidate or classification
mode and never implies AI-to-human fallback.

Before a PR exists, every terminal failure emits
`schemas/pack-source/v1.0.0/pack-source-operational-artifact.schema.json` as
canonical JSON. It contains the exact known source/plan/base/candidate
identities, blockers, and concrete recovery steps. It contains no credential,
environment dump, request header, model prompt, upstream file content, patch,
archive, or other upstream bytes. The workflow retains this artifact for 30
days and does not publish an issue.

## Publication ownership and fail-closed checks

The operation owns exactly `sync/<source-id>` for v1/v2 or `sync/<pack-id>` for
v3 and at most one open PR from that
branch to `main`. The automation identity is `github-actions[bot]`. A pristine
first publication may create both. A pristine advancing candidate may update
the same branch and PR. An exact already-published identity is a no-op.

Immediately before writing, Publish revalidates all of the following together:

- candidate commit and non-regressive relation;
- current base SHA and sealed plan ID;
- exact source provenance and proposed provenance digest;
- automation identity and the ownership record;
- stable branch name, head, ancestry and commit authorship;
- sole open PR identity, base/head/state, managed metadata, content and
  authenticated last-editor identity (a present edit with an unavailable actor
  is ambiguous); and
- the exact validated result tree and every decision-readiness gate.

Publish fails closed, without a force-push, metadata overwrite or competing
PR, when metadata was edited, a human commit is present, the branch diverged,
identity is unexpected, base or plan is stale, the candidate regresses,
provenance moved, the PR was closed, or automation ownership is absent or
ambiguous. A closed owned PR is an explicit blocker; automation does not create
a replacement. Reviewer-authored content is never normalized away.

## Decision readiness

The publication record conforms to
`schemas/pack-source/v1.0.0/pack-source-publication.schema.json`. A PR may be
non-draft and marked decision-ready only when these gates passed for one exact
plan/base/head/candidate/provenance/PR-state identity:

The record binds `result_tree_sha` as the validated content identity and
`head_sha` as the distinct branch and pull-request commit identity.

1. provenance;
2. classification;
3. exact candidate reacquisition;
4. canonical Apply;
5. expected diff;
6. automation ownership; and
7. the Pack-content validation authority.

Auto-merge is false and manual merge remains required. A later change to base,
candidate, provenance, head, managed PR state, or the PR's open identity makes
the readiness record invalid. The next operation must start again with fresh
Inspect; readiness is not patched forward.

## Canonical proposal brief

Publish renders one canonical JSON proposal into Markdown without recomputing
domain facts. Full JSON and Markdown remain run artifacts. The managed PR body
contains only the bounded identity summary and a link to that exact workflow
run, avoiding GitHub's body-size ceiling. The canonical artifacts carry:

1. request actor and reason, source, selector, workflow run and attempt,
   candidate, plan ID, base, commit head, validated result tree and branch/PR
   identity;
2. repository and owner identity, release, exact tag-to-commit resolution,
   verification, tree and parent identity, and provenance hashes;
3. selected resource and file additions, modifications, removals and moves,
   unselected discoveries, and old/new snapshot hashes;
4. affected packs, old and proposed versions, mechanical floors, final
   classifications, classifier identities and rationales, plus mandatory
   migrations and actions;
5. exact reacquisition, Apply, diff and every validation result, including the
   explicit fact that no upstream content was executed; and
6. blockers, decision-readiness state, invalidation conditions, and exact
   retry or recovery instructions.

The managed title/body markers and their canonical hash form part of ownership
revalidation. The title is bounded to 200 bytes, the body to 60 KiB, and the
constructed command to 16 KiB. Logs are not the operational record, and the
Markdown renderer cannot add authority absent from the sealed JSON inputs.

Successful publication uploads `publication.json`, `proposal-brief.json`, and
the rendered brief as one 30-day run artifact. The maintainer skill validates
that artifact and the live PR before reporting decision readiness; neither the
workflow conclusion nor a PR alone establishes success.

## Sandboxed acceptance tracer

Deterministic fake source, GitHub, clock, sleeper, and concurrency fixtures
exercise: pristine creation; pristine update; exact no-op; base advancement;
candidate regression; divergent branch; edited metadata; human commits;
ambiguous ownership; closed PR; unexpected identity; human inspection then
evidence; unavailable AI; three-attempt exponential backoff; `Retry-After`;
non-retryable blockers; secret-free and upstream-byte-free failure artifacts;
active-run preservation and pending supersession; fresh Check when the promoted
pending run starts; valid readiness and later invalidation; bounded input/body
transport; effect-free create/edit command construction; and the permission
boundary between read-only preparation and publication.

The tracer uses no GitHub Models request, workflow dispatch, real source branch,
real synchronization PR, real merge, real refresh, or real bundle update. It
also preserves the seals, provenance bindings, shared lock and transaction
contracts delivered before this workflow.
