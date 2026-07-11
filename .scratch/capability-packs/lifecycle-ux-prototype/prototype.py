#!/usr/bin/env python3
"""THROWAWAY: print candidate pack-lifecycle CLI transcripts; no side effects."""

import sys


def discovery() -> None:
    print("""DECISION 1 — discovery and command grammar

Alternative A: noun hierarchy
$ matty pack list
PACK    VERSION  DESCRIPTION                         AVAILABLE ON
matty   built-in Matty workflow                     codex, opencode
engram  1.19.0   Persistent memory for agent work   codex, opencode

$ matty pack show engram
engram 1.19.0
Provides: persistent-memory
Requires: global tool engram >=1.19.0
Resources: 1 skill, 1 instruction, 1 mcp_server, 1 lifecycle
Activation: inactive on codex; inactive on opencode
Next: matty pack activate engram --surface codex --dry-run

Alternative B: top-level lifecycle verbs
$ matty packs
PACK    VERSION  CODEX     OPENCODE  READINESS
matty   built-in inactive  inactive  —
engram  1.19.0   inactive  inactive  —

$ matty pack engram
engram 1.19.0
Provides: persistent-memory
Requires: global tool engram >=1.19.0
Resources: 1 skill, 1 instruction, 1 mcp_server, 1 lifecycle
Next: matty activate engram --surface codex --dry-run

Shared invariant:
  --surface is explicit; discovery never activates or mutates anything.
  `matty` and `engram` are the only proof packs; web/mobile remain absent.
""")


def activation() -> None:
    print("""DECISION 2 — preview-to-apply interaction

Alternative A: one interactive lifecycle command
$ matty pack activate engram --surface codex
Plan P-7c2a  activate engram 1.19.0 on codex
Intent revision: 4 -> 5

PHASE 1  reversible local projections
  + skill persistent-memory
  + instruction engram-memory
  + MCP server engram
Approval required: activation [plan P-7c2a, phase local-a91f]
Approve reversible local changes? [y/N] y

PHASE 2  executable / external effects
  ! run: engram setup codex
Approval required: executable-action [plan P-7c2a, phase exec-01dd]
Run this external command? [y/N] y

Applying exact plan P-7c2a ...

# Existing convention: the same command previews and stops.
$ matty pack activate engram --surface codex --dry-run
Plan P-7c2a ...
Dry-run: no approvals requested; no state, files, or commands changed.

Alternative B: explicit preview artifact followed by apply
$ matty pack preview activate engram --surface codex --out ./engram-plan.json
Plan P-7c2a ...
Saved immutable plan envelope: ./engram-plan.json
No state, files, or commands changed.

$ matty pack apply ./engram-plan.json
Validated plan P-7c2a against intent revision 4 and fresh host inspection.
Approve reversible local changes? [y/N] y
Run external command `engram setup codex`? [y/N] y
Applying exact plan P-7c2a ...

Shared invariant:
  Approval receipts bind the exact plan and typed phase digests.
  A stale intent revision or host fingerprint executes zero actions.
""")


def unattended() -> None:
    print("""DECISION 3 — unattended and scripted use

Alternative A: applying is interactive-only
$ matty pack update engram --surface codex </dev/null
Plan P-901e  update engram 1.19.0 -> 1.20.0 on codex
PHASE 1 reversible local projections  [activation approval required]
PHASE 2 executable / external effects [executable-action approval required]
error: approval requires an interactive terminal
hint: inspect safely with `--dry-run`; rerun interactively to approve
Result: zero actions; intent revision unchanged.

Alternative B: accept pre-issued, plan-bound receipts
$ matty pack update engram --surface codex \\
    --approval activation:P-901e:local-a91f:<receipt> \\
    --approval executable-action:P-901e:exec-01dd:<receipt>
Validated both typed receipts against exact plan P-901e.
Applying exact plan P-901e ...

$ matty pack update engram --surface codex --yes
error: unknown flag --yes
hint: required human checkpoints cannot be bypassed

Shared invariant:
  `--dry-run` and `matty pack status` remain safe in non-interactive use.
  Missing, mismatched, or stale receipts execute zero actions.
""")


