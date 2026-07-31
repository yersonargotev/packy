# Vercel acceptance validation cost

Date: 2026-07-31

Issue: [Profile and reduce Vercel acceptance validation cost](https://github.com/yersonargotev/packy/issues/398)

## Conclusion

The Vercel foundation collector executed 126 fresh `go test` processes for 63
row/kind mappings, although those mappings name only 36 distinct package/test
seams. Every mapping requested two deterministic runs, so 54 processes repeated
an already completed exact pair without adding candidate, toolchain, input,
expected-result, or failure-behavior coverage.

The collector now executes each exact seam twice and reuses only the normalized
test bodies inside that invocation. It still materializes separate first and
second artifacts for every row and proof kind. The complete 24-row acceptance
matrix, all positive/negative/oracle mappings, deterministic rerun checks,
candidate-bound evidence, fail-fast diagnostic, and single canonical caller are
unchanged.

On the controlled local benchmark, the warm-cache median fell from 122.62 to
55.61 seconds (54.65%), and the disposable-cache median fell from 129.89 to
64.27 seconds (50.52%). These distributions do not overlap, so the controlled
result supports an improvement claim for this host and toolchain. They are not
a prediction of GitHub-hosted runner duration.

## Controlled identity and method

All samples used base candidate
`36b9ed5815bb891a769ec2c8bf417e5af6e5a260`, base tree
`18dd7f256e5b75cc4d72e5e1424e552d100de1e1`, Go 1.26.5, Darwin arm64,
and macOS kernel 25.5.0. The before cohort used the unmodified candidate. The
after cohort used the same checkout with only the collector change and its
contract tests applied; no other source input changed between cohorts.

An uncounted preheat preceded each warm cohort. The three counted warm samples
reused one sandboxed `HOME`, `XDG_CONFIG_HOME`, `GOCACHE`, `GOMODCACHE`, and
`GOPATH`. Each disposable sample used new roots for all five. The already
resolved module cache was copied before timing, and the disposable commands set
`GOPROXY=off` and `GOTOOLCHAIN=local`, so measurement performed no network fetch
or toolchain installation. Every root was beneath `/tmp`; no operator home or
configuration path was written.

`/usr/bin/time -p` measured the acceptance phase wall clock. A `go` wrapper also
recorded each exact `go test` command and its own wall duration. Because the
foundation tests run sequentially, these command durations support exact-seam
attribution. They are reported separately from the phase wall clock and are not
combined with concurrently executed package durations from other validator
phases.

## Before samples

| Cache regime | Sample 1 | Sample 2 | Sample 3 | Median | Range |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm persistent caches | 122.62s | 119.90s | 123.23s | 122.62s | 119.90–123.23s |
| Fresh disposable caches | 129.89s | 128.77s | 130.28s | 129.89s | 128.77–130.28s |

Every before profile recorded 126 test invocations. Their sequential command
totals were 116.91–120.21 seconds warm and 120.40–121.86 seconds disposable.

The 63 foundation mappings contain 36 distinct seams: 17 appear once, 15 twice,
two three times, and two five times. Since each mapping ran twice, executing one
deterministic pair per exact seam removes 54 of 126 processes (42.9%). Material
preheat attribution was:

| Exact test seam | Mappings | Calls | Sequential wall | Calls beyond exact pair |
| --- | ---: | ---: | ---: | ---: |
| `./internal/tools/syncpacksource/TestCompositeBundleTracerReacquiresEveryMemberAndPublishesPackScope` | 5 | 10 | 47.07s | 8 |
| `./internal/vercelacceptance/TestExactSelectedTreesAreCompleteInertAndSealed` | 5 | 10 | 5.68s | 8 |
| `./internal/capabilitypack/TestEvaluateRuntimeModesRequiresExactCoverageAndSecretSafeDiagnostics` | 3 | 6 | 3.48s | 4 |
| `./internal/opencodesmoke/TestPreflightEveryVercelModeFailsBeforeHostEffects` | 3 | 6 | 3.28s | 4 |

The first seam spans positive and oracle responsibilities in acceptance rows 02,
21, and 22. Those responsibilities remain separate in the row registry and in
their emitted proof artifacts; only their identical package/test execution is
shared.

## After samples

| Cache regime | Sample 1 | Sample 2 | Sample 3 | Median | Range |
| --- | ---: | ---: | ---: | ---: | ---: |
| Warm persistent caches | 56.19s | 55.08s | 55.61s | 55.61s | 55.08–56.19s |
| Fresh disposable caches | 64.27s | 63.47s | 69.56s | 64.27s | 63.47–69.56s |

Every after profile recorded exactly 72 invocations: two fresh `-count=1` runs
for each of the same 36 seams. Their sequential command totals were 52.17–53.20
seconds warm and 55.19–61.26 seconds disposable. No acceptance row or proof kind
was removed.

## Equivalence and failure boundary

Reuse is invocation-local and keyed by the candidate SHA, canonical Go version,
GOOS, GOARCH, package, and exact test name. A failing run is never cached: the
collector prints its raw output and the same first-occurrence
`row kind package/test` diagnostic, then exits before later seams. A successful
pair contributes two independently normalized bodies. Each consumer then adds
the cohort identity, exact seam, and toolchain identity to its own canonical
row/kind artifact.

The domain validator still requires the first and second artifacts to be
byte-identical, match the manifest digest, begin with the exact candidate/run/
observation/seam identity, and contain exactly one named RUN/PASS proof. Contract
tests execute the real shell collector behind fake `go` and `git` boundaries to
prove exact-pair invocation counts, byte-identical reuse across rows, toolchain
binding, and the preserved first-row fail-fast diagnostic. Existing tests retain
the 24-row registry, every negative twin and oracle, normalization sensitivity,
and exactly one call from `scripts/validate-packy.sh` with no CI bypass.

## Boundaries and limitations

- `scripts/validate-packy.sh` remains the single local and CI validation
  authority and invokes Vercel acceptance once.
- The collector does not persist a cache across commands or candidates.
- Host evidence rows 17–19 are still collected independently and joined by the
  acceptance gate; the optimized foundation phase still owns the other 21 rows.
- The result applies to the named local identity. CI runner contention and the
  other canonical validation phases were not part of this benchmark.
- Performance remains reported evidence, not a timing gate.
