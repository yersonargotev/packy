# Tasks: Update Experience Overhaul (Slices 2–7)

> Slice 1 (un-pin engram) is DONE. This file covers slices 2–7.
> STRICT TDD IS ACTIVE: each slice follows RED → GREEN → REFACTOR order.
> Test runner: `go test ./...`

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1 000–1 400 total (slices 2–7) |
| 400-line budget risk | High (total); per-slice varies — see per-slice notes |
| Chained PRs recommended | Yes |
| Suggested split | PR 2 → PR 3 → PR 4 → PR 5 → PR 6 → PR 7 (each slice = one PR) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending (ask before apply) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Slice | Goal | Likely PR | Est. lines | Budget risk |
|------|-------|------|-----------|-----------|-------------|
| 2 | 2 | Update-check cooldown (6h TTL, state.json) | PR 2 | ~120 | Low |
| 3 | 3 | Channel-honoring upgrade + split engram downloader | PR 3 | ~150 | Low |
| 4 | 4 | Upgrade+sync deferred via pending_sync flag | PR 4 | ~130 | Low |
| 5 | 5 | CLI prompt default + apply-then-close convergence | PR 5 | ~160 | Low |
| 6 | 6 | TUI pre-Welcome update prompt screen | PR 6 | ~350–450 | **High** — stands alone, own PR mandatory |
| 7 | 7 | Advisory manifest (informational, advisory tag) | PR 7 | ~130 | Low |

---

## Slice 2 — Update-Check Cooldown

**Spec**: update-check-cache | **Files**: `internal/state/state.go`, `internal/update/check.go`, `internal/app/selfupdate.go`, `internal/tui/model.go`

### Phase 1 — Red (failing tests)
- [ ] 2.1 `internal/state/state_test.go` — add test: `InstallState` round-trips `LastUpdateCheck` via `Write`/`Read`; field absent in old JSON is deserialized as zero value (back-compat scenario from spec)
- [ ] 2.2 `internal/update/check_test.go` — add test: `CheckAllWithCooldown` skips GitHub call when `now − LastUpdateCheck < 6h`; makes call and updates timestamp when elapsed ≥ 6h; does NOT update timestamp on fetch error

### Phase 2 — Green (implementation)
- [ ] 2.3 `internal/state/state.go` — add `LastUpdateCheck time.Time \`json:"last_update_check,omitempty"\`` to `InstallState`; carry field through `MergeAgents` return struct
- [ ] 2.4 `internal/update/check.go` — add `CheckAllWithCooldown(ctx, version, profile, homeDir, ttl)` that reads state, compares elapsed time, calls `CheckAll` if stale, writes `LastUpdateCheck` on success only
- [ ] 2.5 `internal/app/selfupdate.go` — replace `updateCheckFiltered` call at line ~94 with cooldown-aware variant; pass `homeDir` and `6*time.Hour` TTL
- [ ] 2.6 `internal/tui/model.go` `Init()` — wrap `update.CheckAll` with cooldown gate using home dir from `os.UserHomeDir()`; accept zero-value (missing field) as always-check (back-compat)

### Phase 3 — Refactor
- [ ] 2.7 Extract clock injection (`nowFn func() time.Time`) into `CheckAllWithCooldown` for test determinism; update tests (2.2) to use injected clock

---

## Slice 3 — Channel-Honoring Upgrade

**Spec**: upgrade-channel, self-update | **Files**: `internal/update/upgrade/strategy.go`, `internal/components/engram/download.go`, `internal/cli/channel.go`

### Phase 1 — Red (failing tests)
- [ ] 3.1 `internal/update/upgrade/strategy_test.go` — add test: `engramBinaryUpgrade` called with `ChannelBeta` passes `@main` ref to `engramDownloadFn`; called with `ChannelStable` passes pinned version ref
- [ ] 3.2 `internal/components/engram/download_test.go` (new or existing) — add test: `DownloadLatestBinary(profile, ChannelBeta)` uses `@main`; `DownloadLatestBinary(profile, ChannelStable)` uses `versions.EngramCore`

### Phase 2 — Green (implementation)
- [ ] 3.3 `internal/components/engram/download.go` — add `channel cli.InstallChannel` param to `DownloadLatestBinary`; when `channel.IsBeta()` use `@main`, else `versions.EngramCore` (fills admitted gap at line ~52–54)
- [ ] 3.4 `internal/update/upgrade/strategy.go` — update `engramDownloadFn` signature to accept channel; `engramBinaryUpgrade` reads `GENTLE_AI_CHANNEL` via `cli.ResolveInstallChannel` and passes channel to `engramDownloadFn`
- [ ] 3.5 `internal/update/upgrade/strategy.go` — update `engramDownloadFn` package-level var declaration to match new signature; update all callers in test stubs

### Phase 3 — Refactor
- [ ] 3.6 Verify `cli.ResolveInstallChannel` handles empty-string and unknown values per spec (fall back to stable, optionally warn); add channel_test.go case for unknown value if not already covered

---

## Slice 4 — Upgrade+Sync Deferred via `pending_sync`

