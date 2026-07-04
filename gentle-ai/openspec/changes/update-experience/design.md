# Design: Update Experience Overhaul

## Technical Approach

Layer the change onto existing modules without new packages. State store (`internal/state`) gains two additive fields backing cooldown + deferred sync. The update check (`internal/update`) gains a TTL gate and an async advisory fetch. The TUI (`internal/tui/model.go`) gains one pre-Welcome screen state. The CLI self-update (`internal/app/selfupdate.go`) makes the existing TTY-aware prompt the default and converges on close-and-reopen. The upgrade executor (`internal/update/upgrade`, `internal/components/engram/download.go`) starts honoring `GENTLE_AI_CHANNEL`. All artifacts are informational-only — no version gate anywhere.

## Architecture Decisions

### Decision: TUI pre-Welcome prompt as a new Screen state

**Choice**: Add `ScreenUpdatePrompt`. `Init` (model.go:602) keeps firing `update.CheckAll`; when `UpdateCheckResultMsg` arrives with `HasUpdates`, set `Screen = ScreenUpdatePrompt` BEFORE Welcome. Render lists current→latest, three key actions: `u`=update (run `UpgradeFn` for gentle-ai then `tea.Quit`), `c`/Enter=keep (transition to `ScreenWelcome`), `v`=view changes (open `UpdateResults[i].ReleaseURL`, types.go:68, via an `openURLFn` command). If no update, go straight to Welcome (current behavior, model.go:851). The banner at model.go:852 stays as a fallback for the keep path.

**Alternatives considered**: Overlay/modal on Welcome; a bubbletea sub-model.
**Rationale**: The codebase models every screen as a `Screen` enum with a `View()` switch (model.go:848). A new enum value is the idiomatic, lowest-risk fit. Bubbletea note: the screen renders only after the async check resolves, so a brief Welcome-less spinner frame may show; reuse the existing `UpdateCheckDone` guard to render a "checking…" state until results land.

### Decision: Converge both OSes on close-and-reopen

**Choice**: After a successful gentle-ai self-upgrade, ALWAYS print "Updated to vX — restart gentle-ai to use the new version." and return (no re-exec) on BOTH Unix and Windows. Drop the Unix `reExec` branch in `restartAfterGentleAIUpgrade` (selfupdate.go:153-184). TUI "update" path runs the upgrade then `tea.Quit`.

**Alternatives considered**: Keep Unix re-exec (status quo); OS-conditional behavior.
**Rationale**: Locked decision. Re-exec is invisible on Unix but impossible on Windows (binary lock) — divergent UX and divergent test surface. One close-and-reopen path is consistent, sidesteps the Windows lock, and the copy makes the exit non-surprising. Tradeoff: Unix users lose seamless restart; mitigated by one clear line of copy.

### Decision: CLI prompt is default, gated by TTY not env

**Choice**: Remove the `envConfirmUpdate` gate (selfupdate.go:29, 110-116). Always call `promptFn` before applying. `defaultPromptForUpdate` already declines when stdin is not a TTY (selfupdate.go:40-41) — that becomes the CI/non-interactive fallback (no update applied, no prompt). Add a `--yes` flag and honor existing `GENTLE_AI_NO_SELF_UPDATE=1` opt-out; keep `GENTLE_AI_SELF_UPDATE_DONE` loop guard.

**Alternatives considered**: Keep env gate but default it on; prompt always regardless of TTY.
**Rationale**: TTY detection already exists and is the correct non-interactive signal. Prompting a pipe would hang CI. `--yes` covers scripted opt-in; env opt-out preserves existing escape hatch.

### Decision: Update-check cooldown = 6h TTL in state.json

**Choice**: Add `LastUpdateCheck time.Time` (`json:"last_update_check,omitempty"`) to `InstallState` (state.go:31). Before `CheckAll` in TUI `Init` (model.go:609) and CLI `selfUpdate` (selfupdate.go:94), skip the network check if `now - LastUpdateCheck < 6h`. Write `LastUpdateCheck = now` ONLY on a successful check (not on error/rate-limit), so failures don't poison the window. Cache the last successful `[]UpdateResult` is out of scope; on cooldown skip we simply suppress the check and show no banner that launch.

**Alternatives considered**: 1h (too chatty), 24h (misses same-day engram bumps), caching results too.
**Rationale**: 6h balances freshness against GitHub rate limits for users who launch many times per day. `omitempty` + zero-value `time.Time` means old state files (no field) always check — safe back-compat. Refresh-on-success-only is the explicit anti-poison rule from the proposal.

### Decision: pending_sync flag drives deferred sync after self-upgrade

**Choice**: Add `PendingSync bool` (`json:"pending_sync,omitempty"`). Set it true in the self-upgrade success path (selfupdate.go ~141, before printing restart copy) and in TUI "Upgrade + Sync" when a gentle-ai self-upgrade occurred. On next launch, early in `app.go` Run (after `state.Read`, app.go:127), if `PendingSync`, the NEW binary runs `cli.RunSync` then writes the flag false. Failure: log a warning, leave the flag set so it retries next launch (idempotent sync).

**Alternatives considered**: Re-exec then sync inline (status quo gap); a separate sentinel file.
**Rationale**: state.json is already read at launch (app.go:127) — one extra bool, no new I/O surface. Leaving the flag on failure makes recovery automatic. Sync is idempotent (sync.go re-sync is a no-op when current), so retry is safe.

### Decision: Advisory manifest = single JSON release asset, async, fail-open

**Choice**: Host `advisory.json` as a release asset on the gentle-ai repo's `latest` release (stable, owner-controlled, CDN-backed, no extra infra): `https://github.com/Gentleman-Programming/gentle-ai/releases/latest/download/advisory.json`. Schema: `{"message": string, "severity": "info"|"warn", "url": string}` — all optional, informational only. Fetch with a 2s timeout in a background goroutine kicked off alongside `CheckAll`; on any error, return empty (fail-open, no blocking). Display the message after the update prompt / on Welcome; never gate.