def status() -> None:
    print("""DECISION 4 — status entry point and density

Alternative A: overview first, then drill-down
$ matty pack status
PACK    SURFACE   INTENT      ATTEMPT             CONFIGURED  AUTHORIZED  USABLE  ACTION
matty   codex     active      verified            yes         yes         yes     —
matty   opencode  inactive    —                   —           —           —       —
engram  codex     active      verified            yes         no          no      login
engram  opencode  active      recovery-required   no          no          no      recover

$ matty pack status engram --surface codex
engram 1.19.0 on codex
Intent: active at revision 5
Latest attempt: verified (P-7c2a)
Readiness: configured=yes, authorized=no, usable=no
Pending human actions:
  - Authenticate Engram in Codex, then rerun `matty pack status engram --surface codex`.
Owned projections: 3 verified; 0 drifted; 0 ambiguous

Alternative B: explicit target only
$ matty pack status
error: pack and --surface are required
hint: `matty pack list` shows available packs and per-surface activation

$ matty pack status engram --surface opencode
engram 1.19.0 on opencode
Intent: active at revision 6
Latest attempt: recovery-required (P-c430)
Readiness: configured=no, authorized=no, usable=no
Next: `matty pack reconcile engram --surface opencode`

Shared invariant:
  Status freshly inspects hosts and performs no mutation.
  Readiness never aliases intent or the latest attempt result.
""")


def blockers() -> None:
    print("""DECISION 5 — dependency and conflict feedback

Alternative A: report the complete known blocker set
$ matty pack activate engram --surface opencode
Cannot preview activation: 3 blockers

UNSATISFIED GLOBAL REQUIREMENT
  engram >=1.19.0 was not found on PATH
  This is a host-global tool requirement, not an OpenCode resource.

CAPABILITY CONFLICT
  engram provides `persistent-memory`
  active pack `other-memory` conflicts with `persistent-memory`
  Matty will not choose a winner or deactivate another pack automatically.

OWNERSHIP AMBIGUITY
  MCP key `engram` already exists with unmanaged configuration
  Matty will preserve it and will not adopt or overwrite it.

Result: no plan; zero actions; intent revision unchanged.

Alternative B: stop at the first blocker
$ matty pack activate engram --surface opencode
error: required global tool `engram >=1.19.0` was not found on PATH
Result: no plan; zero actions; intent revision unchanged.

Supported acquisition is a plan, not a blocker:
PHASE 2  executable / external effects
  ! install global tool: brew install engram
Approval required: executable-action

Shared invariant:
  Preview evaluates the complete desired state and never mutates it.
  Conflicts never use silent last-writer-wins behavior.
""")


def deactivate() -> None:
    print("""DECISION 6 — destructive cleanup confirmation

$ matty pack deactivate engram --surface codex
Plan P-d310  deactivate engram from codex
Intent revision: 5 -> 6

PHASE 1  reversible local projections
  = retain shared skill `engram-memory` (still required by matty)
  - remove engram as contributor from shared instruction `memory-policy`
Approve reversible deactivation changes? [y/N] y

PHASE 3  destructive owned cleanup
  - delete MCP server `engram` (last contributor; fingerprint verified)
  - delete generated instruction `engram-bootstrap` (last contributor; fingerprint verified)
Approval required: destructive-cleanup [plan P-d310, phase cleanup-31ad]

Alternative A: explicit yes/no phase approval
Delete these 2 verified Matty-owned projections? [y/N] y

Alternative B: challenge phrase
Type `engram` to delete these 2 verified Matty-owned projections: engram

Preserved, not deleted:
  ! ~/.codex/modified-by-user.md differs from Matty's last verified fingerprint
Pending human action: inspect and remove this ambiguous target manually if desired.

Shared invariant:
  Approval applies only to the listed cleanup phase and exact plan digest.
  A changed fingerprint makes the plan stale; Apply executes zero actions.
""")


def stale() -> None:
    print("""DECISION 7 — stale-plan follow-up

$ matty pack update engram --surface codex
Plan P-901e  update engram 1.19.0 -> 1.20.0 on codex
Approve reversible local changes? [y/N] y
Run external setup command? [y/N] y

STALE — plan P-901e was not applied
Changed precondition:
  Codex MCP configuration fingerprint: cfg-22a1 -> cfg-94c0
Result: zero actions; intent revision unchanged; all approvals invalidated.

Alternative A: stop and rerun
Next: rerun `matty pack update engram --surface codex` to inspect and approve a new plan.

Alternative B: display an unapproved replacement preview now
Fresh replacement preview (NOT APPROVED):
Plan P-a82b  update engram 1.19.0 -> 1.20.0 on codex
  ! preserve newly changed MCP configuration
  + update 2 reversible local projections
No actions were applied.
Next: rerun `matty pack update engram --surface codex` to approve P-a82b.

Shared invariant:
  Apply never silently replans and never carries approval to a replacement plan.
""")