**Spec**: upgrade-sync | **Files**: `internal/state/state.go`, `internal/app/selfupdate.go`, `internal/app/app.go`, `internal/tui/screens/upgrade_sync.go`

### Phase 1 — Red (failing tests)
- [ ] 4.1 `internal/state/state_test.go` — add test: `PendingSync` bool round-trips via `Write`/`Read`; absent field reads as `false` (back-compat)
- [ ] 4.2 `internal/app/selfupdate_test.go` (new or existing) — add test: after successful `gentle-ai` self-upgrade, `PendingSync = true` is written to state before process exit
- [ ] 4.3 `internal/app/app_test.go` (new or existing) — add test: on startup with `state.PendingSync = true`, `RunSync` is called and `PendingSync` is cleared on success; on failure, `PendingSync` remains true

### Phase 2 — Green (implementation)
- [ ] 4.4 `internal/state/state.go` — add `PendingSync bool \`json:"pending_sync,omitempty"\`` to `InstallState`; carry field through `MergeAgents`
- [ ] 4.5 `internal/app/selfupdate.go` — after `upgradeExecute` confirms `gentle-ai` succeeded, call `state.Read` + set `PendingSync = true` + `state.Write` before calling `restartAfterGentleAIUpgrade`
- [ ] 4.6 `internal/app/selfupdate.go` `restartAfterGentleAIUpgrade` — converge Unix + Windows: drop `goOS() == "windows"` branch; always print "Updated to vX — restart gentle-ai…" and return (no re-exec); remove `reExec` var and `syscall` import if unused
- [ ] 4.7 `internal/app/app.go` — after `state.Read` at line ~127, check `installedState.PendingSync`; if true, call `cli.RunSync`; on success write state with `PendingSync = false`; on failure log error and leave flag set
- [ ] 4.8 `internal/tui/screens/upgrade_sync.go` — set `PendingSync = true` in state when Upgrade+Sync detects a self-upgrade event (parallel to CLI path)

### Phase 3 — Refactor
- [ ] 4.9 Ensure `RunSync` is idempotent (no state left) before merge; add a guard comment citing spec scenario "deferred sync fails → retry"

---

## Slice 5 — CLI Prompt Default + Apply-Then-Close

**Spec**: self-update | **Files**: `internal/app/selfupdate.go`, `internal/app/selfupdate_test.go`

### Phase 1 — Red (failing tests)
- [ ] 5.1 `internal/app/selfupdate_test.go` — add test: `selfUpdate` calls `promptFn` unconditionally when update is available (without `GENTLE_AI_CONFIRM_UPDATE`); test `GENTLE_AI_CONFIRM_UPDATE` env set to "1" does NOT gate the prompt (env is ignored)
- [ ] 5.2 `internal/app/selfupdate_test.go` — add test: non-TTY stdin causes `defaultPromptForUpdate` to return `(false, nil)` (auto-decline in CI)
- [ ] 5.3 `internal/app/selfupdate_test.go` — add test: `--yes` flag (via injected promptFn) bypasses interactive prompt and returns `(true, nil)`

### Phase 2 — Green (implementation)
- [ ] 5.4 `internal/app/selfupdate.go` — remove `envConfirmUpdate` constant and the guard block at lines ~111–116; always call `promptFn` when update is available
- [ ] 5.5 `internal/app/selfupdate.go` — add `--yes` flag handling: accept `yesFlag bool` param or read `GENTLE_AI_YES=1`; when set, substitute `promptFn` with a stub that returns `(true, nil)`
- [ ] 5.6 `internal/app/selfupdate.go` `defaultPromptForUpdate` — change prompt text to "[Y/n]" (default Y); update answer parse: empty string or "y"/"yes" → true; "n"/"no" → false

### Phase 3 — Refactor
- [ ] 5.7 Remove `envConfirmUpdate` from constant block entirely; update any test that set it; add a comment "GENTLE_AI_CONFIRM_UPDATE removed — prompt is now unconditional"

---

## Slice 6 — TUI Pre-Welcome Update Prompt Screen

**Spec**: update-prompt | **Files**: `internal/tui/model.go`, `internal/tui/screens/update_prompt.go` (new), `internal/tui/model_test.go`

> **Flagged: this slice stands alone. Estimated ~350–450 lines. Must be its own PR. Do not bundle with any other slice.**

### Phase 1 — Red (failing tests)
- [ ] 6.1 `internal/tui/model_test.go` — add test: when `UpdateCheckResultMsg` arrives with `HasUpdates = true`, model transitions to `ScreenUpdatePrompt` (not `ScreenWelcome`)
- [ ] 6.2 `internal/tui/model_test.go` — add test: `ScreenUpdatePrompt` key "u" triggers upgrade and then `tea.Quit`
- [ ] 6.3 `internal/tui/model_test.go` — add test: `ScreenUpdatePrompt` key "c" or Enter transitions to `ScreenWelcome` (keep current)
- [ ] 6.4 `internal/tui/model_test.go` — add test: `ScreenUpdatePrompt` key "v" calls open-browser / prints URL; prompt remains visible
- [ ] 6.5 `internal/tui/model_test.go` — add test: when no update available, `UpdateCheckResultMsg` transitions to `ScreenWelcome` (no prompt screen)
- [ ] 6.6 `internal/tui/model_test.go` — add test: cooldown gate in `Init()` (from slice 2) causes `UpdateCheckDone = true` immediately with cached results when elapsed < 6h; `ScreenUpdatePrompt` NOT shown when no update in cache

