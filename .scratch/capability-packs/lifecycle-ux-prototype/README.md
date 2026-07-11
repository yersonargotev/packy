# Pack lifecycle UX prototype

> **THROWAWAY PROTOTYPE** — discussion aid only; this is not capability-pack implementation.

## Question under test

Which CLI interaction makes the complete pack lifecycle understandable while preserving Matty's existing preview and safety conventions?

The prototype prints candidate terminal transcripts. It keeps all state in memory, never invokes Matty or an external command, and never reads or writes `HOME`/`XDG_CONFIG_HOME`.

## Run

```sh
python3 .scratch/capability-packs/lifecycle-ux-prototype/prototype.py discovery
```

Available scenarios will grow only as their decisions are discussed. Run without an argument to list them.

## Fixed constraints

- The CLI adapts only `Preview`, `Apply`, and `Status` from the deep `internal/capabilitypack` module.
- Preview is side-effect free. Apply executes the exact approved plan or returns stale with zero actions.
- Reversible local changes, executable/external effects, and destructive cleanup remain visibly distinct phases with typed approvals.
- Authentication, host trust, reloads, and other pending human actions are reported rather than treated as apply approvals.
- Recovery always begins with fresh inspection and a new plan requiring new approval.
- Readiness reports `configured`, `authorized`, and `usable` independently from reconciliation-attempt state.

## Decision 1: discovery and command grammar

**Confirmed:** use the noun hierarchy under `matty pack`:

- `matty pack list`
- `matty pack show engram`
- `matty pack activate|update|deactivate|reconcile|status ...`

This keeps pack behavior grouped and preserves a narrow Matty root interface. `--surface codex|opencode` remains explicit; no command implies activation on every surface.

## Decision 2: preview-to-apply interaction

**Confirmed:** use one interactive lifecycle command. It calls `Preview`, renders the immutable exact plan, gathers each required typed approval inline, and passes that plan plus its receipts to `Apply`.

`--dry-run` calls only `Preview`, prints the same plan, requests no approvals, and stops. The initial UX has no serialized plan file or public `pack apply` command even though Preview and Apply remain separate facade operations internally.

## Decision 3: unattended and scripted use

**Confirmed:** applying effects is interactive-only in the initial UX. A lifecycle command that reaches an approval requires a TTY. Non-interactive callers may use `--dry-run` and `status`, which are side-effect free.

There is no generic `--yes` and no pre-issued receipt transport in the initial scope. Automated apply stays deferred until a concrete supervised-automation use case can define who issues and protects receipts.

## Decision 4: status entry point and density

**Confirmed:** `matty pack status` gives an overview across every pack/surface pair. `matty pack status <pack> --surface <surface>` drills into intent, latest attempt, readiness, projections, and pending human actions.

Both forms perform fresh inspection without mutation. `configured`, `authorized`, and `usable` remain separate from intent and reconciliation-attempt outcome.

## Decision 5: dependency and conflict feedback

**Confirmed:** Preview evaluates the complete desired state and reports all currently known blockers together, grouped as unsatisfied global requirements, pack conflicts, and unsafe ownership ambiguity.

Planning blockers yield no plan, zero actions, and no intent change. A missing global tool is never represented as a surface resource. When Matty supports acquiring it, acquisition appears as an executable/external plan phase with separate typed approval.

## Decision 6: destructive cleanup confirmation

**Confirmed:** after listing every last-contributor deletion, ask an explicit descriptive `[y/N]` question for the `destructive-cleanup` phase.

Safety comes from the typed receipt bound to the exact plan/phase and from fresh fingerprint validation, not from a challenge phrase. Shared projections with remaining contributors stay in place; drifted or ambiguous targets are preserved as pending human actions.

## Decision 7: stale-plan follow-up

**Confirmed:** explain the changed precondition, guarantee zero actions, invalidate every approval, and stop. The user reruns the lifecycle command to obtain and approve a fresh plan.

The stale invocation performs no automatic replacement Preview, making the no-silent-replan rule visible.

## Decision 8: recovery entry point

**Confirmed:** repeat the originating lifecycle verb. `activate`, `update`, or `deactivate` detects that durable intent already reflects the approved target, clearly announces recovery planning, freshly inspects reality, and previews a new plan.

The old attempt remains history and is never replayed. `reconcile` remains the explicit operation for repairing drift toward current intent rather than the generic spelling for every failed lifecycle operation.

## Decision 9: pending human action and exit result

**Confirmed:** exit zero when the exact approved plan applies and verifies, even if readiness remains incomplete. Prominently report pending human actions and the follow-up status command.

`matty pack status <pack> --surface <surface> --require usable` is the separate inspection-only readiness gate for automation. Apply success never claims that configured implies authorized or usable.

## Decision 10: reconcile targeting

**Confirmed:** support both scopes:

- `matty pack reconcile <pack> --surface <surface>` for targeted diagnosis and repair;
- `matty pack reconcile --surface <surface>` for all active packs on that surface.

Both plan against the complete desired state and contributor sets, preserve unmanaged/ambiguous content, never change activation intent, and keep effect classes separate.

## Decision 11: update version selection

**Confirmed:** `matty pack update <pack> --surface <surface>` targets the single current version in the Matty-owned catalog. The initial UX has no `--version` flag, historical-version retention, or downgrade policy.

Update still previews effects across the complete desired state, preserves unchanged shared projections/contributors, and never fetches remote or unlisted packs.

## Decision 12: inactive pack dependencies during activation

**Confirmed:** activating a requested pack may include its inactive required packs in one combined desired-state plan and approval interaction.

The preview must distinguish requested from required activations and show every effect before approval. Apply cannot add a dependency that was absent from the approved plan. Deactivating a required pack remains rejected without cascade.

## Decision 13: already-converged lifecycle commands

**Confirmed:** already-converged lifecycle commands report a successful no-op, request no approval, skip Apply, preserve the intent revision, and exit zero after fresh inspection.

This applies to already-active activation, catalog-current update, already-inactive deactivation, and reconcile with no drift.

## Consolidated interaction contract

Run `python3 .scratch/capability-packs/lifecycle-ux-prototype/prototype.py complete` for the consolidated command surface, effect-class legend, and outcome matrix. This is the candidate shared understanding to retain from the throwaway prototype.
