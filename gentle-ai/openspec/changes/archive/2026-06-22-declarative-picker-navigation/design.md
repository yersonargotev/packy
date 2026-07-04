# Design: Declarative Picker Navigation (Computed Flow Slice)

## Technical Approach

Implements proposal Approach **B**: replace the triplicated conditional picker
chain in `internal/tui/model.go` with one ordered `[]Screen` computed from
`m.Selection` and filtered by the existing `shouldShow*` predicates. Forward
(Enter on a picker / Continue), the "← Back" row (Enter on the last option), and
Esc (`goBack`) all converge on two scanners over that same slice:
`pickerNextScreen` / `pickerPreviousScreen`. `router.go` `linearRoutes` is
untouched — it cannot express conditional presence or the custom-preset
ordering inversion. Predicates, views, and goldens are unchanged.

**This refactor is NOT a pure no-behavior-change refactor.** The Esc/`goBack`
paths are behavior-preserving. However, the "← Back" row (Enter on the Back
option) paths intentionally fix 4 pre-existing inconsistencies between `goBack`
(Esc) and `confirmSelection` (Enter on Back row):

1. **StrictTDD Back row (Enter) in `confirmSelection`** only checked
   `shouldShowSDDModeScreen` and `shouldShowClaudeModelPickerScreen` — it
   skipped Codex and Kiro. After refactor, `pickerPreviousScreen` correctly
   walks Codex→Kiro→Claude via the slice (mirroring what `goBack` already did
   at lines 3024–3032).
2. **Codex Back row (Enter) in `confirmSelection`** for the PresetCustom case
   went to `ScreenPreset` instead of `ScreenDependencyTree` — the same bug
   that `goBack` at line 3091–3097 already handled correctly.
3. **DependencyTree Back row (Enter) in `confirmSelection`** (lines 2208–2229)
   lacked the `shouldShowOpenCodePluginsScreen()` early check that `goBack`
   (lines 2991–2993) has, causing Enter vs Esc divergence on the
   DependencyTree→OpenCodePlugins edge.
4. **`goBack` Codex picker in custom-only-Codex** went to `ScreenPreset`
   (via `linearRoutes` fallthrough) instead of `ScreenDependencyTree` — the
   `ScreenCodexModelPicker` block at lines 3110–3120 did not branch on custom.

These four inconsistencies are fixed as a deliberate side-effect of unification.
They are explicitly tested with RED-first test cases (see Testing Strategy).

Verified forward order (non-custom):
`Preset → [Claude]* → [Kiro]* → [Codex]* → [SDDMode]* → [ModelPicker]** → [StrictTDD]* → [OpenCodePlugins]*** → DependencyTree`.
(`*` = conditional on `shouldShow*`; `**` = only when SDDMode==Multi AND model cache present; `***` = early-return guard, not in slice)
Custom inverts the picker block:
`Preset → DependencyTree → [Claude]* → [Kiro]* → [Codex]* → [SDDMode]* → [ModelPicker]** → [StrictTDD]* → [OpenCodePlugins]*** → (SkillPicker) → Review`.

## Architecture Decisions

| Decision | Choice | Rejected alternative | Rationale |
|----------|--------|----------------------|-----------|
| Source of order | One `pickerFlowSlice()` built per call | Extend `linearRoutes` | Routes are static maps; cannot encode predicate-gated presence or preset inversion |
| Slice endpoints | Include `ScreenPreset` and `ScreenDependencyTree` as anchors of the picker block | Slice only the optional pickers | Anchors let next/prev resolve the first/last hop (Preset↔first picker, last picker↔DependencyTree) without special cases |
| Custom inversion | Slice itself encodes position of `ScreenDependencyTree` (after Preset in custom, after pickers in non-custom) | Branch at every call site | Keeps inversion in ONE place |
| ModelPicker gate | Include `ScreenModelPicker` only when `SDDMode==Multi` AND `osStatModelCache(opencode.DefaultCachePath())==nil err` | Always include, skip at runtime | Matches current behavior exactly; keeps `osStatModelCache` the single injectable stat |
| Cross-cutting guards | Keep ModelConfigMode / OpenCodePluginsStandalone / Upgrade-Esc as early returns BEFORE slice walk | Fold into slice | They are exit-ramps to unrelated graphs, not chain members |
| Allocation | Rebuild slice per keypress | Cache on Model | Slice is ≤8 elements; trivial cost, no stale-state risk |

## Interfaces / Contracts

