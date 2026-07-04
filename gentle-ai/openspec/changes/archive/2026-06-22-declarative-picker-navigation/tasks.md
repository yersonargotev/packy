# Tasks: Declarative Picker Navigation (Computed Flow Slice)

## Status

`complete`

## Overview

Ordered work units for the declarative picker navigation refactor. Each unit is a reviewable commit containing tests + implementation together. Strict TDD is ACTIVE: every unit begins with RED (failing) tests, then goes GREEN with implementation, then REFACTOR.

All changes are confined to `internal/tui/`. Test runner: `go test ./internal/tui/...`. No view/golden changes. No new Screen constants.

---

## Work Unit 1 — `pickerFlowSlice` method + unit tests

**Goal:** Implement the single source-of-truth ordered picker slice. Tests must fail to compile first (method does not exist), then pass after implementation.

**Files:** `internal/tui/model.go`, `internal/tui/model_test.go`

**Depends on:** nothing (first unit)

### Tasks

- [x] **1.1 RED** — In `model_test.go`, add `TestPickerFlowSlice` table-driven test. Cases must cover:
  - Non-custom, all agents + SDDMode Multi + model cache present → exact `[]Screen` order including `ScreenModelPicker`
  - Non-custom, all agents + SDDMode Single → `ScreenModelPicker` absent
  - Non-custom, all agents + SDDMode Multi + model cache absent → `ScreenModelPicker` absent
  - Non-custom, Claude only → slice contains `ScreenPreset`, `ScreenClaudeModelPicker`, `ScreenDependencyTree`
  - Non-custom, no picker agents (no Claude/Kiro/Codex/OpenCode/SDD) → slice is `[ScreenPreset, ScreenDependencyTree]`
  - Custom, Claude + Kiro + SDDMode Multi + cache → `ScreenDependencyTree` appears at index 1 (before Claude), NOT at the end
  - Custom, no picker agents → `[ScreenPreset, ScreenDependencyTree]` (DependencyTree still at index 1)
  - Anchors always present: first element is always `ScreenPreset`, last non-custom is always `ScreenDependencyTree`
  - Use the existing `withModelCache` pattern (inject `osStatModelCache` override)
  - Compile must fail: `pickerFlowSlice` does not exist yet. (**RED gate**)

- [x] **1.2 GREEN** — Add `func (m Model) pickerFlowSlice() []Screen` to `model.go`.
  - `ScreenPreset` always first anchor
  - If `m.Selection.Preset == model.PresetCustom`: append `ScreenDependencyTree` second
  - Append conditional screens in order: `shouldShowClaudeModelPickerScreen()`, `shouldShowKiroModelPickerScreen()`, `shouldShowCodexModelPickerScreen()`, `shouldShowSDDModeScreen()` (and inside it: SDDMode==Multi AND `osStatModelCache(opencode.DefaultCachePath())==nil` → `ScreenModelPicker`), `shouldShowStrictTDDScreen()`
  - If `!custom`: append `ScreenDependencyTree` last anchor
  - `ScreenOpenCodePlugins` and `ScreenSkillPicker` are intentionally NOT in the slice (they are early-return guards)
  - **Invariant**: no predicate that reads `m.Screen` may be used here
  - Run `TestPickerFlowSlice` → all GREEN

- [x] **1.3 REFACTOR** — Tidy method body (comments, var naming). Run full suite: `go test ./internal/tui/...` must be GREEN. No golden updates.

**Spec coverage:** INV-1, INV-3, INV-6, Scenarios 1, 2, 5, 6, 7

---

## Work Unit 2 — `pickerNextScreen` + `pickerPreviousScreen` + unit tests

**Goal:** Implement the two slice-walker helpers. RED tests written before methods exist.

**Files:** `internal/tui/model.go`, `internal/tui/model_test.go`

**Depends on:** Unit 1 (uses `pickerFlowSlice`)

### Tasks

