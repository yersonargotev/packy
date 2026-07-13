Status: resolved
Blocked by: 01

# Route install through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic install end to end through lifecycle Preview and Apply so dry-run,
managed-skill reconciliation, prompt projection, Engram setup, ownership, and
interrupted recovery no longer depend on CLI-owned policy.

## Acceptance criteria

- [x] Install Preview returns an opaque, caller-immutable plan with a read-only action view and performs no filesystem mutation, state publication, directory creation, or external command.
- [x] Install Apply consumes the exact previewed plan and owns managed-skill discovery, unmanaged-path preservation, container provenance, prompt projection ordering, Engram installation/setup decisions, recovery-state publication, and final confirmed state.
- [x] The lifecycle module uses injected command lookup/execution and time while keeping filesystem behavior internal and sandboxed in tests.
- [x] Apply returns structured results, warnings, and actionable domain errors without writing to command output streams.
- [x] The install command retains its flags, dry-run rendering, relevant output, warnings, exit behavior, idempotency, and package/repository/override source reporting.
- [x] Failure tests cover pre-mutation rejection, state preparation, partial container creation, symlink ownership persistence, external command failures, final state publication, and safe retry.
- [x] Lifecycle policy is tested primarily through the facade, with command tests limited to adapter behavior and a sandboxed end-to-end install baseline.
- [x] Focused tests and the full repository test suite pass.

## Out of scope

- Routing update or uninstall through the facade.
- Adding approvals, plan digests, serialized plans, or capability-pack stale-plan semantics.

## Answer

Implemented classic install through `corelifecycle.Facade.Preview(Install)` and
`Apply(plan)`. The facade now owns the immutable ordered action plan, skill and
Engram decisions, unmanaged preservation, container provenance, prompt
projection, recovery publication, final confirmation, structured warnings and
actionable errors. The CLI constructs the facade from resolved paths, renders
the detached action view and results, and no longer owns install sequencing or
the former `persistClassicState` seam.

Install policy and the complete failure/retry matrix now run through the facade
with sandboxed filesystem paths, fake command lookup/execution, and an injected
clock. Focused tests and sandboxed `go test ./...` pass. A two-axis review
against the ticket baseline finished with no remaining Standards or Spec
findings; update and uninstall remain on their existing CLI-owned flows.
