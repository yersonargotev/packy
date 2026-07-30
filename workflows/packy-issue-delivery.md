# Packy Issue Delivery

Status: Active

## Goal

Deliver one named Packy GitHub issue through a resumable, risk-adjusted
`Delivery Run`. Repeatedly invoke `Advance`; it observes current repository and
GitHub facts, performs the next deterministic work, persists evidence, and
stops only when it:

- needs a decision;
- needs review;
- is waiting for an external result;
- is blocked; or
- is completed.

This workflow is the behavioral contract. The model-invoked
`deliver-packy-issue` skill is only its thin adapter. The private orchestrator,
not its caller, owns phase sequencing, evidence schema v2, qualification
compilation, gates, invalidation, effects, recovery, timing, and cleanup.

## Trigger and authority

Trigger only when the user names one Packy issue by number or URL and explicitly
requests complete delivery. That request authorizes deterministic Git and GitHub
delivery effects, but never release or Pack Source publication, package-manager
effects, or real-user configuration.

Every new run qualifies exactly one `Delivery Authority`:

- a `Self-contained Issue`, when the approved issue completely states the
  objective, verifiable acceptance criteria, limits, dependencies,
  prerequisites, and prior decisions; or
- an `Issue with Specification`, when a separate normative source is shared,
  external, or independently identified and versioned.

Complexity or length alone does not require a separate specification. If the
authority permits materially different interpretations, `Advance` pauses for a
decision rather than inventing intent.

## Risk profile

Qualification selects `low-risk`, `standard`, or `high-risk` from observable
effects; `standard` is the default.

- `low-risk` is limited to passive artifacts or fail-closed reinforcement of
  existing repository validation, with no distributed-product or sensitive
  boundary effect.
- `standard` changes ordinary, reversible product behavior.
- `high-risk` crosses installation, real configuration, security, publication,
  migration, persistent-format, governance, destructive, or similarly
  hard-to-reverse boundaries.

Mechanical policy may raise but never lower the profile. Re-evaluate its floor
whenever the candidate changes. Escalation invalidates only evidence that cannot
satisfy the stronger profile; lowering the profile requires a newly qualified
run.

Every profile retains fresh authority, acceptance traceability, exact-HEAD
binding, sandboxed user paths, final exhaustive validation, required CI, merge
verification, and cleanup. The profile changes only local assurance: focused
checks, review cadence, evidence depth, specialist review, and
sensitive-boundary proof. High-risk delivery adds the specialist review required
by the crossed boundary and may add one exhaustive checkpoint immediately
before a hard-to-reverse effect.

## Advance

At most one run is active per repository and issue. State and locking are shared
across worktrees; different issues may advance concurrently. Requalification
supersedes the previous run with a new identity and preserves prior evidence.

The normal private CLI surface is:

```sh
go run ./internal/tools/deliveryevidence advance \
  --repository /absolute/path/to/packy \
  --issue N \
  [--spec S] \
  --risk-profile low-risk|standard|high-risk
```

Use `--spec S` only when qualification selects a distinct governing
specification; omitting it selects the self-contained authority form. This is a
semantic scope decision, not phase sequencing. Repeat the same command to
resume. Add `--decision`, `--repair`, or
`--review-content` only when the returned state requests that typed semantic
content. Before candidate development, review content may carry an independent
qualification review bound to the exact authority and acceptance-matrix digest.
When that review rejects the matrix, `Advance` persists its traceable findings
and requests one exact qualification correction in the same typed content
envelope. The correction must preserve every criterion identity and text,
replace the complete evidence matrix, and pass a second independent review.
Candidate acceptance proof in review content must cite the exact candidate ID
and the observed positive, negative/failure, mutation, compatibility,
preservation, and migration evidence required by each compiled row; a green
repository validator never manufactures those proofs. Use `--ci-attribution`
only to classify an exact failed run reported
by `Advance` as `infrastructure` or `candidate`; the check, run, HEAD, and URL
must all match the current observation. Add `--authorize-non-local` after exact local readiness when the
complete-delivery request authorizes the deterministic remote effects. The CLI
observes and binds repository, authority, candidate, branch, PR, CI, merge, and
cleanup identities itself; callers never provide those receipts or select an
internal phase.

On every invocation, `Advance`:

1. creates or resumes the run and reacquires observable Git, GitHub, filesystem,
   authority, candidate, validation, review, and CI facts;
2. adopts already-completed matching work;
3. blocks on incompatible identities or stale authority;
4. performs all currently safe deterministic work; and
5. persists the resulting state and automatic phase timing before returning.

The caller supplies only genuine judgment: ambiguous scope classification,
exceptions, profile decisions not settled mechanically, and review or
adjudication content. It does not assemble phase receipts or caller-authored JSON
for observable facts.

LOCAL may prepare a branch and coherent commits but cannot mutate GitHub.
NON-LOCAL begins only after fresh authority, candidate review, exact final
validation, and readiness gates succeed. Every external phase observes before
acting and uses deterministic issue, branch, pull-request, HEAD, and merge
identities. Resume adopts a matching effect, never repeats it, blocks on an
incompatible effect, and never performs automatic external rollback. Pushed
history is never rewritten. After a confirmed merge, only verification and
cleanup may continue.

