# Packy v0.2 product scope

Packy is an installer/configurator for four reviewed Packs on Codex, OpenCode,
and Claude Code **2.1.203 or newer**. It is not an always-on runtime
orchestrator.

## Product boundary

| Area | Current scope |
| --- | --- |
| Packs | Argote at Pack version `1.0.1`; Addy, Engram, and Matty at Pack version `1.0.0`. |
| Authoring | One reviewed manifest and reviewed content per Pack. |
| Surfaces | Codex, OpenCode, and user-global Claude Code. |
| Global lifecycle | Inspect, activate, update, status, deactivate, and remove one Pack at a time. |
| Project lifecycle | Declare and install Packs in a Git worktree; activate personal runtime effects separately. |
| State | Minimal installed Pack receipts for ownership, drift, collisions, update, and removal. |
| External tools | Detect requirements such as Engram without making one Pack depend on another. |
| Distribution | Immutable GitHub Release archives, checksums, and Homebrew formula. |

## User outcomes

- `packy init`, catalog inspection, `doctor`, and `--dry-run` do not change a
  CLI surface.
- Every mutation names one Pack and uses a fresh preview.
- Multiple Packs coexist through independent receipts.
- Drift and collisions fail before mutation.
- Project intent remains reviewable while personal runtime consent stays local.
- Unchanged receipt-owned projections can be updated or removed without
  granting authority over unrelated files.
- The focused Pack validator gives maintainers a short authoring feedback loop.

## Clean-generation boundary

Packy `v0.2.0` does not read prior workstation state or project declarations.
The sole current user follows the [manual one-time reset](../reset-v0.2.md)
before adopting this generation. Existing Git tags and releases remain
immutable.

## Verification

Use focused tests for touched behavior, `./scripts/validate-pack-content.sh`
for Pack content, and `./scripts/validate-packy.sh` for the sandboxed general
check. Release packaging checks run only at the release boundary described in
the [release guide](../release.md).