- [x] **2.1 RED** — Add `TestPickerNextScreen` and `TestPickerPreviousScreen` table-driven tests in `model_test.go`. Cases must cover:
  - **next**: for each consecutive pair in a full-chain slice, assert (target, ok=true)
  - **next**: `m.Screen == ScreenDependencyTree` (non-custom last anchor) → `ok=false`
  - **next**: `m.Screen == ScreenStrictTDD` when custom (DependencyTree absent from right end) → `ok=false`
  - **next**: non-member screens (`ScreenModelConfig`, `ScreenSkillPicker`, `ScreenReview`) → `ok=false`
  - **prev**: for each consecutive pair, assert reverse (target, ok=true)
  - **prev**: `m.Screen == ScreenPreset` (first anchor) → `ok=false`
  - **prev**: same non-member screens → `ok=false`
  - **prev**: custom slice — `ScreenClaudeModelPicker` → prev should return `ScreenDependencyTree`
  - Compile fails: methods do not exist yet. (**RED gate**)

- [x] **2.2 GREEN** — Implement `pickerNextScreen()` and `pickerPreviousScreen()` on `Model` in `model.go`.
  - Both call `m.pickerFlowSlice()` on each invocation (no caching — slice is ≤8 elements)
  - Linear scan for `m.Screen` in slice; `ok=false` when not found
  - `pickerNextScreen`: returns slice[i+1] when found at i < len-1; `ok=false` at last position
  - `pickerPreviousScreen`: returns slice[i-1] when found at i > 0; `ok=false` at position 0
  - Run `TestPickerNextScreen` + `TestPickerPreviousScreen` → all GREEN

- [x] **2.3 REFACTOR** — Review helper bodies. Run `go test ./internal/tui/...` → GREEN.

**Spec coverage:** INV-2, INV-7, Scenarios 3, 4, 5, 6, 9, 10

---

## Work Unit 3 — `applyPickerEntry` helper

**Goal:** Centralize all picker-state initialization into one helper that handles every target screen a caller may navigate to. Prevents state zeroing when entering Kiro/Codex-first paths.

**Files:** `internal/tui/model.go`, `internal/tui/model_test.go`

**Depends on:** Unit 1 (needs `pickerFlowSlice` shape to know which screens to initialize)

### Tasks

- [x] **3.1 RED** — Add `TestApplyPickerEntry` in `model_test.go`. Cases:
  - `applyPickerEntry(ScreenClaudeModelPicker)` → `m.ClaudeModelPicker` is non-zero (initialized via `screens.NewClaudeModelPickerStateFromPhaseAssignments`)
  - `applyPickerEntry(ScreenKiroModelPicker)` → `m.KiroModelPicker` initialized
  - `applyPickerEntry(ScreenCodexModelPicker)` → `m.CodexModelPicker` initialized
  - `applyPickerEntry(ScreenModelPicker)` → `m.ModelPicker` initialized (requires model cache override)
  - `applyPickerEntry(ScreenSDDMode)` → `m.Screen == ScreenSDDMode` (no extra state init needed)
  - `applyPickerEntry(ScreenStrictTDD)` → `m.Screen == ScreenStrictTDD`
  - `applyPickerEntry(ScreenDependencyTree)` → `m.Screen == ScreenDependencyTree`
  - Custom Kiro-only: model starts at `ScreenDependencyTree`, `applyPickerEntry(ScreenKiroModelPicker)` → `KiroModelPicker` is non-zero value, not zero struct
  - Custom Codex-only: same invariant for `CodexModelPicker`
  - Compile fails: method does not exist. (**RED gate**)