def recovery() -> None:
    print("""DECISION 8 — recovery entry point

$ matty pack activate engram --surface opencode
...
PHASE 1 reversible local projections: applied and verified
PHASE 2 executable / external effects: FAILED at `engram setup opencode`

RECOVERY REQUIRED — attempt A-44b2
Intent: engram remains active at revision 8
Completed: 3 verified owned projections
Failed: external setup command (exit 1)
Not started: runtime verification
No speculative rollback was attempted.

Alternative A: canonical reconcile command
Next: `matty pack reconcile engram --surface opencode`

$ matty pack reconcile engram --surface opencode
Recovery plan P-r810 (new plan; attempt A-44b2 retained for history)
  = preserve 3 verified projections
  ! retry external setup command
  ? verify configured / authorized / usable
Approve executable action? [y/N]

Alternative B: repeat the originating verb
Next: `matty pack activate engram --surface opencode`

$ matty pack activate engram --surface opencode
Intent is already active; preparing recovery plan P-r810 for attempt A-44b2
  = preserve 3 verified projections
  ! retry external setup command
  ? verify configured / authorized / usable
Approve executable action? [y/N]

Shared invariant:
  Recovery starts from fresh inspection and a new plan with new approvals.
  It never mutates or replays the old plan.
""")


def pending() -> None:
    print("""DECISION 9 — pending human action and exit result

$ matty pack activate engram --surface codex
...
VERIFIED — exact plan P-7c2a applied
Readiness: configured=yes, authorized=no, usable=no

PENDING HUMAN ACTION
  Authenticate Engram in Codex. Matty cannot perform or approve host login.
  After login: `matty pack status engram --surface codex`

Alternative A: Apply succeeded; readiness can be gated separately
Process exit: 0

$ matty pack status engram --surface codex --require usable
Readiness: configured=yes, authorized=no, usable=no
error: engram on codex does not satisfy required readiness `usable`
Process exit: nonzero

Alternative B: lifecycle command exits nonzero until usable
error: approved changes verified, but engram on codex is not usable
Process exit: nonzero

Shared invariant:
  Attempt result and readiness are separate facts.
  Authentication, trust, and reload remain host-owned pending human actions.
""")


def reconcile() -> None:
    print("""DECISION 10 — reconcile targeting

Alternative A: explicit pack and surface required
$ matty pack reconcile engram --surface codex
Plan P-rc21  repair engram on codex toward intent revision 8
  ~ restore drifted owned instruction `engram-memory` [reversible local]
  = retain shared skill `memory` with contributors {matty, engram}
  ! preserve unmanaged MCP key `custom-memory` [pending human action]
Intent revision: unchanged (8)
Approve reversible repair? [y/N]

Alternative B: optional bulk reconcile for a surface
$ matty pack reconcile --surface codex
Plan P-rc44  repair 2 active packs on codex toward intent revision 8
  matty:
    ~ restore drifted owned instruction `matty-core` [reversible local]
  engram:
    ~ restore drifted owned instruction `engram-memory` [reversible local]
  shared:
    = retain skill `memory` with contributors {matty, engram}
    ! preserve unmanaged MCP key `custom-memory` [pending human action]
Intent revision: unchanged (8)
Approve 2 reversible repairs? [y/N]

Shared invariant:
  Planning always considers complete desired state and contributor sets.
  Reconcile changes projections toward current intent; it never changes intent.
""")


def update() -> None:
    print("""DECISION 11 — update version selection

Alternative A: target the current Matty-owned catalog version
$ matty pack update engram --surface codex
Plan P-u120  update engram 1.19.0 -> 1.20.0 on codex
Source: Matty-owned catalog (current: 1.20.0)
  ~ update 2 reversible local projections
  = retain shared skill `memory` and contributor set
  ! run `engram setup codex` after local commit

$ matty pack update engram --surface codex --version 1.19.0
error: unknown flag --version

Alternative B: allow explicit selection among catalog versions
$ matty pack update engram --surface codex --version 1.19.0
Plan P-u119  update engram 1.18.0 -> 1.19.0 on codex
Source: Matty-owned catalog (selected: 1.19.0; current: 1.20.0)
  ~ update 2 reversible local projections
  = retain shared skill `memory` and contributor set

Shared invariant:
  Update changes durable target intent only after stale checks pass.
  It never fetches an unlisted or remote pack version.
""")