```go
// pickerFlowSlice returns the ordered conditional picker chain for the current
// Selection, filtered by shouldShow* predicates. Anchors (Preset, DependencyTree)
// are always present so next/prev can resolve chain endpoints.
func (m Model) pickerFlowSlice() []Screen {
    custom := m.Selection.Preset == model.PresetCustom
    s := []Screen{ScreenPreset}
    if custom {
        s = append(s, ScreenDependencyTree) // component selector precedes pickers
    }
    if m.shouldShowClaudeModelPickerScreen() { s = append(s, ScreenClaudeModelPicker) }
    if m.shouldShowKiroModelPickerScreen()   { s = append(s, ScreenKiroModelPicker) }
    if m.shouldShowCodexModelPickerScreen()  { s = append(s, ScreenCodexModelPicker) }
    if m.shouldShowSDDModeScreen() {
        s = append(s, ScreenSDDMode)
        if m.Selection.SDDMode == model.SDDModeMulti {
            if _, err := osStatModelCache(opencode.DefaultCachePath()); err == nil {
                s = append(s, ScreenModelPicker)
            }
        }
    }
    if m.shouldShowStrictTDDScreen() { s = append(s, ScreenStrictTDD) }
    if !custom { s = append(s, ScreenDependencyTree) }
    return s
}

// pickerNextScreen / pickerPreviousScreen find m.Screen in the slice and step.
// ok=false when m.Screen is not a chain member (caller falls through to
// existing logic / linearRoutes). Endpoints: prev(Preset)=false, next(DependencyTree)=false.
func (m Model) pickerNextScreen() (Screen, bool)
func (m Model) pickerPreviousScreen() (Screen, bool)
```

Note: `ScreenOpenCodePlugins` and `ScreenSkillPicker` are intentionally NOT in
the slice — Plugins is an early-return guard (`shouldShowOpenCodePluginsScreen`)
and SkillPicker sits after StrictTDD only in custom and keeps its own handling.
The slice covers exactly the triplicated blocks named in the proposal.

**Invariant — `pickerFlowSlice` is `m.Screen`-independent**: all membership
predicates (`shouldShowClaudeModelPickerScreen`, `shouldShowKiroModelPickerScreen`,
`shouldShowCodexModelPickerScreen`, `shouldShowSDDModeScreen`,
`shouldShowStrictTDDScreen`) depend only on `m.Selection`, never on `m.Screen`.
`shouldShowOpenCodePluginsScreen()` IS screen-sensitive (returns false when
`m.Screen == ScreenPreset`), which is exactly why OpenCodePlugins is NOT in the
slice but remains an early-return guard called AFTER `pickerNextScreen` resolves
the target. This invariant must be maintained: no predicate that reads `m.Screen`
may be added to `pickerFlowSlice`.

## Data Flow

    Enter on picker (HandleNav) ─┐
    Enter on "← Back" row ───────┤
    Esc (goBack) ────────────────┴─→ guards (ModelConfigMode / Upgrade / Standalone)
                                       │ not a guard case
                                       ▼
                          pickerNextScreen / pickerPreviousScreen
                                       │ ok
                                       ▼  ok=false → existing fallthrough
                                  m.setScreen(target)   (cursor reset to 0)

## Call-Site Rewrite Plan

`setScreen` already resets `Cursor=0`, so helpers only return the target.

1. **Claude HandleNav forward** (`model.go:1129–1156`): replace the
   `else if shouldShowKiro… / Codex… / SDDMode… / custom… / StrictTDD… / else DependencyTree`
   ladder (after the ModelConfigMode branch at 1120–1128) with:
   `else if next, ok := m.pickerNextScreen(); ok { if next==ScreenDependencyTree { m.buildDependencyPlan() }; m.applyPickerEntry(next) }`.
   Picker-state init moves into `applyPickerEntry(next Screen)` — see shape note below.
2. **Kiro HandleNav forward** (`1179–1201`): same collapse.
3. **Codex HandleNav forward** (`1256–1275`): same collapse.
4. **SDDMode forward** (`1952–1988`): keep the `SDDModeMulti → ModelPicker` and
   single-mode assignment clearing; replace the post-SDDMode target ladder with
   `pickerNextScreen`. (ModelPicker entry already explicit here.)
5. **SDDMode Back row** (`1993–2015`, both custom + non-custom branches): replace
   entire block with `if prev, ok := m.pickerPreviousScreen(); ok { m.applyPickerEntry(prev) }`.
   Delete both `// NOTE: keep in sync` comments.
6. **ModelPicker Continue/Back** (`2060–2108`): Continue → `pickerNextScreen`;
   Back (`2103–2108`) → `pickerPreviousScreen` (resolves to SDDMode). ModelConfigMode
   early-return stays.
7. **StrictTDD Back** (`2134–2152`): replace the
   `SDDMode/ModelPicker/Claude/custom/Preset` ladder with `pickerPreviousScreen`.
