# Validator phase timing and failure observability

Date: 2026-07-31

Issue: [#389](https://github.com/yersonargotev/packy/issues/389), part of
[#386](https://github.com/yersonargotev/packy/issues/386)

## Decision

The existing validator output is sufficient to identify progress and the
failed phase once execution reaches formatting, build, vet, ordinary tests, or
race tests. GitHub Actions timestamps also make the wall duration of every
current phase diagnosable in CI. Existing plain local output does not provide
portable phase timing, however, and it does not mark the boundary between
Vercel and Addy acceptance precisely enough to reproduce the CI timing split.

That local gap repeated across the three warm-cache and three disposable-cache
runs below. A follow-up may therefore add **opt-in human-readable timestamps**
to explicit phase markers, including separate Vercel and Addy start markers.
The default output and exit behavior should remain unchanged. This is an output
usability recommendation, not a timing gate or a second validation authority.

This research does not propose a canonical receipt schema, JSON snapshot,
candidate digest, result reuse lifecycle, or additional validation authority.
Packy Delivery continues to own the candidate-bound exhaustive-validation
receipt; Packy owns only its observable validator command, phases, output, and
exit status.

## Method and definitions

The canonical entrypoint is
[`scripts/validate-packy.sh`](../../scripts/validate-packy.sh). It executes
Vercel acceptance, Addy acceptance, formatting, build, vet, ordinary tests, and
race tests in that order. The existing `==>` markers begin at formatting.

This report keeps three different quantities separate:

- **wall time** is elapsed time between first-party log timestamps or around a
  complete local process;
- **summed reported package time** is the sum of numeric durations on Go `ok`
  lines within one phase; packages run concurrently, so this is not CPU time
  and may exceed wall time; and
- **job time** includes checkout, toolchain, cache restoration, and other
  workflow overhead in addition to the validator.

The six local cache experiments are the controlled samples already collected
on one immutable tree for
[`validate-packy-performance-regression.md`](validate-packy-performance-regression.md).
The three CI jobs were re-read from their first-party Actions logs for this
question. No candidate, validator, workflow, cache, or user configuration was
mutated to collect the CI evidence.

## Controlled local cache samples

All six successful runs used candidate
[`da2051827d738ef5319bf2fd4a6b5a932dffdc29`](https://github.com/yersonargotev/packy/commit/da2051827d738ef5319bf2fd4a6b5a932dffdc29),
tree `fa3387888e389f8675763afba3d5550aa292b841`, macOS 26.5.2 on Darwin
arm64, and Go 1.26.5. Every run sandboxed `HOME` and `XDG_CONFIG_HOME` under
`/tmp`. The warm cohort reused one preheated `GOCACHE`, `GOMODCACHE`, and
`GOPATH`; every disposable run used fresh roots for all five locations.

| Cache regime | Run 1 wall | Run 2 wall | Run 3 wall | Median | Range |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm, reused after preheat | 225.24s | 225.50s | 225.79s | 225.50s | 225.24–225.79s |
| Fresh disposable roots | 290.24s | 288.98s | 286.42s | 288.98s | 286.42–290.24s |

Each output contained exactly one formatting, build, vet, ordinary-test, and
race marker and exited zero. The retained research record reports package
cohorts separately from wall time: warm `internal/cli` ordinary tests had a
12.470s median and race tests a 45.778s median; disposable medians were 26.265s
and 52.006s respectively. Disposable `internal/release` ordinary tests had a
69.046s median, while the warm result was cached.

The local harness did not retain a durable full sum of every numeric package
duration, so this report does not reconstruct one. More importantly, plain
local output has no timestamps. It can prove phase order and success, but it
cannot derive phase wall durations after the fact. Addy also performs unmarked
mapping validation before its first package marker, so even externally
timestamped output cannot identify the exact Vercel/Addy boundary from the
current local text alone.

## Comparable CI samples

The three successful `CI / Validate Packy-owned code` jobs used GitHub-hosted
Ubuntu 24.04 on linux/amd64, Go 1.23.0, and the same exact verified Linux-X64 Go
cache key. The workflow disables setup-go's implicit cache and restores Packy's
verified cache. Candidate identity is explicit for each row.

| Run and candidate | Job wall | Validator wall | Vercel | Addy | Format | Build | Vet | Ordinary tests | Race |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| [30632193765](https://github.com/yersonargotev/packy/actions/runs/30632193765), `4949c7f` | 339s | 318.722s | 94.144s | 4.850s | 0.172s | 2.059s | 1.108s | 116.860s | 99.528s |
| [30631810670](https://github.com/yersonargotev/packy/actions/runs/30631810670), PR head `02f5611`, checked-out merge `8b6e2f4` | 286s | 255.578s | 73.156s | 3.825s | 0.158s | 1.704s | 0.950s | 97.874s | 77.912s |
| [30627471106](https://github.com/yersonargotev/packy/actions/runs/30627471106), `da20518` | 283s | 260.930s | 83.149s | 4.556s | 0.141s | 1.639s | 0.874s | 90.976s | 79.594s |

Phase wall durations come from adjacent Actions timestamps around validator
output boundaries. Vercel is measured from validator shell start to the first
Addy marker; Addy is measured from that marker to `==> formatting`. This split
is reproducible in Actions logs, but Addy's unmarked prevalidation means the
boundary is an operational approximation rather than an explicit contract.

The same logs expose reported package durations without conflating them with
wall time:

| Run | Addy package sum | Ordinary package sum | Race package sum |
| --- | ---: | ---: | ---: |
| 30632193765 | 2.411s across 7 packages | 182.054s across 15 timed packages | 220.369s across 14 timed packages |
| 30631810670 | 1.745s across 7 packages | 162.684s across 15 timed packages | 177.457s across 14 timed packages |
| 30627471106 | 2.573s across 7 packages | 145.262s across 15 timed packages | 178.929s across 14 timed packages |

Vercel acceptance, formatting, build, and vet emit no numeric package-duration
lines, so only their wall durations are supported. Cached packages and
`[no test files]` entries have no numeric duration and are not included in the
sums.

## Controlled phase failure

The existing fake-command seam in
`TestValidationEntrypointIgnoresHostileUnownedGoContent` was exercised in a
disposable repository copy. The copied Vercel and Addy scripts were successful
stubs, `gofmt` succeeded, and a `go` shim returned exit 97 only for the build
command. All home, configuration, Go cache, module cache, and GOPATH roots were
under `/tmp`; tracked files and operator configuration were untouched.

Observed output was:

```text
==> formatting
==> build
controlled build-phase failure
```

The process exited 97, emitted exactly one formatting marker and one build
marker, and emitted no vet, tests, or race marker. The failing phase is
unambiguous. Because the validator is fail-fast and sequential, reaching the
build marker also proves that both acceptance cohorts and formatting completed.
The same reasoning applies to failures in later marked phases.

The limitation is before formatting: Vercel has no canonical start marker and
Addy's first visible package marker follows unmarked validation work. A failure
inside those acceptance cohorts can print a useful test-specific error, but the
top-level output does not provide the same complete phase-progress ledger.

## Sufficiency assessment

| Diagnostic question | Local output | GitHub Actions output |
| --- | --- | --- |
| Which marked phase failed? | Sufficient | Sufficient |
| Which earlier marked phases completed? | Sufficient by sequential marker order | Sufficient by order and timestamps |
| How long did each marked phase take? | Insufficient without an external timestamping wrapper | Sufficient |
| Can Vercel and Addy wall time be split exactly? | No | Approximate from first Addy marker, not an explicit boundary |
| Are package duration and phase wall time distinguishable? | Yes when numeric package lines are retained | Yes |
| Is a machine receipt or second authority needed? | No | No |

Existing logs therefore suffice for ordinary local and CI failure diagnosis,
and Actions already suffices for CI timing diagnosis. The repeated local timing
gap justifies only a small opt-in presentation change: human-readable
timestamps on explicit phase markers, including Vercel and Addy. Any follow-up
must preserve the default output, phase order, fail-fast exit behavior, package
allowlist, sandboxing, and the single `scripts/validate-packy.sh` authority.

## Limitations

- The local and CI cohorts use different hosts, Go versions, and candidates;
  they answer observability and cache-state questions, not a cross-host
  performance comparison.
- The three CI candidates are controlled by recorded SHA and environment but
  are not the same tree, so their spread is not attributed to one cause.
- GitHub's PR run names both the branch-head SHA and the synthetic merge SHA;
  the checked-out merge SHA is the identity used by the validator.
- Summed package durations are concurrent elapsed results, not CPU time.
- The controlled failure uses the repository's established disposable test
  seam. It proves top-level phase reporting without turning the failure fixture
  into a production validator option.
