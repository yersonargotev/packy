# ADR 0015: Detect governance drift without repair authority

## Status

Accepted.

## Context

Packy's protected-main, release, environment, credential, workflow, and
publication controls are effective external state. Repository tests prove the
adapters and policies checked into Git, but cannot prove that the corresponding
GitHub state remains current. A scheduled comparison is needed without giving
the detector authority to repair the controls it observes.

Some control surfaces, especially installed GitHub App authority and account or
release recovery authority, are not completely observable with the
repository-scoped workflow token. Treating an unavailable projection as an
empty safe value would hide drift. Treating weekly polling as authorization for
private-security exceptions would also preserve the stale-success window
rejected by ADR 0013.

## Decision

Packy owns one versioned governance expected-state contract for stable security
and repository controls and one pure domain evaluator. Mutable product
publication state, including the latest release tag and publication timestamp,
is not a fixed governance expectation. Exact release identity and correctness
remain owned by the release workflow and ADR 0014. Every governance observation
is sanitized and bound to the repository,
protected ref, commit, workflow definition, and UTC collection time. Evaluation
distinguishes clean state, confirmed drift, unclassifiable drift, and collection
failure. Missing, malformed, stale, or wrong-identity evidence fails closed.

Each contract control names only the promotion or publication boundaries it can
invalidate. A current clean observation is necessary but not sufficient after
drift: a human Owner must classify the exact canonical drift evidence before a
later clean rerun may resolve the issue or unblock its affected boundary.

The scheduled observer carries read-only repository permissions. A separate
reporter may create, update, deduplicate, and resolve only the canonical drift
issue. Neither component receives authority to change settings, refs,
environments, credentials, Apps, releases, packages, Pages, workflows, or
protection rules.

Installed-App inventory, recovery readiness, and residual Owner release
authority use a versioned sanitized Owner attestation when GitHub exposes no
complete repository-token projection. The collector verifies that record and
its review deadline; expiration or absence is unclassifiable drift. Policy
requires an immediate replacement after a relevant control, incident, App,
credential, or required-check identity/source change. The attestation never
contains credential values, recovery material, or raw installation responses.

The Release and pack-source workflows perform a fresh read-only evaluation
before the first affected action. Release drift stops before build, OIDC,
drafting, upload, publication, or Homebrew mutation, preserving ADR 0014's one
candidate and no-repair rules. After a successful release, final verification
reruns the publication boundary against freshly fetched protected `main`; a
normal publication remains clean, while unexpected remaining drift follows the
canonical governance-drift path. Private-security remains deterministically
denied; drift polling never authorizes it.

## Consequences

- Confirmed, unclassifiable, failed, or stale evidence blocks only its declared
  boundary.
- A clean rerun cannot silently erase an unclassified signal.
- Human-only surfaces remain explicit and time-bounded rather than being
  represented as automatically observable.
- Expected-state changes use the protected PR path and receive CODEOWNER review.
- Normal advancement of an exact, verified release does not require an
  expected-state update; tag, timestamp, author, immutability, notes, artifact,
  provenance, and Homebrew agreement remain release-instance verification.
- A detector outage may block promotion or publication until current evidence
  can be collected; this is intentional fail-closed behavior.

## Enforcement

Pure fixtures cover all evaluation states, stale evidence, affected-boundary
gating, exact-evidence classification, canonical issue lifecycle, and absence
of self-correction. Fake-GitHub tests prove projected GET-only collection and
sanitization without collecting mutable latest-release identity. Workflow
trust-boundary tests keep observation authority separate from issue reporting,
require fresh gates before Release and synchronization promotion, and require
Release final verification to rerun the publication boundary.