8. **StrictTDD forward** (`2114–2132`): In custom mode the slice ends at
   StrictTDD, so `pickerNextScreen(StrictTDD)` returns `ok=false`. The forward
   path MUST preserve this exact control flow in order:
   (a) `shouldShowOpenCodePluginsScreen()` guard → `ScreenOpenCodePlugins`
       (early return, not a slice member);
   (b) `m.Selection.Preset == model.PresetCustom` branch:
       `shouldShowSkillPickerScreen()` → `ScreenSkillPicker`,
       else `BuildReviewPayload` → `ScreenReview`;
   (c) `ok=false` from `pickerNextScreen` means "fall through to (a)/(b)" — it
       does NOT mean "rebuild dependency plan" or "go to DependencyTree". The
       Review fallback in (b) MUST NOT be dropped. For non-custom where the
       slice includes DependencyTree, `pickerNextScreen` returns `ok=true` and
       the standard hop is used after calling `buildDependencyPlan()`.
9. **DependencyTree Back** (`2208–2229`, confirmSelection) + **goBack
   DependencyTree** (`2986–3004`): non-custom only. Keep `isPiOnlyAgents` and
   `shouldShowOpenCodePluginsScreen` early checks, then `pickerPreviousScreen`.
   Delete the `// NOTE: keep in sync` comment.
10. **goBack** picker blocks — `ScreenStrictTDD` (`3011–3043`), `ScreenSDDMode`
    (`3053–3082`), `ScreenClaudeModelPicker` custom (`3084–3088`),
    `ScreenKiroModelPicker` (`3090–3108`), `ScreenCodexModelPicker` (`3110–3120`):
    replace each with `pickerPreviousScreen`. This is where Esc converges with the
    Enter "← Back" row.

**`applyPickerEntry(next Screen)` shape (MANDATORY).** The helper initializes
the target picker's state for ANY target it may be navigated to — including
cases where Claude is absent and the first picker from `ScreenDependencyTree`
(custom) is Kiro or Codex. Required initializations per target:

| Target | Initialization |
|--------|---------------|
| `ScreenClaudeModelPicker` | `m.ClaudeModelPicker = screens.NewClaudeModelPickerStateFromPhaseAssignments(...)` |
| `ScreenKiroModelPicker` | `m.KiroModelPicker = screens.NewKiroModelPickerStateFromAssignments(m.Selection.KiroModelAssignments)` |
| `ScreenCodexModelPicker` | `m.CodexModelPicker = screens.NewCodexModelPickerStateFromAssignments(m.Selection.CodexModelAssignments)` |
| `ScreenModelPicker` | `m.ModelPicker = screens.NewModelPickerState(cachePath, ...)` |
| Others | `m.setScreen(next)` only (no extra state) |

The current `ScreenDependencyTree` Continue block (line 2171) initializes only
`ClaudeModelPicker` before calling `setScreen`. After refactor, the block calls
`applyPickerEntry` which must handle Kiro-first and Codex-first entry too —
i.e., when only Kiro or only Codex is selected (no Claude) and custom mode
navigates directly from DependencyTree to those pickers.

The "← Back" row Enter handlers in confirmSelection (`ScreenClaudeModelPicker`
`1904–1919`, Kiro `1920–1935`, Codex `1936–1951`) keep their ModelConfigMode
early return, then call `pickerPreviousScreen` — same target as goBack.

## Cross-Cutting Guards (early-return, BEFORE slice walk — do NOT fold in)

- **ModelConfigMode** exit-ramp → `ScreenModelConfig` (sets `ModelConfigMode=false`):
  forward (1120,1171,1236,2062), back row (1908,1922,1938,2103), goBack (2945).
- **OpenCodePluginsStandalone** → `goBackFromOpenCodePlugins` / Welcome (2882, 3046, 3591).
- **Upgrade Esc-quit**: `GentleAIUpgradeVersion` → `tea.Quit` (1392) — runs before
  `goBack` is even reached; unchanged.

## Edge Cases

- Cursor reset: handled by `setScreen` (`3175`) — helpers never touch cursor.
- Picker internal sub-modes: Claude `InCustomMode`, Kiro `InCustomMode`, Codex
  `CustomMode != CodexCustomModeNone` consume Esc/Back inside `Handle*Nav` BEFORE
  the chain is reached; slice logic only runs when `updated/assignments != nil`
  (a real confirm) or on the explicit "← Back" row. Cursor-reset-on-exit
  (`1113–1115`, `1166–1168`, `1212–1214`) is preserved verbatim.
- ModelPicker separator row (`2049`, `SeparatorRowIdx`) and empty-provider mode
  (`AvailableIDs==0`, `2019–2045`) keep their dedicated pre-slice handling; only
  the post-decision target ladders are replaced by the helpers.
- Empty/edge slice: `pickerNextScreen(DependencyTree)` and
  `pickerPreviousScreen(Preset)` return `ok=false` → caller fallthrough unchanged.