- [x] **3.2 GREEN** — Implement `func (m *Model) applyPickerEntry(next Screen)` in `model.go`.
  - Switch on `next`; per target: call the appropriate `screens.New*State*` constructor then `m.setScreen(next)`
  - For `ScreenClaudeModelPicker`: `m.ClaudeModelPicker = screens.NewClaudeModelPickerStateFromPhaseAssignments(...)`
  - For `ScreenKiroModelPicker`: `m.KiroModelPicker = screens.NewKiroModelPickerStateFromAssignments(m.Selection.KiroModelAssignments)`
  - For `ScreenCodexModelPicker`: `m.CodexModelPicker = screens.NewCodexModelPickerStateFromAssignments(m.Selection.CodexModelAssignments)`
  - For `ScreenModelPicker`: `m.ModelPicker = screens.NewModelPickerState(cachePath, ...)`
  - Default: `m.setScreen(next)` only
  - Run `TestApplyPickerEntry` → GREEN

- [x] **3.3 REFACTOR** — Verify all `screens.New*` constructors receive the same arguments currently used at each call site. Run `go test ./internal/tui/...` → GREEN.

**Spec coverage:** INV-1 (state correctness on entry), Design `applyPickerEntry` shape table

---

## Work Unit 4 — RED-first "← Back" row regression tests (Enter on Back option)

**Goal:** Write all 8 failing Back-row test cases BEFORE touching any call sites. These must fail (or be inert, producing no screen change) under the current implementation, proving the bugs exist.

**Files:** `internal/tui/model_test.go` (new test function `TestPickerBackRowRegression`)

**Depends on:** none (read-only addition to test file; Units 1–3 may run in parallel with this unit once 1 is done, but this must run before Unit 5)

### Tasks

- [x] **4.1 RED** — Add `TestPickerBackRowRegression` table-driven test in `model_test.go`. Each case:
  - Sets up a `Model` on the relevant screen with appropriate `Selection`
  - Positions cursor at the "← Back" row index (last option in `GetCurrentOptions()`)
  - Sends `tea.KeyMsg{Type: tea.KeyEnter}`
  - Asserts `state.Screen == wantScreen`
  - Cases required (must FAIL before Unit 5 implementation):
    1. **Codex Back non-custom Codex-only**: `ScreenCodexModelPicker`, only Codex agent selected, no custom → want `ScreenPreset`
    2. **Codex Back non-custom Kiro+Codex**: `ScreenCodexModelPicker`, Kiro+Codex agents → want `ScreenKiroModelPicker`
    3. **Codex Back custom Codex-only** (bug: currently → `ScreenPreset`): `ScreenCodexModelPicker`, custom, Codex-only → want `ScreenDependencyTree` (**RED must fail**)
    4. **StrictTDD Back Codex+no Claude+no Kiro** (bug: skips Codex): `ScreenStrictTDD`, Codex+OpenCode, no Claude/Kiro → want `ScreenCodexModelPicker` (**RED must fail**)
    5. **StrictTDD Back Kiro+no Claude+no Codex** (bug: skips to SDDMode or wrong): `ScreenStrictTDD`, Kiro+OpenCode, no Claude/Codex → want `ScreenKiroModelPicker` (**RED must fail**)
    6. **DependencyTree Back non-custom OpenCode no StrictTDD/SDDMode** (bug: lacks `shouldShowOpenCodePluginsScreen` check): non-custom, OpenCode only (minimal preset) → want `ScreenOpenCodePlugins` (**RED must fail**)
    7. **applyPickerEntry custom Kiro-only DependencyTree Continue**: navigate custom DependencyTree → assert lands on `ScreenKiroModelPicker` with initialized `KiroModelPicker`
    8. **applyPickerEntry custom Codex-only DependencyTree Continue**: navigate custom DependencyTree → assert lands on `ScreenCodexModelPicker` with initialized `CodexModelPicker`
  - Document which cases fail due to: (a) inert Back row, (b) wrong screen, (c) missing predicate check. At least cases 3, 4, 5, 6 must fail. Cases 1, 2 may already pass; document status.

**Note:** These tests are the RED gate for Unit 5. Do not proceed to Unit 5 until all 8 are committed (regardless of which fail vs pass — the failing ones are the regression net).

**Spec coverage:** INV-2, INV-2a, INV-2b, Scenarios 3, 4; Design bugfixes 1–4

---

