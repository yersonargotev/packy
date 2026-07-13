Status: resolved
Blocked by: 01

# Route uninstall through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic uninstall end to end through lifecycle Preview and Apply so
ownership verification, marker-owned prompt removal, safe container cleanup,
and interrupted-install cleanup are owned by the lifecycle module.

## Acceptance criteria

- [x] Uninstall Preview is read-only and returns an opaque plan describing only verified Matty-owned removal and cleanup candidates.
- [x] Uninstall Apply verifies previewed cleanup preconditions before mutation and removes only managed skill links, marker-owned prompt/config content, classic state, and unchanged empty containers whose provenance is proven.
- [x] Missing state, corrupt state, interrupted install, changed containers, unmanaged symlinks, pre-existing containers, and contributor-owned bytes preserve the current safety behavior.
- [x] A converged uninstall reports no work without mutating the filesystem.
- [x] Apply returns a structured result or actionable domain error and never writes directly to command output streams.
- [x] The uninstall command retains its flags, dry-run rendering, relevant output, exit behavior, and preservation guarantees.
- [x] Facade and sandboxed end-to-end tests cover pristine cleanup, unmanaged preservation, recovery cleanup, preview/apply change detection, and repeated uninstall.
- [x] Focused tests and the full repository test suite pass.

## Out of scope

- Removing Installed Source data.
- Reading or deleting capability-pack state or projections.
- General workstation path redesign.

## Answer

Implemented classic uninstall through `corelifecycle.Facade.Preview(Uninstall)`
and exact-plan `Apply`. The lifecycle module now owns state and skill ownership
verification, marker-owned Codex/OpenCode cleanup, allowlisted container
provenance, stale artifact/container rejection, interrupted-install cleanup,
safe retargeted-link preservation, and structured work/no-work results.

The CLI now only resolves paths, invokes the facade, and renders the unchanged
dry-run and result messages. Uninstall policy coverage moved to sandboxed facade
tests while CLI coverage retains adapter contracts and lifecycle baselines.
Focused tests, the sandboxed full suite, `go vet ./...`, and both code-review
axes pass with zero findings.
