# `internal/cli` race-validation cost

Date: 2026-07-31

Issue: [#399](https://github.com/yersonargotev/packy/issues/399), part of
[#386](https://github.com/yersonargotev/packy/issues/386)

## Conclusion

The canonical race phase no longer repeats `internal/cli`. Its 145 top-level
tests contain no goroutines, parallel tests, channels, atomics, wait groups, or
synchronization primitives. They exercise CLI adaptation, rendering, schemas,
filesystem integration, and sandboxed Git fixtures sequentially. The complete
package remains in ordinary exhaustive validation, while the synchronized
domains that the CLI composes remain race-instrumented in their owning
packages.

On one base candidate and toolchain, removing only `internal/cli` from the race
package list reduced the warm-cache race-phase median from 53.82 to
28.66 seconds (46.75%) and the disposable-cache median from 67.60 to 42.08
seconds (37.75%). All twelve comparable full race-phase runs passed. The result
is large relative to both cohorts' ranges, so this report claims a controlled
local improvement, not a prediction of an exact CI duration.

## Inventory and coverage decision

The inventory covered all 19 `internal/cli/*_test.go` files and all 145
top-level tests reported by `go test -list '^Test' ./internal/cli`.

### In-process shared-memory concurrency

None of the CLI tests starts a goroutine, calls `t.Parallel`, creates a channel,
or refers to `sync.Mutex`, `sync.RWMutex`, `sync.Once`, `sync.WaitGroup`, or an
atomic operation. The package composes synchronized implementations, but its
tests call them sequentially:

- `newWorkstationResolver` composes `workstation.Resolver`, whose `sync.Once`
  belongs to `internal/workstation`;
- `activationFacade` composes the capability-pack activation store, whose
  mutex-protected state belongs to `internal/capabilitypack`; and
- the CLI's snapshot, stale-plan, and lifecycle matrices use synchronous fake
  calls and counters rather than concurrent access.

Meaningful in-process contention remains under race in its owning packages.
Examples include bundle-transaction exclusion in `internal/bundletransaction`,
bundle observation and activation coordination in `internal/capabilitypack`,
snapshot gating in `internal/packsync`, and source-resolution coordination in
`internal/skillbundle`. No package containing those tests was removed from the
race list.

### Subprocess-only behavior

The only direct `exec.Command` uses in CLI tests are sandboxed Git operations:

- the repository-fixture helper in `internal/cli/root_test.go`; and
- the optional frozen-baseline identity lookup in
  `internal/cli/identity_equivalence_test.go`.

The race detector instruments the Go test process, not these Git children.
Their behavior, fixtures, failure diagnostics, and environment isolation remain
covered by the ordinary exhaustive CLI run. Other command behavior uses an
in-process fake runner sequentially.

### Repeated integration work

The removed race copy also repeated sequential filesystem and lifecycle
matrices, including the four-way rollout matrix, recovery cases, frozen rename
transcripts, rendering/schema contracts, and the two CLI status tests already
selected by Addy acceptance. Ordinary validation still executes the entire CLI
package exactly once, and Addy acceptance keeps its existing focused status
coverage.

## Deterministic preservation contract

`internal/ci/validation_test.go` continues to execute the real validator with
recording shims and requires the exact build, vet, ordinary-test, and race-test
invocations. It now requires `internal/cli` exactly once in ordinary tests and
zero times under race. The expected race list excludes exactly CLI and release;
every other Packy-owned package remains required.

The negative contract test removes `internal/capabilitypack` from a synthetic
race invocation and requires rejection. A separate Go-AST guard rejects a CLI
test that introduces a goroutine, parallel test, channel, or synchronization
import until the race authority is deliberately reconsidered; synthetic cases
prove each form fails closed. Together these contracts prevent a later omission
of a genuinely concurrent package or a stale CLI exemption from silently
weakening coverage. The existing tracer also keeps the single canonical
validator, explicit package authority, hostile-content isolation, sensitive
release subprocess boundary, diagnostics, and sandboxed HOME/configuration
roots.

## Measurement method and identity

Measurements used base commit
`6cae62527abceb68d5081989965774489bc099b2`. The initial full-phase comparison
used validator/contract patch id
`9f1b72d1ec32c98d140a1bf4a33baf406ef61a0d`; the final after cohort used patch
id `55525f947229f36b25b8b637dbdd56eec4cd4160` after review added only the
fail-closed AST contract tests. CLI and product source were identical across
the cohorts. The host was Darwin arm64 25.5.0 with Go 1.26.5.

Each command used `-race -count=1 -timeout 10m`. HOME, XDG configuration,
`GOCACHE`, `GOMODCACHE`, and GOPATH all lived under per-investigation temporary
roots in `/tmp`; no operator configuration or cache root was read as a test
home or written by validation. Warm samples reused one preheated set of Go
caches. Each disposable sample used fresh roots.

The before package set was the previous canonical race authority: all 39
Packy-owned packages except `internal/release` (38 packages). The after set was
the new authority: all packages except `internal/cli` and `internal/release`
(37 packages). The final after cohort includes the additional fast AST contract
tests in `internal/ci`, so the comparison does not hide their preservation cost.

Before changing the authority, a narrower CLI-only cohort confirmed the
attribution:

| Cache regime | Sample 1 | Sample 2 | Sample 3 | Median | Range |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm `internal/cli -race` | 45.76s | 46.21s | 47.41s | 46.21s | 45.76–47.41s |
| Disposable `internal/cli -race` | 56.30s | 55.38s | 54.87s | 55.38s | 54.87–56.30s |

The comparable full race-phase before/after cohorts were:

| Cache regime | Selection | Sample 1 | Sample 2 | Sample 3 | Median | Range | Change |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Warm | Before: CLI included | 63.25s | 53.82s | 52.68s | 53.82s | 52.68–63.25s | — |
| Warm | After: CLI excluded | 31.10s | 28.66s | 28.62s | 28.66s | 28.62–31.10s | -46.75% |
| Disposable | Before: CLI included | 74.17s | 67.60s | 63.48s | 67.60s | 63.48–74.17s | — |
| Disposable | After: CLI excluded | 43.77s | 42.08s | 39.82s | 42.08s | 39.82–43.77s | -37.75% |

Package durations are not summed: Go runs package-list tests concurrently, so
the wall-clock phase measurements are the comparison authority.

## Preserved boundaries

- `scripts/validate-packy.sh` remains the single local and CI validation
  authority.
- All 145 CLI tests remain in ordinary exhaustive validation.
- Every package with identified in-process concurrent tests remains under
  `-race`.
- Release subprocess and cross-platform scenarios remain ordinary-only and
  execute exactly once.
- Acceptance phases, formatting, build, vet, diagnostics, and the explicit
  Packy-owned allowlist are unchanged.
- The measurements and validator use sandboxed user and configuration roots.

## Limitations

- The measurements are local Darwin arm64 results, not interchangeable with
  GitHub-hosted Ubuntu timings.
- Static inventory establishes that current CLI tests do not create a
  shared-memory concurrency boundary. The AST guard forces a deliberate
  race-authority decision when a future CLI test introduces a direct
  concurrency construct. Indirect concurrency hidden behind a helper still
  requires ordinary code review, while the exact invocation tracer makes
  unrelated package omissions fail closed.
- Wall-clock performance still varies with host scheduling. The median change
  is claimed because the before and after ranges do not overlap in either cache
  regime.