## Work Unit 5 — Rewrite forward call sites (Claude / Kiro / Codex HandleNav)

**Goal:** Collapse the triplicated forward-navigation ladders in the three model-picker `HandleNav` blocks onto `pickerNextScreen` + `applyPickerEntry`. Keep ModelConfigMode early returns intact.

**Files:** `internal/tui/model.go`

**Depends on:** Units 1, 2, 3 (helpers must exist); `TestInstallNavigationRoundTrips` must be GREEN before starting

### Tasks

- [x] **5.1** — Pre-condition check: run `go test ./internal/tui/...` and confirm all 11 `TestInstallNavigationRoundTrips` cases GREEN. Record baseline.

- [x] **5.2** — Rewrite **Claude HandleNav forward** (`model.go:~1129–1156`):
  - Keep the ModelConfigMode early return block (lines ~1120–1128) unchanged
  - Replace the `else if shouldShowKiro… / Codex… / SDDMode… / custom… / StrictTDD… / else DependencyTree` ladder with:
    ```
    } else if next, ok := m.pickerNextScreen(); ok {
        if next == ScreenDependencyTree { m.buildDependencyPlan() }
        m.applyPickerEntry(next)
    }
    ```
  - Delete the custom-preset StrictTDD/SkillPicker/Review branch in this block (custom forward is now handled by slice; StrictTDD will call these in Unit 6)
  - Run `go test ./internal/tui/...` → GREEN

- [x] **5.3** — Rewrite **Kiro HandleNav forward** (`model.go:~1179–1201`): same pattern as 5.2.
  - Run `go test ./internal/tui/...` → GREEN

- [x] **5.4** — Rewrite **Codex HandleNav forward** (`model.go:~1256–1275`): same pattern.
  - Run `go test ./internal/tui/...` → GREEN

- [x] **5.5** — Rewrite **SDDMode forward target ladder** (`model.go:~1952–1988`):
  - Keep the SDDMode Multi → ModelPicker initialization block
  - Replace the post-assignment target ladder (after mode is set) with `pickerNextScreen` (SDDMode is not the last in the slice in non-custom)
  - Run `go test ./internal/tui/...` → GREEN

- [x] **5.6** — Rewrite **ModelPicker Continue** (`model.go:~2060–2100`) and **ModelPicker Back row** (`~2103–2108`):
  - Continue path: use `pickerNextScreen` for the post-ModelPicker target
  - Back path: use `pickerPreviousScreen` (resolves to SDDMode); keep ModelConfigMode early return
  - Run `go test ./internal/tui/...` → GREEN

**Spec coverage:** INV-1, INV-4, Scenarios 1, 2, 5, 6, 8

---

## Work Unit 6 — Rewrite back call sites (confirmSelection + goBack)

**Goal:** Unify "← Back" row (Enter) and Esc paths onto `pickerPreviousScreen`. Fix all 4 Back-row bugs. Delete all `// NOTE: keep in sync` comments.

**Files:** `internal/tui/model.go`

**Depends on:** Units 1, 2, 3, 4 (regression tests must be RED), Unit 5 (forward paths stable)

### Tasks

- [x] **6.1** — Verify `TestPickerBackRowRegression` cases 3, 4, 5, 6 still FAIL (confirming bugs not yet fixed). This is the pre-condition checkpoint.

- [x] **6.2** — Rewrite **SDDMode Back row** (`confirmSelection`, `model.go:~1993–2015`):
  - Both custom + non-custom branches replaced by:
    ```
    if prev, ok := m.pickerPreviousScreen(); ok { m.applyPickerEntry(prev) }
    ```
  - Delete `// NOTE: keep in sync` comment(s) in this block
  - Run `go test ./internal/tui/...` → GREEN

- [x] **6.3** — Rewrite **StrictTDD Back row** (`confirmSelection`, `model.go:~2134–2152`):
  - Replace `SDDMode/ModelPicker/Claude/custom/Preset` ladder with `pickerPreviousScreen`
  - Run `go test ./internal/tui/...` → GREEN; `TestPickerBackRowRegression` cases 4, 5 now GREEN

