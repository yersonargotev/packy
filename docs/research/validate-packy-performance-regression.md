# Current Packy validation performance regression

Date: 2026-07-31

Issue: [#390](https://github.com/yersonargotev/packy/issues/390), part of
[#386](https://github.com/yersonargotev/packy/issues/386)

## Conclusion

The observed increase from a 120-second historical median to 336.5 seconds is
real for the named GitHub Actions cohorts, but it is not evidence of a cache or
runner regression. The cohorts used the same `ubuntu-24.04` runner family and Go
1.23.0. Seven of the eight current runs restored the same verified Go cache key;
the one cache miss finished within the cache-hit cohort's spread.

Controlled local runs on one immutable tree confirm that cache state matters but
does not explain the whole regression. Three warm-cache runs had a 225.50-second
median; three fresh disposable-cache runs had a 288.98-second median. The
63.48-second cache penalty is material, yet the warm median itself remains
105.50 seconds above the historical CI median.

The first-party logs attribute the increase principally to added Packy-owned
work since the historical candidates:

- a Vercel acceptance pre-phase now consumes roughly 87–95 seconds before the
  unchanged Addy package markers begin;
- ordinary `internal/release` increased from 71–76 seconds to 88–139 seconds;
- `internal/cli` under race increased from 11–12 seconds to 78–90 seconds; and
- the validator's explicit allowlist grew from 24 to 39 packages.

No recursive or duplicate canonical phase is visible. Each current log has one
format, build, vet, ordinary-test, and race marker; the structural tracer also
requires exactly those four Go invocations, requires `internal/release` exactly
once in ordinary tests and never under race, and rejects a recursively launched
validator. The cause is therefore classified as **repository-work growth**, with
ordinary runner variation inside that larger workload. Cache state is not the
primary cause, and the evidence does not support adding parallelism, a second
cache, a second validator, or removing a validation stage.

## Method and identities

All durations below come from first-party Actions timestamps or Go package
timings, not a synthetic wall-clock assertion. Historical identities are the
three runs recorded immediately after the optimization completed in
[`ci-validation-performance-evidence.md`](ci-validation-performance-evidence.md).
The current eight-run cohort is the last eight successful validation jobs before
issue #390 was opened. This makes each cohort internally coherent, but candidates
differ within and between cohorts; the comparison diagnoses accumulated
repository work rather than benchmarking one immutable tree before and after a
single change.

The checked-out research identity is
[`da2051827d738ef5319bf2fd4a6b5a932dffdc29`](https://github.com/yersonargotev/packy/commit/da2051827d738ef5319bf2fd4a6b5a932dffdc29),
the 2026-07-31 merge of PR #397. Current local samples must record this exact
identity (or replace it consistently if the main agent deliberately samples a
later tree). The canonical entrypoint and phase order are defined by
[`scripts/validate-packy.sh`](../../scripts/validate-packy.sh); CI invokes that
entrypoint once in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml).

## Comparable CI samples

### Historical optimized cohort

All three jobs used GitHub-hosted `ubuntu-24.04`, runner 2.335.1, image
`20260714.240.1`, Go 1.23.0, and a setup-go cache hit with key suffix
`b4a9db62…`. The validation step included one Addy cohort and one copy of every
canonical phase.

| Run | Candidate | Validation step | `internal/release` ordinary | `internal/cli` race |
| --- | --- | ---: | ---: | ---: |
| [29717665008](https://github.com/yersonargotev/packy/actions/runs/29717665008) | `01100fe` | 120s | 75.777s | 12.183s |
| [29719076007](https://github.com/yersonargotev/packy/actions/runs/29719076007) | `b26ac9f` | 121s | 75.236s | 11.810s |
| [29719190010](https://github.com/yersonargotev/packy/actions/runs/29719190010) | `ade996a` | 115s | 71.426s | 11.152s |
| **Median; range** | | **120s; 115–121s** | **75.236s; 71.426–75.777s** | **11.810s; 11.152–12.183s** |

### Current regression cohort

All eight jobs used GitHub-hosted `ubuntu-24.04`, runner 2.336.0, Go 1.23.0,
and image `20260720.247.2` except run 30594994642, which used
`20260726.254.1`. Seven restored verified cache key
`Linux-X64-go-3a7d45c…`; run 30588833456 recorded a miss for that key.

| Run | Candidate | Cache | Validation step | `internal/release` ordinary | `internal/cli` race |
| --- | --- | --- | ---: | ---: | ---: |
| [30607228763](https://github.com/yersonargotev/packy/actions/runs/30607228763) | `5cf9558` | hit | 353s | 134.301s | 88.134s |
| [30606890117](https://github.com/yersonargotev/packy/actions/runs/30606890117) | `4917a85` | hit | 365s | 139.333s | 89.966s |
| [30594994642](https://github.com/yersonargotev/packy/actions/runs/30594994642) | `9d4e7b3` | hit | 308s | 89.902s | 85.361s |
| [30594636234](https://github.com/yersonargotev/packy/actions/runs/30594636234) | `d536a83` | hit | 303s | 90.249s | 88.733s |
| [30591879941](https://github.com/yersonargotev/packy/actions/runs/30591879941) | `89f9442` | hit | 319s | 95.701s | 77.917s |
| [30591479805](https://github.com/yersonargotev/packy/actions/runs/30591479805) | `e27d9a7` | hit | 320s | 96.507s | 81.700s |
| [30588833456](https://github.com/yersonargotev/packy/actions/runs/30588833456) | `46ea307` | miss | 355s | 87.773s | 87.810s |
| [30588227965](https://github.com/yersonargotev/packy/actions/runs/30588227965) | `e0f4b55` | hit | 372s | 92.481s | 89.785s |
| **Median; range** | | | **336.5s; 303–372s** | **94.091s; 87.773–139.333s** | **87.972s; 77.917–89.966s** |

The cache-miss run's 355 seconds is slower than the cohort median but faster
than two cache-hit runs. Restoring a cache is therefore not sufficient to
explain the spread or the 216.5-second median increase. The small runner/image
revision changes also do not align with the timing order.

## Local cache-state samples

All six successful runs used commit `da2051827d738ef5319bf2fd4a6b5a932dffdc29`,
tree `fa3387888e389f8675763afba3d5550aa292b841`, macOS 26.5.2 on Darwin arm64,
and Go 1.26.5. They invoked the unchanged `./scripts/validate-packy.sh` entrypoint
with `HOME` and `XDG_CONFIG_HOME` beneath `/tmp`. The warm cohort reused one
preheated `GOCACHE`, `GOMODCACHE`, `GOPATH`, home, and configuration root. Each
disposable run used new roots for all five. No sample used or deleted the
operator's real caches or configuration. Each output contained exactly one
format, build, vet, ordinary-test, and race marker and exited zero.

| Cache regime | Sample 1 | Sample 2 | Sample 3 | Median | Range |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm persistent Go caches | 225.24s | 225.50s | 225.79s | 225.50s | 225.24–225.79s |
| Fresh disposable Go caches per run | 290.24s | 288.98s | 286.42s | 288.98s | 286.42–290.24s |

Local samples answer how cache state affects one current candidate on one host.
They must not be presented as a before/after optimization result or directly
merged with GitHub-hosted runner measurements.

The package output supplies a narrower local attribution. Warm-cache ordinary
`internal/release` results were cached; under fresh disposable roots their
median was 69.046 seconds (68.655–69.893). `internal/cli` ordinary tests had a
12.470-second warm median (12.444–12.524) and a 26.265-second disposable median
(26.057–26.404). Its race result remained substantial in both regimes: a
45.778-second warm median (45.695–45.991) and 52.006-second disposable median
(51.915–52.302). Cache reuse therefore removes real work, especially the
ordinary release cohort, while acceptance and meaningful race work still leave
a stable warm critical path.

## Canonical phase attribution

The current validator runs Vercel acceptance, Addy acceptance, formatting,
build, vet, ordinary tests, and meaningful race tests in that order. Its package
authority is explicit rather than `./...`; `internal/release` is excluded from
build because it is test-only and from race because its subprocess and
cross-platform children are not instrumented, while it remains in the ordinary
exhaustive suite. See [`scripts/validate-packy.sh`](../../scripts/validate-packy.sh).

The current logs permit bounded attribution without inventing precision:

| Cohort/phase | Historical logs | Current logs | Finding |
| --- | ---: | ---: | --- |
| Pre-format acceptance | about 6–11s | about 92–96s | New Vercel acceptance dominates the increase before formatting; Addy's seven package markers themselves remain about 5–8s. |
| Format | under 0.1s | about 0.2s | Immaterial. |
| Build | about 0.7s | about 2.0–2.3s | Growth, but not material to the total. |
| Vet | about 0.8s | about 1.0–4.1s | Growth, but not material to the total. |
| Ordinary tests | about 78–84s | about 108–158s | `internal/release` is the supported critical-path attribution; `internal/cli` also rose from about 5s to 27–37s but generally completes before release. |
| Race | about 21–24s | about 98–126s | `internal/cli -race` is the largest reported package cohort at 78–90s, versus 11–12s historically. |

Package times are elapsed package results produced by concurrent package-list
testing; they cannot be summed as though packages ran serially. Phase-marker
intervals are the canonical wall-clock attribution. This is why the report uses
`internal/release` and `internal/cli` as supported cohorts but does not assign
the remaining phase time speculatively to individual tests.

## Causal classification

### Repository work: supported primary cause

Between historical candidate `ade996a` and the checked-out identity, the
validator allowlist grew from 24 to 39 packages. The checked-in diff adds a
Vercel acceptance script and substantial tests under `internal/ci`,
`internal/cli`, and `internal/release`. The log deltas occur exactly in those
acceptance, ordinary-test, and race phases. This is direct evidence of more
intentional repository assurance work, not accidental rediscovery of unowned
packages.

### Duplicate execution: not observed

Every sampled current log contains exactly one canonical marker for formatting,
build, vet, ordinary tests, and race. The behavioral tracer in
[`internal/ci/validation_test.go`](../../internal/ci/validation_test.go) records
the real entrypoint's child invocations, rejects recursive Bash validation,
asserts the exact package allowlist, and proves the expensive release package
appears once in ordinary tests and zero times in race. This preserves the fix
and boundary originally specified in [#101](https://github.com/yersonargotev/packy/issues/101)
and documented in
[`validate-packy-performance.md`](validate-packy-performance.md).

### Cache/runner variation: secondary only

The current cohort's single verified-cache miss does not separate from the hit
distribution. Both cohorts use Ubuntu 24.04 and Go 1.23.0, and the minor runner
and image revisions do not track the duration ordering. Cache and shared-runner
variance remain plausible contributors to the 69-second current spread, but
they cannot explain the phase-localized median increase.

### Another observed cause: no evidence

The primary sources show neither another validator authority nor missing
canonical stages. Network and subprocess work inside acceptance/release tests
may contribute to their measured package times, but the logs and structural
sources do not support a narrower claim without a dedicated controlled
benchmark.

## Safe optimization reasoning

This research does not implement or claim a measured optimization. It narrows
future work to independently testable boundaries:

1. **[Vercel acceptance](https://github.com/yersonargotev/packy/issues/398)** is
   an independent pre-phase with its own script and
   deterministic failure boundary. A follow-up may profile its row/test
   invocations and remove only demonstrated duplicate work. Any cache must bind
   the exact candidate, toolchain, acceptance inputs, and result identity and
   fail closed on absence or mismatch. It must preserve all acceptance rows and
   the one canonical caller.
2. **[`internal/cli -race`](https://github.com/yersonargotev/packy/issues/399)**
   is independently selectable only at the test level
   after a coverage review distinguishes instrumented in-process concurrency
   from subprocess-only or structurally duplicated tests. A follow-up must keep
   meaningful race coverage and deterministic failure reporting; package-wide
   removal from race is not supported by this report.
3. **[`internal/release` ordinary tests](https://github.com/yersonargotev/packy/issues/400)**
   are sensitive subprocess, artifact,
   and release scenarios and must remain in the ordinary exhaustive phase
   exactly once. A follow-up may profile whether shared setup or repeated
   cross-builds are demonstrably identical, but must preserve immutable-release,
   recovery, and process-scenario coverage.

Each non-trivial item is now a separate implementation ticket, blocked by #390
and linked beneath #386. Parallel phases, path selection, dropping a cohort, or
adding a result receipt are not authorized: they change orchestration or
assurance rather than removing measured work. Each ticket requires comparable
before/after samples on the same candidate and cache regime; this report makes
no optimization before/after claim.

## Preserved boundaries

- `scripts/validate-packy.sh` remains the single exhaustive local and CI
  authority.
- Vercel and Addy acceptance, formatting, build, vet, ordinary tests,
  meaningful race coverage, and sensitive release subprocess coverage remain.
- The explicit Packy-owned allowlist remains the trust boundary; no `go test
  ./...` discovery is introduced.
- Validation continues to sandbox `HOME` and `XDG_CONFIG_HOME` while preserving
  only explicitly selected Go caches.
- Performance observations remain evidence, not flaky wall-clock gates.

## Limitations

- The historical and current CI cohorts contain different candidates, so they
  establish regression shape and attribution, not a controlled single-change
  causal estimate.
- GitHub-hosted runner CPU contention is not observable in these logs; the
  report therefore does not claim that all current spread is repository-caused.
- Go's package timings do not expose individual-test cost and concurrent package
  timings are not additive.
- The six controlled local samples use one Apple-silicon host and Go 1.26.5;
  they quantify cache sensitivity for that identity but are not interchangeable
  with Ubuntu/Go 1.23 CI samples.
- The measurement harness captured phase markers and package output but did not
  add timestamps inside the validator; precise local acceptance sub-phase wall
  times remain outside this issue and are addressed by #389's observability
  experiment.
- Seven canonical validator invocations were made locally: one uncounted warm
  preheat and the six reported samples. All effects were confined to disposable
  roots under `/tmp`; only the three linked implementation issues changed
  external tracker state.
