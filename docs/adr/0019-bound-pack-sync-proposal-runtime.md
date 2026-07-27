# ADR 0019: Bound Pack Sync proposal runtime

## Status

Accepted.

## Context

The manual Pack Sync workflow accumulated five exhaustive repository
validations across Check, Validate, Apply, and publication. A composite
registration consequently ran for roughly 50 minutes and still failed after
all expensive work because GitHub rejected a pull-request body above its
65,536-byte API limit. Passing that body through a file protected argv but did
not change the API limit.

The safety invariants remain necessary: exact candidate reacquisition,
domain-owned validation of inert Pack content, sealed result-tree equality,
provenance, ownership, least privilege, and manual merge. Repeating the entire
repository suite inside an operational workflow is not one of those invariants.
Ordinary pull-request CI already owns exhaustive repository validation.

## Decision

The operational graph is **Admit → Inspect → Classify → Publish**, with
**Prepare** replacing Publish for read-only proof. There is no normal Validate
job or validation artifact prerequisite.

Operational validation uses the narrow Pack-content authority:

- `capabilitypack.ValidatePortableContent` validates every portable manifest,
  dependency, contract, and referenced inert resource;
- `packsync.ValidateContent` additionally validates the strict
  `sources.json`/canonical-lock bijection; and
- `scripts/validate-pack-content.sh` is the sandboxed runner entrypoint.

An operation performs no more than two independent Pack-content validations.
Composite Inspect validates the complete prospective result and final
Apply/ApplyComposite validates its staged result. Single-source publication
needs only the final staged validation. The unchanged current bundle is not
revalidated after its sealed precondition hash has matched. Final composite
Apply must still compare the staged bundle hash exactly with
`ResultBundleSHA256`; the Git adapter must still seal and verify the workspace
and commit tree.

Admission rejects oversized or malformed transport before checkout,
acquisition, or model use:

- all workflow inputs together: 60 KiB;
- one registration or ordered registration bundle: 16 KiB;
- proposed manifest: 48 KiB;
- human evidence: 16 KiB;
- operator reason: 2 KiB; and
- composite members: 2 through 8.

Managed pull-request title, body, and constructed GitHub command are bounded at
200 bytes, 60 KiB, and 16 KiB respectively. The managed body is a concise
identity and workflow-run link. Full canonical JSON and Markdown evidence
remain 30-day run artifacts. Prepare exercises the exact create/edit command
builders with private temporary body files and records their digests without
calling GitHub mutation commands. `gh pr create --dry-run` is not used because
it may still push.

Every job has a timeout and every Go setup uses dependency caching. The normal
critical-path timeout budget is ten minutes: Admit 1, governance 1, Inspect 2,
Classify 2, and Publish 4. A platform outage or measured rate-limit delay is an
exception to report with Actions timestamps, not a reason to restore redundant
validation.

The old workflow remains disabled through merge. After merge, maintainers may
enable the replacement only to run the exact preparation-only proof from the
approved issue branch. No production dispatch is allowed during proof. Failure
immediately returns the workflow to disabled; permanent activation requires an
accepted proof showing unchanged remote branch/PR state and the sealed
effect-free transport fields.

## Consequences

- Pull-request CI remains the single exhaustive pre-merge validation authority.
- Operational runtime is bounded without weakening exact content, provenance,
  result-tree, ownership, or manual-merge guarantees.
- Large canonical evidence can no longer fail late at GitHub's PR-body limit.
- Immutable v1/v2/v3 schemas remain available for historical artifacts; their
  validation records are not a required job-to-job transport in the new graph.

## Supersession

This ADR supersedes only the expensive proof shape in ADR 0009, ADR 0016, and
ADR 0017: the standalone Validate job, repeated exhaustive repository suite,
and pre-merge live dispatch requirement. Their domain ownership, exact
candidate, atomicity, provenance, least-privilege, no-upstream-execution, and
manual-merge decisions remain accepted.

## Enforcement

Structural tests reject a Validate job, `validate-packy.sh`, uncached Go setup,
unbounded inputs, missing timeouts, or validation-artifact dependency in the
operational workflow. Lifecycle tests count Pack-content validations, prove
exact staged-result equality, retain full evidence artifacts, keep managed PR
bodies bounded, and exercise create/edit body-file transport without effects.