**Alternatives considered**: GitHub Pages (extra setup, another moving part), raw repo file (tied to a branch ref, no CDN), gist (low discoverability, easy to lose).
**Rationale**: A release asset is the most stable owner-controlled option for a solo maintainer — same trust boundary as releases, CDN latency, editable by re-uploading the asset without a code change. 2s timeout + fail-open guarantees zero added launch latency on a slow/absent endpoint.

### Decision: Channel-honoring engram upgrade

**Choice**: Thread channel into the engram download. `cli.ResolveInstallChannel` already exists (channel.go:18). Add a `version`/`ref` param to `engram.DownloadLatestBinary` (download.go:49): stable → `versions.EngramCore` pin (current behavior, download.go:54); beta → `@main` ref. The executor (`strategy.go:399`, `engramBinaryUpgrade` 491-503) resolves channel via `ResolveInstallChannel("")` and passes it through. This fills the gap the comment at download.go:52-54 admits but never implements.

**Alternatives considered**: Read env directly inside download.go (bypasses the existing resolver), separate beta downloader.
**Rationale**: `ResolveInstallChannel` is the single source of truth for channel; reuse it rather than re-parsing the env. The drop point is precisely `engramBinaryUpgrade`, which today ignores channel entirely.

### Decision: 7-slice order confirmed, TUI prompt stands alone

**Choice**: Keep proposal order: (2) cooldown → (3) channel → (4) pending_sync → (5) CLI prompt → (6) TUI prompt → (7) manifest. Slices 2-5,7 each fit under 400 lines. Slice 6 (TUI pre-Welcome screen + render + key handling + tests) is the one likely to exceed 400 lines → its own PR, no other slice bundled.

**Rationale**: Cooldown adds the state field both later slices reuse, so it goes first. Channel and pending_sync are small, independent executor/state changes. TUI carries the most surface (new screen, view, input, tests) and is isolated to keep reviewer load bounded.

## Data Flow

    launch ──→ state.Read ──→ PendingSync? ──→ RunSync ──→ clear flag
                  │
                  └─ now - LastUpdateCheck < 6h? ── yes ──→ skip check
                                 │ no
                                 ▼
                     CheckAll ─┬─→ UpdateResult{ReleaseURL} ─→ prompt (TUI screen / CLI [Y/n])
                               └─→ advisory.json (2s, fail-open) ─→ message
                     on success: LastUpdateCheck = now
                     update chosen ─→ upgradeExecute(channel) ─→ set PendingSync ─→ close

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/state/state.go` | Modify | Add `LastUpdateCheck time.Time`, `PendingSync bool` (omitempty); carry both in `MergeAgents` |
| `internal/update/check.go` | Modify | TTL gate helper; refresh-on-success timestamp write |
| `internal/update/advisory.go` | Create | `FetchAdvisory(ctx)` — 2s timeout, fail-open, `Advisory{Message,Severity,URL}` |
| `internal/tui/model.go` | Modify | `ScreenUpdatePrompt` enum, View case, key handling, gate `CheckAll` on cooldown |
| `internal/tui/screens/update_prompt.go` | Create | Render pre-Welcome prompt (update / view-changes / keep) |
| `internal/app/selfupdate.go` | Modify | Drop env gate; prompt default; `--yes`; converge restart copy (no re-exec); set `PendingSync` |
| `internal/app/app.go` | Modify | Deferred-sync check after `state.Read`; advisory display |
| `internal/update/upgrade/strategy.go` | Modify | Resolve + pass channel into `engramBinaryUpgrade` |
| `internal/components/engram/download.go` | Modify | Accept channel/ref; beta → `@main`, stable → pin |

## Interfaces / Contracts

```go
// state.InstallState additions (back-compatible, omitempty)
LastUpdateCheck time.Time `json:"last_update_check,omitempty"`
PendingSync     bool      `json:"pending_sync,omitempty"`

// advisory.json (release asset) — all fields optional, informational only
type Advisory struct {
    Message  string `json:"message"`
    Severity string `json:"severity"` // "info" | "warn"
    URL      string `json:"url"`
}
func FetchAdvisory(ctx context.Context) (Advisory, bool) // false = fail-open

// engram download gains channel awareness
func DownloadLatestBinary(profile system.PlatformProfile, channel cli.InstallChannel) (string, error)
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Cooldown TTL gate; refresh-on-success-only | Inject clock + state; assert no check inside window, timestamp untouched on error |
| Unit | Prompt default + TTY decline + `--yes`; channel routing (beta→@main) | Swap `promptFn`/stdin; table-test `ResolveInstallChannel` → ref |
| Unit | pending_sync set/clear/retry-on-failure; advisory fail-open on timeout/4xx | Fake state writer; httptest server returning slow/500 |
| Integration | TUI `ScreenUpdatePrompt` transitions (update/keep/view) | bubbletea model test (model_test.go pattern) |

## Migration / Rollout

All state fields are additive with `omitempty`; old binaries ignore unknown JSON, new binaries treat missing fields as zero (always-check / no-pending-sync). No data migration. Manifest is fail-open: not publishing `advisory.json` yields no behavior change. Each slice ships as an independent, revertible PR per the proposal rollback plan.

## Open Questions

- [ ] Confirm `advisory.json` lives on gentle-ai `latest` release vs. a dedicated `advisory` tag (latency identical; tag avoids re-upload per release).
- [ ] TUI "view changes": open in browser (`openURLFn`) vs. print URL and stay on prompt — confirm with maintainer UX preference.
