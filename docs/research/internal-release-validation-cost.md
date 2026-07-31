# `internal/release` ordinary-validation cost

Date: 2026-07-31

Issue: [Profile and reduce internal/release validation cost](https://github.com/yersonargotev/packy/issues/400),
part of [Map: Reduce Packy-owned delivery rework and validation latency](https://github.com/yersonargotev/packy/issues/386)

## Conclusion

The ordinary `internal/release` cohort repeatedly executed the exact successful
fresh tag-push process scenario for assertion-only consumers. Eight executions
used the same event input, checked-out candidate, release-candidate adapter,
toolchain, expected success, normalized result, and fail-fast behavior. Those
assertions now live in one canonical test that executes the scenario twice: one
complete execution and one independent execution retained for the determinism
comparison. No result or sandbox is cached across tests or `-count` iterations.

No build artifact is shared. The four release artifacts have distinct target
platforms, while the package-install smoke binary has a distinct version,
flags, output, and consumer. Identity-drift, recovery, release-disappearance,
and post-effect scenarios also retain their distinct inputs and failure
behavior.

The controlled disposable-cache median fell from 77.65 seconds to 66.74
seconds, 14.05% lower, and the ranges did not overlap. The warm-cache median
fell from 79.32 seconds to 65.98 seconds, but its ranges overlap;
the warm result is inconclusive and is not an improvement claim.

## Candidate and method

The before candidate was
[`a6aa1ba22e40ac38b1b5ee9480a1e348380891a5`](https://github.com/yersonargotev/packy/commit/a6aa1ba22e40ac38b1b5ee9480a1e348380891a5),
tree `135d2c41013119db12741625474cefe64ebf0cb1`. The after candidate was
`b746c40054a989130a0b16359dac2cc979f5ee58`, tree
`d1258326d49c2fd5aa529f467cc8a2a6fb9ae3f2`. Both cohorts used macOS
26.5.2 on Darwin arm64 and Go 1.26.5.

Every sample ran this package command and captured its JSON event stream in the
sandbox for per-test attribution. The commands and resulting summary are
durable here; the disposable raw streams were not checked in.

```sh
/usr/bin/time -p go test ./internal/release -count=1 -json
```

All writable state was beneath a new `/tmp` sandbox. `HOME`,
`XDG_CONFIG_HOME`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, and `TMPDIR` were set
explicitly. Before a sample, `go mod download` populated the sandboxed module
cache from the operator's already-present module-cache directory through a
`file://` proxy. Timed tests then used `GOPROXY=off`; no network was available.

The warm cohort reused one preheated set of sandboxed roots. Its preheat was
not counted. Each disposable sample used new roots and a fresh Go build cache;
local module bytes were populated before timing. Tests used `-count=1`, so no
Go test result was reused.

## Package measurements

| Cache regime | Before samples | Before median; range | After samples | After median; range | Finding |
| --- | --- | --- | --- | --- | --- |
| Warm persistent cache | 65.29s, 79.32s, 87.28s | 79.32s; 65.29–87.28s | 65.62s, 65.98s, 67.33s | 65.98s; 65.62–67.33s | 16.82% lower median, but overlapping ranges make the result inconclusive |
| Fresh disposable cache per run | 79.08s, 77.00s, 77.65s | 77.65s; 77.00–79.08s | 62.90s, 66.74s, 70.16s | 66.74s; 62.90–70.16s | 14.05% lower median with non-overlapping ranges |

The package-level result includes host variance outside the changed seam. In
particular, unchanged cross-build and package-smoke times vary between samples.
The exact affected-test medians give the narrower causal attribution:

| Affected seam | Before warm | After warm | Before disposable | After disposable |
| --- | ---: | ---: | ---: | ---: |
| `TestReleaseWorkflowClassifiesTagPushAndManualModes` | 4.91s | 4.46s | 5.94s | 4.26s |
| `TestReleaseWorkflowSealsCandidateAndRevalidatesPrivilegedBoundaries` | 1.72s | 1.05s | 2.03s | 1.03s |
| Fresh result, root, fake-boundary, determinism, and mutation assertions | 4.73s | 2.33s | 4.15s | 2.14s |
| `TestReleaseScenarioDistinguishesFreshPublicationFromSafeResumeAndCannotRecreateDisappearedRelease` | 2.49s | 1.63s | 2.41s | 1.80s |
| **Sum of medians** | **13.85s** | **9.47s** | **14.53s** | **9.23s** |

The consolidated canonical test pays for two independent process executions
and compares their stable results. Workflow structure, recovery, and ancestry
tests no longer repeat the already-proven exact fresh path; they retain their
distinct scenarios and assertions.

## Cost attribution and identity decision

The reproducible JSON profile identified three material paths:

- `TestBuildReleaseArtifactsCreatesChecksummedSupportedPlatforms` had a
  26.48-second disposable before median. It performs real builds for
  Darwin/amd64, Darwin/arm64, Linux/amd64, and Linux/arm64. Target platform and
  candidate bytes differ, so none is reusable.
- `TestReleaseScenarioRejectsIdentityDriftAtEveryPrivilegedBoundary` had a
  13.09-second disposable before median. Its boundary/drift pairs intentionally
  vary observed identity, expected failure, diagnostic, and preceding effects,
  so none is reusable.
- `TestPackageInstallSmokeLifecycleWithLocalReleaseBinary` had a 9.41-second
  disposable before median. Its native `v0.99.0` binary, flags, and lifecycle
  consumer differ from release artifacts, so it cannot share a cross-build.

The only exact repeated work was the base fresh tag-push scenario. Its result,
disposable-root, fake-boundary, deterministic-rerun, and mutation-sensitive
assertions now share one test lifecycle. The workflow tests retain structural
proof that the checked-in normalizer is the adapter, while dry-run, recovery,
ancestry, disappearance, and every divergent fixture continue through the
unchanged runner with their own sandbox and failure behavior. `go test
-count=2 -shuffle=on` verifies that each iteration independently executes the
canonical scenario; there is no package-global scenario cache.

## Preserved release assurance

- Fresh publication executes once canonically, plus one independent identical
  rerun solely for deterministic-result comparison and one commit-mutated run
  for identity sensitivity.
- Identity drift retains every tag movement and protected-main ancestry loss at
  all five privileged boundaries, plus release target mismatch, missing
  release, and divergent sealed release at every release-bearing boundary.
- Retained-candidate recovery retains one successful original-run continuation
  and the missing candidate, expired metadata, divergent bytes, and divergent
  metadata denials before privilege.
- Release disappearance remains covered both during manual admission and after
  admission during retained continuation.
- Post-effect behavior retains protected-main advancement, exact published
  continuation, release-state reacquisition, and body, attestation, and
  incomplete-asset drift.
- Boundary, identity, effect, and recovery diagnostics remain unchanged. Git,
  GitHub, and release-candidate external boundaries remain fakes.
- `internal/release` remains exactly once in the canonical ordinary-test
  allowlist and remains excluded from race because its subprocess and
  cross-platform children are not race-instrumented.

## Limitations

The measurements characterize two immutable source trees on one Apple-silicon
host and one Go toolchain. They are not interchangeable with GitHub-hosted CI.
Wall-clock variation in unchanged cross-build and package-smoke tests is
material; this is why only the disposable-cache distribution supports a
package-level improvement claim and why the affected-test table is the causal
seam attribution. Performance remains evidence, not a timing assertion in the
test suite.