### Phase 2 — Green (implementation)
- [ ] 6.7 `internal/tui/model.go` — add `ScreenUpdatePrompt Screen` constant after `ScreenUnknown` block (preserve iota order; append after last screen)
- [ ] 6.8 `internal/tui/model.go` `Update()` — in `UpdateCheckResultMsg` handler: if `HasUpdates`, set `m.Screen = ScreenUpdatePrompt`; else remain at `ScreenWelcome`; show spinner until `UpdateCheckDone`
- [ ] 6.9 `internal/tui/model.go` `Update()` — add `ScreenUpdatePrompt` key handler: "u" → run `UpgradeFn` then `tea.Quit`; "c"/Enter → `setScreen(ScreenWelcome)`; "v" → `openBrowser(UpdateResults[0].ReleaseURL)` or print URL if browser unavailable, stay on screen
- [ ] 6.10 `internal/tui/screens/update_prompt.go` (create) — implement `RenderUpdatePrompt(results []update.UpdateResult, spinnerFrame int) string`; shows available version, three options (Update / View changes / Keep current version), spinner while checking
- [ ] 6.11 `internal/tui/model.go` `View()` — add `case ScreenUpdatePrompt: return screens.RenderUpdatePrompt(m.UpdateResults, m.SpinnerFrame)`
- [ ] 6.12 `internal/tui/model.go` `optionCount()` — add `case ScreenUpdatePrompt: return 3`
- [ ] 6.13 `internal/app/app.go` — wire `openBrowser` helper (open URL or fallback to `fmt.Fprintln`) into model for "View changes" action

### Phase 3 — Refactor
- [ ] 6.14 Verify `ScreenUpdatePrompt` is listed in `confirmSelection()` switch and `withResetOperationState()` if it holds state; confirm `TickMsg` keeps spinner running while `!UpdateCheckDone` on this screen
- [ ] 6.15 Add `case ScreenUpdatePrompt` to any exhaustive switch that lints missing cases (`optionCount`, `confirmSelection`, `withEscKey`)

---

## Slice 7 — Advisory Manifest

**Spec**: advisory-manifest | **Files**: `internal/update/advisory.go` (new), `internal/update/advisory_test.go` (new), `internal/app/app.go`

### Phase 1 — Red (failing tests)
- [ ] 7.1 `internal/update/advisory_test.go` (create) — add test: `FetchAdvisory` with `httptest` server returning valid JSON `{message: "hi", severity: "info"}` returns `Advisory{Message:"hi"}, true`
- [ ] 7.2 `internal/update/advisory_test.go` — add test: `FetchAdvisory` with slow server (> 2s) returns `Advisory{}, false` (timeout, fail-open)
- [ ] 7.3 `internal/update/advisory_test.go` — add test: `FetchAdvisory` with HTTP 500 returns `Advisory{}, false`
- [ ] 7.4 `internal/update/advisory_test.go` — add test: `FetchAdvisory` with malformed JSON returns `Advisory{}, false`
- [ ] 7.5 `internal/update/advisory_test.go` — add test: `FetchAdvisory` with empty `message` field returns `Advisory{}, false` (nothing to display)

### Phase 2 — Green (implementation)
- [ ] 7.6 `internal/update/advisory.go` (create) — define `Advisory{Message, Severity, URL string}`; implement `FetchAdvisory(ctx context.Context) (Advisory, bool)` with 2s timeout, GET to advisory tag asset URL (`https://github.com/Gentleman-Programming/gentle-ai/releases/download/advisory/advisory.json`), JSON decode, fail-open on any error
- [ ] 7.7 `internal/app/app.go` — launch `update.FetchAdvisory` in background goroutine alongside `update.CheckAll` at TUI init; collect result; display non-empty `Advisory.Message` as informational text on Welcome screen or after prompt (never gate launch)

### Phase 3 — Refactor
- [ ] 7.8 Inject advisory fetch URL as package-level var in `advisory.go` for test override; confirm `advisoryHTTPClient` timeout is exactly 2s and not shared with other clients

---

## Cross-Slice Notes

- `state.MergeAgents` must be updated in slices 2 and 4 (both add fields); ensure no merge-conflict between those PRs by keeping slice 2 merged before slice 4 opens.
- `internal/app/selfupdate.go` is touched by slices 4 and 5; ensure slice 4 is merged first (pending_sync write) before slice 5 (prompt default removal) to avoid rebase conflicts.
- Slice 6 depends on slice 2 (cooldown gate in `Init()`); merge slice 2 before opening slice 6.
- Slices 3 and 7 are independent of all others and can be opened in any order after slice 2.