## Testing Strategy (Strict TDD ACTIVE — RED first)

| Layer | What | Approach |
|-------|------|----------|
| Unit | `pickerFlowSlice` order/membership per Selection | Table-driven over preset × agents × SDDMode × cache; assert exact `[]Screen`. Use `withModelCache` helper + `osStatModelCache` override for the Multi gate. RED before any wiring. |
| Unit | `pickerNextScreen`/`pickerPreviousScreen` | Table over (slice state, current screen) → (target, ok); cover ALL endpoints (`ok=false` for `prev(Preset)` and `next(DependencyTree)` in non-custom, `next(StrictTDD)` in custom) AND non-member screens (`ScreenModelConfig`, `ScreenSkillPicker`, `ScreenReview`) which must also return `ok=false`. RED before wiring. |
| Regression | `TestInstallNavigationRoundTrips` (all 11) | Net for Esc (forward+reverse parity); run unchanged before and after. |
| Golden (unchanged) | `TestPresetSelectionNextScreenFlowMatrix`, `TestCustomPresetPostComponentFlowMatrix` | MUST pass without `-update`. No view/golden edits allowed. |

**RED-first "← Back" row (Enter) tests required** — these cover the 4 fixed
inconsistencies and MUST be written before implementation:

| Test case | What to assert |
|-----------|----------------|
| Codex Back row, non-custom, Codex-only | Enter on Back row of `ScreenCodexModelPicker` → `ScreenPreset` (not `ScreenDependencyTree`). Verifies finding #1 / bug #2 fix. |
| Codex Back row, non-custom, Kiro+Codex | Enter on Back row → `ScreenKiroModelPicker`. |
| Codex Back row, custom, Codex-only | Enter on Back row of `ScreenCodexModelPicker` → `ScreenDependencyTree` (currently bug: goes to `ScreenPreset`). RED must fail before fix. |
| StrictTDD Back row, Codex+no Claude | Enter on Back row of `ScreenStrictTDD` → `ScreenCodexModelPicker` (currently bug: skips Codex, only checks Claude/SDDMode). RED must fail before fix. |
| StrictTDD Back row, Kiro+no Claude+no Codex | Enter on Back row → `ScreenKiroModelPicker` (same latent bug). |
| DependencyTree Back row (non-custom, OpenCode, no StrictTDD/SDDMode) | Enter on Back row → `ScreenOpenCodePlugins` (currently bug: `confirmSelection` lacks `shouldShowOpenCodePluginsScreen` check that `goBack` has). RED must fail before fix. |
| `applyPickerEntry` from DependencyTree, Kiro-only custom | DependencyTree Continue → `ScreenKiroModelPicker` with properly initialized `KiroModelPicker` state (not zero value). |
| `applyPickerEntry` from DependencyTree, Codex-only custom | DependencyTree Continue → `ScreenCodexModelPicker` with properly initialized `CodexModelPicker` state. |

RED-GREEN-REFACTOR: write `pickerFlowSlice` + helper tests first (fail to
compile/assert), implement the methods (GREEN), then collapse the call sites
(REFACTOR) keeping round-trips and Back-row tests green at each step.

## Migration / Rollout

No migration. Single cohesive refactor; revert the branch to restore triplicated
transitions. No schema/data/external-contract change.

## Risk Register

| Risk | Likelihood | Mitigation | 
|------|------------|-----------|
| Chain-edge drift (first/last hop) | Med | Anchors in slice + round-trip net; assert endpoints in helper unit test |
| Custom inversion mishandled | Med | Single inversion point in slice; custom matrix golden + 4 custom round-trip cases |
| Picker state init lost during collapse | Med | `applyPickerEntry` covers all targets including Kiro/Codex-first custom paths |
| Dropping custom StrictTDD Review fallback | High | Explicit design contract in step 8; RED test for custom-StrictTDD→Review path |
| ModelPicker gate divergence | Low | Slice calls the same `osStatModelCache`; gate covered by Multi+cache round-trip case |
| Slice alloc per keypress | Low | ≤8 elems; acceptable for TUI |
| Back-row regression (latent bugs re-introduced) | Med | 8 explicit RED-first Back-row test cases covering all 4 known inconsistencies |

**Changed-line estimate**: ~270–370 lines (new method ~30 + 2 helpers ~30 +
`applyPickerEntry` ~25 + unit tests ~120 + Back-row tests ~60; collapsing 10
blocks net-removes ~80–120). Fits one PR under the 400-line budget. Delivery:
`ask-on-risk` → no split needed.

## Open Questions

None blocking. `applyPickerEntry` vs inlining picker-state init is an
implementation detail for sdd-tasks; behavior is identical either way.
