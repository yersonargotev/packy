# ADR 0020: Adopt risk-adjusted issue delivery orchestration

## Status

Superseded by [ADR 0021](0021-extract-issue-delivery.md).

## Context

Packy's issue-delivery workflow applies nearly the same local evidence,
review, and validation cadence to every approved issue. A low-risk repository
change can therefore require a duplicate specification issue, exhaustive
validation before every commit, paired reviews for every repair commit, and
caller-authored JSON for facts already observable from Git and GitHub.

These controls preserve useful invariants, but their repetition amplifies small
findings and makes the workflow's cost disproportionate to the change's
observable effects. The evidence adapter is also shallow: callers must understand
and sequence many recording operations instead of using one delivery interface.

ADR 0001 keeps the distributed Packy product an installer/configurator rather
than a runtime orchestrator. Issue delivery is Packy-owned repository
maintenance, so its orchestration may be deepened without adding a public
`packy deliver` command or installing runtime behavior for users.

## Decision

### Delivery Authority

Every new delivery has one `Delivery Authority` with exactly one of two forms:

- a `Self-contained Issue` whose approved contents completely state the
  objective, verifiable acceptance criteria, necessary limits, dependencies,
  prerequisites, and prior decisions; or
- an `Issue with Specification` when a separate normative source is shared,
  external, or independently identified and versioned.

Issue and specification identity are no longer required to be distinct because
a self-contained issue has no second specification identity. Complexity or
length alone does not require a separate specification.

### Delivery Risk Profile

Qualification explicitly selects `low-risk`, `standard`, or `high-risk` from
observable effects. `standard` is the default. Mechanical policy may raise but
never lower the profile, and the profile can only escalate within one
`Delivery Run`.

`Low-risk Delivery` has no distributed-product or sensitive-boundary effect and
is limited to passive artifacts or fail-closed reinforcement of existing
repository validation. `Standard Delivery` changes ordinary, reversible product
behavior. `High-risk Delivery` crosses installation, real configuration,
security, publication, migration, persistent-format, governance, destructive,
or similarly hard-to-reverse boundaries.

All profiles retain fresh authority, acceptance traceability, exact-HEAD
binding, sandboxed user paths, final exhaustive validation, required CI, merge
verification, and cleanup. Profiles vary only local assurance: focused checks,
review cadence, evidence depth, specialist review, and sensitive-boundary proof.

The implementation re-evaluates the profile floor whenever the candidate
changes. Crossing an escalator raises the profile and invalidates only evidence
that cannot satisfy the stronger policy. Lowering a profile requires a new
qualified run.

### Candidate-based review and validation

The `Delivery Candidate`—the complete proposed repository change—is the unit of
review and validation. Local commits remain coherent units of source history but
do not determine review count.

Standards and Spec review the accumulated candidate in parallel. Accepted
findings are adjudicated and repaired as a batch, followed by a final
Standards-and-Spec review of the complete resulting candidate. High-risk work
adds the specialist review required by its sensitive boundary.

A `Bounded Repair` preserves behavior, contract, scope, architecture, security
posture, and acceptance meaning. It receives focused verification and
confirmation from the originating review axis. A `Candidate-changing Repair`
creates a new candidate and repeats the reviews required by its profile.

Development uses affected tests, formatting, `git diff --check`, and other
focused checks selected from the acceptance matrix. The repository validation
authority runs once after the final candidate is stable and binds its receipt to
the exact commit and tree. Any later change invalidates that receipt. High-risk
policy may require an additional exhaustive checkpoint only before a
hard-to-reverse effect.

Before the first push, bounded repairs may be incorporated into coherent local
commits. Pushed history is never rewritten by the delivery workflow.

### Deep private orchestration

Packy owns a private deep module for issue delivery. Its primary interface is
one `Advance` operation that creates or resumes a `Delivery Run`, reacquires
observable facts, performs deterministic work, and stops in one of these
observable states:

- needs decision;
- needs review;
- waiting for an external result;
- blocked; or
- completed.

The module hides phase sequencing, v2 persistence, qualification compilation,
evidence admission, invalidation, gates, Git/GitHub effects, recovery, and
cleanup. The private delivery-evidence CLI becomes a thin adapter. The public
Packy binary does not expose the orchestrator.

Git, GitHub, filesystem, clock, validation, and review execution vary behind
internal seams with production and test adapters. Callers provide only genuine
judgment: ambiguous scope classification, exceptions, profile decisions not
settled mechanically, and review or adjudication content.

The orchestrator extracts explicit criteria, exclusions, dependencies, and
references; generates stable row identities and profile-shaped evidence
requirements; and compiles the canonical ledger and matrix. It pauses rather
than inventing intent when authority permits materially different
interpretations.

### Run identity, effects, and recovery

At most one `Delivery Run` is active per repository and issue. State and locking
are shared across worktrees, while different issues may advance concurrently.
Requalification supersedes the previous run and creates a new identity without
rewriting prior evidence.

An explicit complete-delivery request authorizes deterministic Git and GitHub
delivery effects. LOCAL may prepare the branch and commits but cannot mutate
GitHub. NON-LOCAL begins only after fresh authority, candidate review, exact
validation, and readiness gates succeed. Release publication, Pack Source
publication, and real-user configuration remain outside this authority.

Every external phase observes before acting and uses deterministic issue,
branch, pull-request, HEAD, and merge identities. Resume adopts an already
completed matching effect, blocks on incompatible identity, and never performs
automatic external rollback. After a confirmed merge, only verification and
cleanup may continue.

### Evidence schema and timing

New runs use evidence schema v2, which represents the authority union, risk
profile, candidate reviews, repair classification, run state, and automatic
timing. Schema v1 remains readable and valid under its original semantics.
Existing v1 evidence is never converted implicitly; a v1 run either finishes
under the legacy workflow or is explicitly requalified as a new v2 run.

Every v2 transition records timestamps automatically and separates active
agent time, tool execution, validation, review, remote CI wait, and cleanup.
Callers do not assemble phase receipts manually.

For low-risk delivery, the operating objective is at most approximately 25
minutes from qualification to PR readiness and 25–35 minutes end-to-end when CI
finishes within 10 minutes. This is an observable performance objective, not a
correctness gate.

## Consequences

- Low-risk work retains final assurance while removing duplicate authority,
  per-commit review amplification, and repeated exhaustive validation.
- Ordinary and sensitive work use the same resumable interface but receive
  stronger evidence and specialist checks when their effects require them.
- Delivery state becomes recoverable and tool-observed instead of caller-built.
- The repository gains a private orchestrator without changing the distributed
  Packy product shape accepted by ADR 0001.
- Schema v1 compatibility remains explicit, but active v1 runs do not gain v2
  behavior without requalification.
- The orchestration module owns more implementation complexity so its caller and
  test interface can remain small.

## Out of scope

- path-selective CI or changes to universal merge gates;
- release, Pack Source, package, or Pages publication;
- a public or installed `packy deliver` command;
- a standalone semantic vocabulary or CLI-seam preflight;
- generalizing issue delivery to repositories other than Packy.

## Enforcement

Tests exercise behavior through the `Advance` interface with production-shaped
test adapters. They cover self-contained low-risk delivery, proportional repair,
candidate invalidation, automatic risk escalation, stale authority and HEAD
blocking, crash-safe resume, per-issue locking, exact validation reuse,
high-risk specialist evidence, v1 compatibility, verified cleanup, and
automatic phase timing.