## Qualification and development

Qualification compiles the authority into a canonical scope ledger and
acceptance-evidence matrix with stable row identities. It extracts explicit
criteria, exclusions, dependencies, and references and selects profile-shaped
evidence requirements. Every acceptance obligation must remain traceable to its
authority and have positive, negative or failure, and preservation or
compatibility evidence as applicable.

An independent qualification review may approve the exact compiled matrix or
reject it with findings tied to criterion identities and authority links. A
rejection moves the run to one persisted correction request. The corrected
matrix is stored in a new immutable run revision, retains the original
criterion identities and text, and returns to `needs-review`; development
cannot resume through that path until a later independent review approves the
corrected matrix. Resume preserves the original run bytes and adopts the active
revision without duplicating either effect.

For bugs, use `diagnosing-bugs` only while reproduction, cause, or failure
boundary is uncertain. Run Delegation Preflight before separable local work.
Use CodeGraph before code discovery for architecture, symbol, call-flow, or
impact questions.

Development advances the complete `Delivery Candidate`, independent of its
local commit count. Use affected tests, formatting, `git diff --check`, and
other focused checks selected by the acceptance matrix. Use sandboxed `HOME`,
`XDG_CONFIG_HOME`, Packy home, source, and state paths for any check that
resolves or writes user paths. Local commits remain coherent history units; they
do not trigger exhaustive validation or determine review count.

## Candidate review and repair

When the candidate is ready, run independent Standards and Spec reviews in
parallel over the complete accumulated candidate. The Spec axis receives the
immutable Delivery Authority, scope ledger, acceptance matrix, and prior
adjudications. High-risk candidates also receive their required specialist
review.

Adjudicate all findings and repair accepted findings as a batch:

- A `Bounded Repair` preserves behavior, contract, scope, architecture, security
  posture, and acceptance meaning. Run focused verification and obtain
  confirmation from the originating review axis.
- A `Candidate-changing Repair` changes one of those properties. It creates a
  new candidate, triggers risk-floor re-evaluation, and repeats the reviews
  required by the resulting profile.

After repairs, run one final Standards-and-Spec review of the complete resulting
candidate. Do not add review per commit or per bounded repair beyond originating
axis confirmation.

Before the first push, bounded repairs may be incorporated into coherent local
commits. Once pushed, never rewrite history.

## Exact final validation

After the final candidate is stable and its required reviews are satisfied,
create the intended final local commit and run the repository validation
authority exactly once:

`./scripts/validate-packy.sh`

Also retain the acceptance evidence and `git diff --check` result required by
the matrix. Bind the validation receipt to the exact commit and tree. Any later
repository change invalidates it and returns the run to candidate development;
do not reuse or patch the receipt. A high-risk pre-effect checkpoint, when
required by policy, is the only additional exhaustive validation.

## Delivery, CI, merge, and cleanup

Immediately before the first GitHub mutation, reacquire authority and readiness.
The issue and any separate specification must still carry
`status:approved`; an unapproved authority blocks without mutation. Push the
deterministic issue branch and create or adopt its PR to `main` with `Closes
#N`, the candidate summary, and exact validation evidence.

Wait for required CI on the exact validated HEAD. Retry an observed
infrastructure failure without changing the candidate. A change-attributable
failure returns to candidate development; after repair, review and validation
policy applies normally. If a bug failure restores diagnostic uncertainty,
invoke `diagnosing-bugs` before changing the candidate.

Merge only when every required check is green for the exact reviewed and
validated PR HEAD. Use a merge commit and delete the remote branch. Fetch with
pruning, verify `origin/main` contains the merge, verify the issue is closed,
and clean the integration worktree and local issue branch without disturbing
operator changes. Fast-forward local `main` only when Git can preserve the
operator checkout; otherwise report that it remains behind.

Completion requires the merged commit on `origin/main`, the closed issue, no
local or remote issue branch, clean integration state, verified cleanup, and a
success brief reporting authority, profile, candidate reviews, exact validation,
CI, merge, cleanup, timing, and preserved operator state.

## Recovery and pauses

Crashes and repeated invocations are normal. `Advance` resumes from persisted
v2 state, reacquires facts, and continues without duplicating matching local or
external effects. It pauses only for a genuine decision, required review,
external wait, or a blocking identity or invariant mismatch. Each pause returns
a decision-ready or status brief with linked evidence rather than raw logs.

For low-risk delivery, the operating objective is approximately 25 minutes from
qualification to PR readiness and 25–35 minutes end-to-end when CI completes
within 10 minutes. This is an observable performance objective, not a
correctness gate.

## Explicit legacy-v1 behavior

New runs always use evidence schema v2. Schema v1 is readable only under its
original workflow semantics. Never implicitly convert, resume, or enrich v1
evidence with v2 behavior.

Historical sequencing commands are absent from the normal CLI surface. They
are reachable only as:

```sh
go run ./internal/tools/deliveryevidence legacy-v1 <historical-subcommand> ...
```

When an existing v1 run is encountered, require an explicit choice:

- finish it under the legacy v1 workflow; or
- explicitly requalify it as a new v2 Delivery Run with a new identity.

No other part of this normal workflow describes or authorizes schema v1
behavior.