- [x] **6.4** — Rewrite **DependencyTree Back row** (`confirmSelection`, `model.go:~2208–2229`):
  - Keep `isPiOnlyAgents` early-return guard
  - Keep `shouldShowOpenCodePluginsScreen` early-return check (this is the fix for bug #3)
  - Then: `if prev, ok := m.pickerPreviousScreen(); ok { m.applyPickerEntry(prev) }`
  - Delete `// NOTE: keep in sync` comment
  - Run `go test ./internal/tui/...` → GREEN; `TestPickerBackRowRegression` case 6 now GREEN

- [x] **6.5** — Rewrite **goBack `ScreenStrictTDD`** (`model.go:~3011–3043`):
  - Use `pickerPreviousScreen`; keep OpenCodePluginsStandalone early-return guard
  - Run `go test ./internal/tui/...` → GREEN

- [x] **6.6** — Rewrite **goBack `ScreenSDDMode`** (`model.go:~3053–3082`):
  - Use `pickerPreviousScreen`
  - Run `go test ./internal/tui/...` → GREEN

- [x] **6.7** — Rewrite **goBack `ScreenClaudeModelPicker` custom block** (`model.go:~3084–3088`) and **`ScreenKiroModelPicker`** (`~3090–3108`) and **`ScreenCodexModelPicker`** (`~3110–3120`):
  - Each: `if prev, ok := m.pickerPreviousScreen(); ok { m.applyPickerEntry(prev) }`
  - The Codex custom back now returns `ScreenDependencyTree` instead of falling through to `linearRoutes` (fixes bug #4)
  - Run `go test ./internal/tui/...` → GREEN; `TestPickerBackRowRegression` cases 1, 2, 3 now GREEN

- [x] **6.8** — Rewrite **`confirmSelection` Back rows for Claude, Kiro, Codex pickers** (`model.go:~1904–1951`):
  - Each block: keep ModelConfigMode early return, then `if prev, ok := m.pickerPreviousScreen(); ok { m.applyPickerEntry(prev) }`
  - Run `go test ./internal/tui/...` → GREEN

- [x] **6.9** — Verify ALL `TestPickerBackRowRegression` cases GREEN. If any remain RED, fix before proceeding.

**Spec coverage:** INV-2, INV-2a, INV-2b, Scenarios 3, 4; Design bugfixes 1–4

---

## Work Unit 7 — StrictTDD forward: preserve custom Review fallback

**Goal:** Ensure the StrictTDD forward path correctly uses `pickerNextScreen` for non-custom (→ DependencyTree) while preserving the exact custom control flow (OpenCodePlugins guard → SkillPicker → Review). This is the HIGH-risk item from the design risk register.

**Files:** `internal/tui/model.go`

**Depends on:** Units 1, 2, 5

### Tasks

- [x] **7.1 RED** — Add test cases to `TestPickerBackRowRegression` (or new `TestStrictTDDForward`) covering:
  - Custom, no OpenCode, no Skills: StrictTDD Enter → `ScreenReview` (not DependencyTree)
  - Custom, no OpenCode, has Skills: StrictTDD Enter → `ScreenSkillPicker`
  - Custom, has OpenCode: StrictTDD Enter → `ScreenOpenCodePlugins` (guard fires before slice)
  - Non-custom, has StrictTDD: StrictTDD Enter → `ScreenDependencyTree` via `pickerNextScreen` + `buildDependencyPlan()`
  - These tests must be written first; some may already pass, others fail if the refactor dropped the Review branch.

- [x] **7.2 GREEN** — Rewrite **StrictTDD forward** (`model.go:~2114–2132`) following design step 8 precisely:
  - (a) `shouldShowOpenCodePluginsScreen()` guard → `ScreenOpenCodePlugins` (early return, NOT a slice member)
  - (b) `m.Selection.Preset == model.PresetCustom` branch: `shouldShowSkillPickerScreen()` → `ScreenSkillPicker`, else `BuildReviewPayload` → `ScreenReview`
  - (c) Else (non-custom): `if next, ok := m.pickerNextScreen(); ok { m.buildDependencyPlan(); m.setScreen(next) }`
  - The Review fallback in (b) must NOT be dropped
  - Run `go test ./internal/tui/...` → GREEN; `TestCustomPresetPostComponentFlowMatrix` unchanged (no golden updates)

**Spec coverage:** INV-3, INV-5, Scenarios 7, 8; Design risk "Dropping custom StrictTDD Review fallback"

---

## Work Unit 8 — Final green pass, vet, and cleanup

**Goal:** Confirm the full test suite is green, `go vet` is clean, no golden files were touched, and dead code was removed.

**Files:** `internal/tui/model.go`, `internal/tui/model_test.go`

**Depends on:** All previous units

### Tasks

- [x] **8.1** — Run `go test ./internal/tui/...` — must be 100% GREEN. Zero failures.

- [x] **8.2** — Run `go vet ./internal/tui/...` — must be clean.

- [x] **8.3** — Confirm no golden files were modified:
  - `git diff --name-only internal/tui/testdata/` must be empty.

- [x] **8.4** — Verify `TestInstallNavigationRoundTrips` all 11 cases pass individually: `go test -run TestInstallNavigationRoundTrips ./internal/tui/...`

- [x] **8.5** — Verify `TestPickerBackRowRegression` all 8 cases GREEN.

- [x] **8.6** — Scan for leftover `// NOTE: keep in sync` comments in `model.go`; there must be none.

- [x] **8.7** — Scan for leftover triplicated picker ladders: confirm no `shouldShowKiro` / `shouldShowCodex` / `shouldShowSDD` chains remain in the HandleNav forward blocks.

- [x] **8.8** — Run `go build ./...` from repo root; must succeed.

---

## Execution Order and Parallelism

```
Unit 1 (pickerFlowSlice)
  ├── Unit 2 (next/prev helpers)    [sequential after 1]
  ├── Unit 3 (applyPickerEntry)     [sequential after 1; can run parallel with 2]
  └── Unit 4 (RED Back-row tests)   [can start after 1; parallel with 2 and 3]
         └── Unit 5 (forward call sites)  [requires 1, 2, 3]
               └── Unit 6 (back call sites)   [requires 1, 2, 3, 4, 5]
                     └── Unit 7 (StrictTDD forward)  [requires 1, 2, 5]
                           └── Unit 8 (green pass)   [requires all]
```

Units 2, 3, and 4 can be worked in parallel once Unit 1 is complete. Units 5, 6, 7, and 8 are strictly sequential.

---

## Review Workload Forecast

| Metric | Estimate |
|--------|----------|
| New helpers (`pickerFlowSlice`, `pickerNextScreen`, `pickerPreviousScreen`, `applyPickerEntry`) | ~85 lines |
| Unit tests (Units 1–3 + 4 RED tests) | ~150 lines |
| StrictTDD forward tests (Unit 7) | ~30 lines |
| Call-site collapses (10 blocks across Units 5, 6, 7) — net after deletions | ~(+110 new, -120 removed) = net −10 lines |
| **Total changed lines (added + removed, gross)** | **~375 lines** |
| Fits ~400-line single-PR budget | **Yes (borderline)** |
| `Chained PRs recommended` | **No** |
| `400-line budget risk` | **Med** (within budget, but within 10% of limit; a second review pass could push it over if comments/cleanup expand) |
| `Decision needed before apply` | **No** — proceed single PR; delivery strategy `ask-on-risk` clears at Med risk if the implementer monitors gross line count as Units 5–6 land |

**Guidance:** The design's own estimate (270–370 lines) is slightly optimistic vs this task breakdown (375 gross). If Unit 5 or 6 requires more scaffolding than expected during apply, the implementer should flag before proceeding past Unit 6 to avoid exceeding 400 lines.
