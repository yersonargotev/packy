# Proposal: Update Experience Overhaul

## Intent

The gentle-ai update/upgrade/sync flow is unreliable and passive: engram bumps force a gentle-ai release, TUI users get only a silent banner, "Upgrade + Sync" doesn't fully sync, the upgrade executor ignores the active channel, and every launch hits the GitHub API with no cooldown. This change makes updates reach users without a gentle-ai release, prompts users at launch (Codex-style), and fixes the three reliability gaps — while staying informational-only (no forced-update gate).

## Scope

### In Scope
1. **Engram always-latest** (slice 1, DONE — branch `fix/engram-always-latest`, commit `36aee12`): un-pin engram-core + gentle-engram; runtime fetch filters tags to `^v[0-9]+\.[0-9]+\.[0-9]+$`; unifies download with update-check source of truth; prerelease/rc tags stay invisible until a clean `vX.Y.Z` is cut.
2. **Codex-style startup prompt**: when an update is available at launch, prompt every launch (no snooze/skip state). "Update" applies then CLOSES the app (user reopens; sidesteps Windows binary lock). Prompt shows "view changes" (release notes link) and "keep current version". Applies to TUI (new pre-Welcome screen) AND CLI (make the existing `[y/N]` the default, drop the `GENTLE_AI_CONFIRM_UPDATE` env gate).
3. **Remote advisory manifest (informational only)**: small JSON fetched at launch carrying an optional message ("important update, run X"). No version gate, no forced update. Only lever to reach already-deployed clients for FUTURE breaking changes; cannot help clients that predate it.
4. **Reliability fixes**: (a) `gentle-ai upgrade` honors `GENTLE_AI_CHANNEL` (beta → `@main`, stable → latest release); (b) "Upgrade + Sync" completes after a self-upgrade (flag so the NEW binary auto-runs sync on next launch); (c) update-check cooldown caches the check so launches don't hammer the GitHub API (today it fails silently on rate-limit).

### Out of Scope (Non-goals)
- Minimum-supported-version / forced-update gate (manifest is info-only).
- Rewriting the install pipeline or sync engine wholesale.
- Snooze / skip-this-version state (cadence is ask-every-launch by decision).

## Capabilities

### New Capabilities
- `update-prompt`: launch-time update prompt (TUI pre-Welcome screen + CLI default prompt) with update / view-changes / keep-current actions, ask-every-launch cadence, apply-then-close behavior.
- `advisory-manifest`: launch fetch of a remote informational JSON message; no version gating.
- `update-check-cache`: cooldown cache for the update check persisted in state.

### Modified Capabilities
- `self-update`: CLI prompt becomes default (env gate removed); upgrade executor honors `GENTLE_AI_CHANNEL`.
- `upgrade-sync`: sync completes across a self-upgrade via a deferred-sync flag.
- `version-resolution`: engram download filters to stable `vX.Y.Z` tags (slice 1, done).

## Approach

- **State store**: add fields to `InstallState` (`~/.gentle-ai/state.json`): `last_update_check`, `pending_sync` (deferred-sync flag). No skip/snooze fields by decision.
- **Cooldown**: gate `update.CheckAll` (TUI `Init`) and CLI check behind a TTL read from `last_update_check`; refresh on success only, so rate-limit failures don't poison the cache.
- **Prompt**: TUI gains a pre-Welcome model state that renders when an update is available; CLI `selfUpdate` always prompts. "Update" applies + exits (Unix and Windows converge on close-and-reopen).
- **Channel**: upgrade executor reads `GENTLE_AI_CHANNEL` and routes beta to `@main`, stable to latest release.
- **Deferred sync**: on self-upgrade, set `pending_sync`; new binary runs sync on next launch then clears the flag.
- **Manifest**: fetch a small JSON at launch (short timeout, fail-open), display its optional message; never block or gate on it.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/state/state.go` | Modified | Add `last_update_check`, `pending_sync` fields |
| `internal/update/check.go`, `github.go` | Modified | Cooldown TTL; manifest fetch |
| `internal/tui/model.go` | Modified | Pre-Welcome prompt state; gate `CheckAll` on cooldown |
| `internal/app/selfupdate.go` | Modified | Default prompt; remove env gate; apply-then-close |
| `internal/cli/upgrade_sync.go` | Modified | Complete sync across self-upgrade via `pending_sync` |
| upgrade executor (`strategy.go` area) | Modified | Honor `GENTLE_AI_CHANNEL` |
| `internal/app/version.go`, `versions.go` | Done | Engram un-pin + stable-tag filter (slice 1) |

## Proposed Slicing (reviewable units, ≤400 lines each where possible)

1. **Engram un-pin** — DONE (branch `fix/engram-always-latest`).
2. **Update-check cooldown** — state fields + TTL gate; smallest, unblocks the rest.
3. **Channel fix** — upgrade executor honors `GENTLE_AI_CHANNEL`.
4. **Upgrade + Sync fix** — `pending_sync` flag + deferred sync on next launch.
5. **CLI prompt default** — drop env gate, make prompt default, apply-then-close.
6. **TUI startup prompt** — pre-Welcome screen (largest UI slice; may need its own PR).
7. **Advisory manifest** — fetch + display informational message.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Network dependency on upgrade (fetch latest at runtime) | Med | Short timeout, fail-open to current binary; install script is the escape hatch |
| No buffer for a bad stable release (always-latest) | Med | Staging lever: cut prerelease/rc tags (invisible) until a clean `vX.Y.Z` is ready |
| Prompt fatigue from ask-every-launch | Med | Predictable cadence; "keep current" is one keystroke; revisit snooze later if needed |
| Manifest can't reach pre-manifest clients | High (inherent) | Acknowledged; one-time out-of-band advertisement covers the transition |
| Apply-then-close surprises users mid-session | Low | Clear prompt copy; matches today's Windows behavior |

## Rollback Plan

Each slice is an independent PR. Revert per slice. Slice 1 (engram un-pin) rolls back by restoring the pin in `versions.go`. Cooldown/state-field additions are additive and backward-compatible (older binaries ignore unknown JSON fields). The manifest is fail-open, so disabling its endpoint reverts behavior with no client change.

## Dependencies

- A hosted remote endpoint for the advisory manifest JSON (slice 7).
- One transition gentle-ai release shipping this work (bootstrap, below).

## Bootstrap / Migration Plan

New behavior only exists in clients that already updated (chicken-and-egg). CLI users mostly auto-migrate — `selfUpdate` of the gentle-ai binary works and is unaffected by the engram pin. TUI-only users see only a passive banner today and must act once manually. The version-independent escape hatch is the install script (`irm/curl … | iex/sh`), which always pulls latest regardless of installed logic. Plan: cut ONE transition gentle-ai release with this work, advertise the one-time manual update out-of-band (Discord / README / release notes); from then on the new prompt + manifest carry the load. Transition cost is exactly one more gentle-ai release; afterward engram releases reach users via runtime fetch with zero gentle-ai release.

## Success Criteria

- [ ] An engram bump reaches users with ZERO gentle-ai release (runtime fetch).
- [ ] User is prompted at launch (TUI and CLI) when an update is available.
- [ ] "Upgrade + Sync" completes in one action, including across a self-upgrade.
- [ ] `gentle-ai upgrade` installs from the channel-correct source (beta → `@main`).
- [ ] Launches do not hit the GitHub API within the cooldown window; rate-limit no longer silently suppresses the banner.
- [ ] Advisory manifest message displays at launch without gating or forcing updates.
