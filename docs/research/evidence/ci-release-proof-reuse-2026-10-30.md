# CI source-proof reuse evidence

Date: 2026-10-30. This is non-normative evidence for
[issue 909](https://github.com/yersonargotev/packy/issues/909).
The release contract remains in [Packy release](../../../workflows/packy-release.md).

## Observed invocation map

GitHub's workflow-run and attempt-job APIs returned these successful runs and
step timestamps. Durations below describe those historical runs, not the
performance of the proposed change.

| Run | Source commit | Run elapsed | Complete source-validation step |
| --- | --- | --- | --- |
| [PR CI](https://github.com/yersonargotev/packy/actions/runs/41109253477) | `dde1cbbd713fd8517ecefb04ec387c8cd26f7182` | 11m09s | 18:43:23 to 18:54:01 UTC, 10m38s |
| [Main CI](https://github.com/yersonargotev/packy/actions/runs/41110970449) | `1787223cc10000849a4e684b55b9b49e60c3901c` | 11m11s | 18:55:43 to 19:06:12 UTC, 10m29s |
| [Release v0.2.36](https://github.com/yersonargotev/packy/actions/runs/41115011399) | `1787223cc10000849a4e684b55b9b49e60c3901c` | 24m14s | 19:24:47 to 19:36:01 UTC, 11m14s |

The three run elapsed times sum to 46m34s. That sum excludes the intervals
between runs. The PR and main runs validate different commits. Main CI and
Release validate the same commit.

In both CI runs, `validate` executes `validate-packy.sh --ci` independently of
`release-artifacts`. The latter builds unpublished `v0.0.0` archives before
four parallel native `release-contracts` jobs. The source-validation branch
finishes last in these runs. Main CI's archive build takes 2m25s, followed by
native contracts whose last job completes at 19:04:16 UTC.

Release serializes its duplicate source validator before the selected-version
archive build. That build runs from 19:36:01 to 19:38:26 UTC, also 2m25s.
The four native jobs then test those actual release archives. Darwin amd64 is
the last contract job, ending at 19:46:09 UTC. Protected GitHub publication,
protected Homebrew publication, and installation smoke follow in that order.

Job starts, setup steps, job transitions, and environment waits contribute to
elapsed time. The gap between a prerequisite finishing and the next job
starting does not establish how much time belongs to queueing versus approval.
These observations prove duplication and dependency order. They do not measure
a future elapsed-time improvement.

The pre-change operator workflow also requires a local canonical validator.
That invocation is outside these three Actions runs and has no measured
duration in this comparison.

## Reproduce the historical observations

For each run ID in the table, read run metadata and every page of attempt 1's
jobs. The API responses expose run timestamps, commit, event, workflow identity,
job timestamps, and individual step results.

```sh
gh api repos/yersonargotev/packy/actions/runs/41110970449
gh api --paginate --slurp \
	'repos/yersonargotev/packy/actions/runs/41110970449/attempts/1/jobs?per_page=100'
```

The main run reports repository `yersonargotev/packy`, workflow ID `307623507`,
path `.github/workflows/ci.yml`, event `push`, branch `main`, and attempt `1`.
Its source-validation job is `124252982945`. Its `Run CI validation` step
completes successfully. These identifiers describe this observation and are
not configured release inputs.

## Comparison boundary

The change replaces the operator's local canonical validation and Release's
duplicate canonical validation with verification of trusted exact-commit main
CI evidence. PR and main CI retain their complete source-validation jobs.
Release still constructs the selected version from clean exact source and
executes four native contracts on its own retained archives.

Regression tests and a read-only invocation of the verifier can establish that
source-validation work is removed and valid main evidence is accepted.
They cannot measure the new tag-triggered critical path. No release or release
tag is published for this implementation. A comparable future release run is
needed before reporting an elapsed-time reduction.
