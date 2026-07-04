# Archive Report: Declarative Picker Navigation

**Date**: 2026-06-22  
**Change**: declarative-picker-navigation  
**Status**: SHIPPED — merged to main, verify PASS

## What Shipped

The declarative picker navigation refactor is a behavior-preserving internal refactor of the installer TUI's conditional picker chain (`internal/tui/model.go`). It replaced triplicated imperative navigation logic with a single computed flow slice.

### Core Implementation
- **`pickerFlowSlice()` method**: Computes the ordered conditional chain from `m.Selection`, filtered by existing `shouldShow*` predicates. Includes the PresetCustom ordering inversion (DependencyTree position).
- **`pickerNextScreen()`/`pickerPreviousScreen()` helpers**: Scan the slice forward/backward from the current screen.
- **`applyPickerEntry()` helper**: Centralizes picker-state initialization for all target screens, preventing state zeroing when entering Kiro/Codex-first paths.

### Call-Site Rewrites
Collapsed triplicated navigation blocks across:
- Forward transitions in `confirmSelection()` and HandleNav callbacks (Claude/Kiro/Codex)
- "← Back" row transitions (Enter on last option)
- Esc transitions in `goBack()`

Removed all `// NOTE: keep in sync` comments that marked the original hazard points.

### Intentional Bugfixes (side-effects of unification)
The refactor fixed four pre-existing inconsistencies between `goBack` (Esc) and "← Back" row (Enter) paths:

1. **StrictTDD Back row**: corrected skipped Codex → now walks Codex→Kiro→Claude via slice (matching Esc behavior)
2. **Codex Back row in custom mode**: changed from `ScreenPreset` → `ScreenDependencyTree`
3. **DependencyTree Back row**: added missing `shouldShowOpenCodePluginsScreen()` early check (parity with Esc)
4. **Codex custom goBack**: removed fallthrough to `linearRoutes`; now returns `ScreenDependencyTree`

All four are explicitly tested in `TestPickerBackRowRegression`.

### Follow-Up Scope Item (Already Completed)
After the main refactor, the `ScreenPreset` confirm ladder was also collapsed onto `pickerNextScreen` (commit b7baf8e), with the OpenCodePlugins guard preserved after the slice walk.

---

## Final Status

**Verdict**: ✅ **PASS**  
**Build**: ✅ Passed  
**Tests**: ✅ All green  
- `go test ./internal/tui/...` — green
- `go vet ./internal/tui/...` — clean
- All 11 `TestInstallNavigationRoundTrips` cases pass unchanged
- All `TestPickerBackRowRegression` cases (8 total) green, covering the 4 fixed bugs

**Golden Files**: Unchanged  
- `TestPresetSelectionNextScreenFlowMatrix` and `TestCustomPresetPostComponentFlowMatrix` pass without `-update`

**Conformance**: ✅ Design invariants held  
- `pickerFlowSlice` depends only on `m.Selection`, never `m.Screen` (INV-6)
- Custom StrictTDD forward preserves `BuildReviewPayload → ScreenReview` fallback
- Cross-cutting guards (`ModelConfigMode`, `OpenCodePluginsStandalone`, Upgrade Esc-quit) remain early-return guards
- `osStatModelCache` remains the single injectable boundary for ModelPicker gate

**Merged**: ✅ To main (refactor commit chain through b7baf8e)

---

## Capability Specification

This change is captured by:
- **Topic Key**: `openspec/specs/installer-picker-navigation/spec.md`
- **Location**: `/Users/alanbuscaglia/work/gentle-ai/openspec/specs/installer-picker-navigation/spec.md`

The spec establishes the picker flow ordering contract, symmetric reverse navigation requirement, and cross-cutting guard boundaries. All requirements are enforced by the TUI test suite (existing + new regression tests).

---

## Affected Areas

| File | Change |
|------|--------|
| `internal/tui/model.go` | Added `pickerFlowSlice`, `pickerNextScreen`, `pickerPreviousScreen`, `applyPickerEntry`; collapsed 10 call sites |
| `internal/tui/model_test.go` | Added `TestPickerFlowSlice`, `TestPickerNextScreen`, `TestPickerPreviousScreen`, `TestApplyPickerEntry`, `TestPickerBackRowRegression` + custom/forward tests |
| `openspec/specs/installer-picker-navigation/spec.md` | New capability spec (promoted from delta) |

---

## Rollback

The refactor is single-branch cohesive. Reverting the commit chain returns `model.go` to triplicated transitions. No schema, data, or external contract changes.

---

## Notes

- Zero `// NOTE: keep in sync` comments remain in `model.go`
- No view/golden output changes
- No scope creep; related nav graphs (backups, uninstall, agent-builder, profiles, upgrade/sync) untouched
- Strict TDD active throughout; all tests written before implementation
- Changed lines: ~375 gross (fits single PR under ~400-line budget)
