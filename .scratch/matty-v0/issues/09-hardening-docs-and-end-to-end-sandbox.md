Status: ready-for-agent

# Harden v0 with docs and end-to-end sandbox tests

## Parent

.scratch/matty-v0/PRD.md

## What to build

Add final v0 hardening: concise README usage docs, end-to-end sandbox tests for install/doctor/update/uninstall, and verification that Matty stays global-first, macOS-first, and safe around existing Gentle AI or user-managed config.

## Acceptance criteria

- [ ] README documents Matty's purpose, v0 scope, commands, global paths, and safety model.
- [ ] End-to-end sandbox tests cover install → doctor → update → uninstall.
- [ ] Tests verify no real HOME mutation is possible through the standard test suite.
- [ ] Tests cover coexistence with pre-existing Gentle AI markers/config.
- [ ] The final verification command for v0 is documented and passes.
- [ ] Out-of-scope items remain explicitly out of scope in docs.

## Blocked by

- .scratch/matty-v0/issues/07-implement-doctor-and-conflict-checks.md
- .scratch/matty-v0/issues/08-complete-update-and-uninstall-flows.md
