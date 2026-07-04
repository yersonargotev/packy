# Proposal: Declarative Picker Navigation (Computed Flow Slice)

## Intent

The installer TUI's conditional picker chain (Preset → Claude → Kiro → Codex → SDDMode → ModelPicker → StrictTDD → OpenCodePlugins → DependencyTree) has its ordering **triplicated** across `internal/tui/model.go`: forward transitions in `confirmSelection()`/HandleNav callbacks, "Back row" transitions also in `confirmSelection()`, and Esc transitions in `goBack()`. Multiple `// NOTE: ... keep in sync` comments mark the hazard. This duplication already caused two real bugs (Codex Back row inert; SDDMode back skipping Codex). Goal: make the chain a single source of truth so desync bugs cannot recur.

## Scope

### In Scope
- New method `(m Model) pickerFlowSlice() []Screen` — computes the ordered conditional chain from `m.Selection`, filtered by existing `shouldShow*` predicates; includes the PresetCustom ordering inversion (DependencyTree position).
- New helpers `pickerNextScreen`/`pickerPreviousScreen` that scan that slice forward/backward from the current screen.
- Redirect the picker-chain transitions onto the slice and remove the duplicated blocks (`model.go`):
  - SDDMode back: `~1993–2015` (confirmSelection) + `~3053–3082` (goBack).
  - StrictTDD back: `~2134–2152` + `~3011–3043`.
  - DependencyTree back: `~2208–2229` + `~2985–3004`.
  - Claude/Kiro/Codex HandleNav forward transitions: `~1129–1154`, `~1179–1199`, `~1256–1272`.
- Unit test for `pickerFlowSlice` (table-driven, per Selection state) before wiring.

### Out of Scope (non-goals)
- The `shouldShow*` predicates themselves — unchanged.
- Any view/render code; no golden output changes.
- Unrelated nav graphs: backups, uninstall, agent-builder, profiles, upgrade/sync.
- ModelConfigMode and OpenCodePluginsStandalone exit-ramps stay as early-return guards (not folded into the slice).

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None — pure internal refactor; no spec-level behavior change. External navigation behavior is invariant (enforced by existing tests).

## Approach

Approach **B (computed flow slice)** from exploration. The chain is already an ordered sequence filtered by predicates; make that explicit as one `[]Screen`. Both Enter (Back row, in `confirmSelection`) and Esc (in `goBack`) navigate by scanning the same slice via `pickerPreviousScreen`/`pickerNextScreen`. No router coupling — `model.go` stays self-contained; `linearRoutes` (router.go) is untouched because it cannot express conditional presence or preset-dependent order.

## Invariants (acceptance backbone)

- All 11 `TestInstallNavigationRoundTrips` cases pass unchanged, including the 2 regression cases (Codex back, SDDMode back skipping Codex).
- `TestPresetSelectionNextScreenFlowMatrix` / `TestCustomPresetPostComponentFlowMatrix` goldens unchanged.
- ModelConfigMode exit-ramps still return to `ScreenModelConfig`.
- OpenCodePluginsStandalone guard preserved.
- `ScreenModelPicker` appears only when `SDDMode == Multi` AND model cache present (slice calls the same `osStatModelCache` stat).
- `osStatModelCache` remains an injectable package var.
- PresetCustom ordering inversion preserved.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/tui/model.go` | Modified | Add `pickerFlowSlice` + next/prev helpers; collapse triplicated chain blocks onto them |
| `internal/tui/model_test.go` (or new) | Modified/New | Add `pickerFlowSlice` unit test |
| `internal/tui/router.go` | Unchanged | `linearRoutes` not touched |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Behavior drift from a chain edge | Med | Round-trip tests are the safety net; run before/after |
| PresetCustom inversion mishandled | Med | Slice builds DependencyTree position from Preset; covered by custom matrix test |
| Strict TDD active in repo | High (constraint) | Apply phase MUST follow RED-GREEN-REFACTOR: write `pickerFlowSlice` test first, then wire helpers |
| Slice allocation per keypress | Low | Cheap; acceptable for TUI |

## Rollback Plan

Single cohesive refactor in one branch. Revert the branch/commit — `model.go` returns to triplicated transitions; no schema, data, or external contract changes.

## Dependencies

None. Self-contained in `internal/tui`.

## Delivery

Estimated ~250–350 changed lines (new method + helpers add lines; collapsing 3 duplicated blocks removes lines). Fits one PR within the ~400-line budget. Delivery strategy: `ask-on-risk`.

## Success Criteria

- [ ] `pickerFlowSlice` is the single source of the conditional chain order; no remaining `keep in sync` comments for it.
- [ ] All 11 round-trip cases + both flow-matrix goldens pass unchanged.
- [ ] All listed invariants hold.
- [ ] Diff stays within one PR (~400 lines) or splits per `ask-on-risk`.