def dependency() -> None:
    print("""DECISION 12 — inactive pack dependencies during activation

Candidate example uses placeholder pack names only to test composition UX;
the initial proof catalog remains limited to `matty` and `engram`.

Alternative A: block and sequence explicitly
$ matty pack activate dependent-pack --surface codex
Cannot preview activation: 1 blocker
INACTIVE PACK REQUIREMENT
  dependent-pack requires base-pack on codex
  Next: `matty pack activate base-pack --surface codex`
  Then rerun: `matty pack activate dependent-pack --surface codex`
Result: no plan; zero actions; intent revision unchanged.

Alternative B: one combined, explicit activation plan
$ matty pack activate dependent-pack --surface codex
Plan P-dep2  activate 2 packs on codex
Requested: dependent-pack
Required activation: base-pack (currently inactive)
Intent revision: 12 -> 13
  base-pack: 2 reversible projections
  dependent-pack: 1 reversible projection
Approve activation of base-pack AND dependent-pack? [y/N]

Shared invariant:
  No dependency activation is hidden in Apply.
  Deactivating base-pack while dependent-pack needs it is rejected without cascade.
""")


def noop() -> None:
    print("""DECISION 13 — already-converged lifecycle commands

Alternative A: successful no-op
$ matty pack update engram --surface codex
engram on codex is already at catalog-current version 1.20.0.
Intent revision: unchanged (13)
Projections: configured; no drift found
No plan, approval, or Apply was needed.
Process exit: 0

$ matty pack reconcile engram --surface codex
engram on codex already matches intent revision 13.
No plan, approval, or Apply was needed.
Process exit: 0

Alternative B: invalid-operation error
$ matty pack update engram --surface codex
error: engram on codex is already at catalog-current version 1.20.0
Process exit: nonzero

Shared invariant:
  Fresh inspection proves convergence; zero state, file, or external mutations occur.
""")


def complete() -> None:
    print("""CONSOLIDATED PACK LIFECYCLE UX

Discovery and inspection (always side-effect free)
  matty pack list
  matty pack show <pack>
  matty pack status [<pack> --surface <surface>] [--require usable]

Lifecycle (Preview inline; interactive typed approvals; exact Apply)
  matty pack activate <pack> --surface <surface> [--dry-run]
  matty pack update <pack> --surface <surface> [--dry-run]
  matty pack deactivate <pack> --surface <surface> [--dry-run]
  matty pack reconcile [<pack>] --surface <surface> [--dry-run]

Effect legend shown in every preview
  reversible local       staged/validated; rollback within the local transaction
  executable / external  runs after local commit; failure stops at a barrier
  destructive cleanup    only verified last-contributor targets; separate approval

Outcome matrix
  blocked             all known requirement/conflict/ownership blockers; no plan
  declined            no Apply; zero actions; intent unchanged
  no-op               freshly converged; no approval or Apply; exit 0
  stale               changed precondition; zero actions; rerun original command
  verified            approved plan applied exactly; readiness reported separately
  pending human       verified may exit 0; status --require usable is the readiness gate
  recovery-required   truthful completed/failed/not-started facts; no speculative rollback
  recovery            repeat originating verb; fresh plan and fresh approvals

Composition and ownership
  activation may combine explicitly listed required packs in one plan
  deactivation of a required pack is rejected without cascade
  shared projections remain while contributors exist
  drifted/ambiguous/unmanaged targets are preserved for human action
  reconcile can target one pack or every active pack on a surface; intent never changes

Version and interaction limits
  update targets catalog-current only; initial catalog is Matty-owned
  proof is limited to matty and engram; web/mobile remain deferred
  Apply requires a TTY; no --yes, serialized plan file, or unattended receipt transport
""")


SCENARIOS = {
    "discovery": discovery,
    "activation": activation,
    "unattended": unattended,
    "status": status,
    "blockers": blockers,
    "deactivate": deactivate,
    "stale": stale,
    "recovery": recovery,
    "pending": pending,
    "reconcile": reconcile,
    "update": update,
    "dependency": dependency,
    "noop": noop,
    "complete": complete,
}


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in SCENARIOS:
        print("usage: prototype.py <scenario>")
        print("scenarios: " + ", ".join(SCENARIOS))
        return 2
    SCENARIOS[sys.argv[1]]()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
