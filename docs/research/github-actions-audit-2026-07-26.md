# GitHub Actions value and cost audit

Research date: 2026-07-26

## Question and evidence boundary

Which checked-in GitHub Actions workflows are active, what causes each one to
run, which ones enforce pull-request or `main` integration policy, how have
their recent runs behaved, and where does their value justify—or fail to
justify—their execution cost?

This is a read-only point-in-time audit of `yersonargotev/packy` at
[`0b87d489759854831688af9d730750304daf6d7a`](https://github.com/yersonargotev/packy/tree/0b87d489759854831688af9d730750304daf6d7a).
The evidence is the nine YAML files under `.github/workflows`, the GitHub
Actions workflow/run/job APIs queried through `gh`, failed-run logs, and the
live `main` branch-protection response. No workflow, run, issue, pull request,
repository setting, secret, release, ref, or local user configuration was
changed. Durations below are elapsed wall time from GitHub's run timestamps,
not billable-minute exports.

## Executive finding

Packy has three materially different Actions surfaces:

1. **PR integration gates:** `CI`, PR `Security`, and `Governance` produce the
   six checks currently required by protected `main`. `Addy trusted governance`
   supplies trusted evidence consumed by CI's required Addy promotion gate.
2. **Post-integration assurance:** trusted-main `Security`, `Claude stable
   canary`, and `Governance drift` detect vulnerabilities, upstream
   compatibility changes, and repository-governance drift after or independent
   of a PR.
3. **Operational automation:** `Release` and `Synchronize pack source` are
   manually dispatched, evidence-heavy mutation workflows. They do not gate
   ordinary PR integration and should not be evaluated as CI.

The recent PR gate signal is strong: the last ten runs of CI, PR Security, and
Addy trusted governance all succeeded. Their largest recurring cost is CI's
multiple macOS compatibility jobs: median workflow elapsed time was 6m42s, with
several jobs executing in parallel, so elapsed time understates runner usage.
The operational sync workflow is the clear reliability and cost outlier: 25 of
its 46 observed dispatches failed. For today's Vercel admission, all seven
protected-`main` production dispatches failed; four feature-branch preparation
dispatches succeeded, but that read-only proof cannot exercise the final GitHub
write boundary. A complete production or preparation path runs the full
Packy-owned validation suite **five times**, in separate cold-cache jobs; the
latest production failure consumed 44m10s before an oversized `gh` argument
list stopped Publish.

## Live enforcement boundary

Classic branch protection on `main` is active, applies to admins, requires
up-to-date branches and conversation resolution, and requires these checks:

| Required context | Producer | Meaning |
|---|---|---|
| `Validate Packy-owned code` | `CI` | Canonical `./scripts/validate-packy.sh` validation. |
| `Claude 2.1.203 package smoke` | `CI` | Exact-floor macOS package compatibility. |
| `Addy 1.1.0 promotion gate` | `CI` | Promotion decision using trusted exact-merge evidence. |
| `Governance / Validate authorization` | `Governance` | PR-to-open-issue authorization metadata. |
| `CodeQL` | PR `Security` | Read-only CodeQL analysis of candidate code. |
| `Dependency review` | PR `Security` | Advisory dependency-change review. |

Thus these workflows can block a PR merge. None of the push/schedule-only jobs
can prevent a commit already merged or pushed to `main`; they are detectors.
`Addy trusted governance` is not itself a required context, but its artifact is
part of the required Addy gate's trust chain. The Release and sync workflows
can block their own publication operations, not normal integration.

## CI and assurance workflows

Recent-run statistics cover up to the latest ten runs returned by GitHub on
2026-07-26. “Range” is shortest-to-longest elapsed workflow duration.

| Workflow | Trigger and purpose | Enforcement | Recent executions | Value and cost |
|---|---|---|---|---|
| **CI** (`ci.yml`) | Every PR, push to `main`, and Mondays 06:17 UTC. Runs Packy validation; PR-only Claude, Vercel-host, Codex, OpenCode, Vercel acceptance, and Addy promotion checks; replays Addy effect-free on trusted main. | Three required PR contexts. Push/schedule results are post-integration assurance. | Last 10: **10 success**; median **6m42s**, range **5m30s–7m46s**. Latest [run 30219643948](https://github.com/yersonargotev/packy/actions/runs/30219643948), push, success in 5m30s. | Highest direct integration value. Main cost is five PR-only compatibility/acceptance jobs, including macOS runners and package/tool setup. The weekly schedule overlaps with same-time Security but provides a useful no-change regression pulse. |
| **Addy trusted governance** (`addy-governance.yml`) | `pull_request_target` on opened, reopened, or synchronized PRs. Checks out the exact protected base, resolves the current merge/workflow identity through trusted metadata, and retains sanitized promotion evidence without executing candidate workflow code. | Indirectly supports the required Addy promotion context. | Last 10: **10 success**; median **34s**, range **28–39s**. Latest [run 30218236228](https://github.com/yersonargotev/packy/actions/runs/30218236228), success in 39s. | High trust-boundary value at low observed cost. Its use of a privileged event is constrained by base checkout, read permissions, exact identities, and sanitized artifacts. |
| **Security (PR)** (`security-pr.yml`) | Every PR. Runs read-only CodeQL without upload and advisory dependency review. | Both job names are required contexts. | Last 10: **10 success**; median **1m30s**, range **1m23s–1m47s**. Latest [run 30218237333](https://github.com/yersonargotev/packy/actions/runs/30218237333), success in 1m33s. | High security value and modest cost. Dependency review is doubly advisory (`warn-only` and `continue-on-error`) even though its job context is required; it guarantees execution, not rejection of a vulnerable dependency change. |
| **Security (trusted main)** (`security.yml`) | Push to `main` and Mondays 06:17 UTC. Runs CodeQL and uploads security events. | Does not gate integration; detects on trusted main. | Last 10: **10 success**; median **1m38s**, range **1m26s–1m51s**. Latest [run 30219643964](https://github.com/yersonargotev/packy/actions/runs/30219643964), success in 1m26s. | Preserves the code-scanning alert surface that PR analysis intentionally cannot upload. Some compute duplicates PR CodeQL after every merge, but the permission boundary and upload purpose are distinct. |
| **Governance** (`governance.yml`) | PR metadata changes; issue lifecycle/label changes; issue comments; completion of Release or sync runs. Resolves affected PRs from trusted metadata and publishes authorization status. | Produces the required `Governance / Validate authorization` context. | Last 10: **9 success, 1 failure**; median **31s**, range **7–39s**. Latest [run 30221375565](https://github.com/yersonargotev/packy/actions/runs/30221375565), success in 9s. | High policy value at low cost, but event breadth creates many short runs. The one failure was expected fail-closed behavior: [run 30219628430](https://github.com/yersonargotev/packy/actions/runs/30219628430) denied PR 313 because closing issue 312 was no longer open; a following event succeeded. This is policy signal, not workflow instability. |
| **Claude stable canary** (`claude-canary.yml`) | Daily 07:17 UTC or manual dispatch on `main`. Runs current-`stable` Claude package smoke on macOS, retains evidence, and opens one canonical compatibility issue on a smoke failure. | Advisory; exact-floor PR gate remains independent. | All 6 lifetime/recent runs: **5 success, 1 failure**; median **58s**, range **25s–1m21s**. Latest [run 30196370629](https://github.com/yersonargotev/packy/actions/runs/30196370629), success in 47s. | Valuable early warning for upstream drift, but it consumes a macOS runner daily and can mutate issues. The initial scheduled [failure 29908756331](https://github.com/yersonargotev/packy/actions/runs/29908756331) lacked the expected evidence directory, artifact upload also failed, and issue 178 was opened; five subsequent runs succeeded, so no repeated current failure is visible. |
| **Governance drift** (`governance-drift.yml`) | Mondays 08:43 UTC or manual dispatch on `main`. Observes expected repository governance using a read-only token, retains evidence, and maintains a canonical drift issue. | Advisory detector; Release and sync separately call the same fail-closed boundary before publication. | Only [run 30094525097](https://github.com/yersonargotev/packy/actions/runs/30094525097): **success in 1m08s**. | High control value with negligible weekly cost, but one manual run is too little evidence for reliability or duration trends. Issue write permission is isolated to reporting. |

### CI duplication and signal notes

- PR CI and PR Security are intentionally separate permission/security domains;
  combining them would save little elapsed time because they run concurrently
  and would deepen a single workflow's authority.
- CI repeats `./scripts/validate-packy.sh` on PR and then on `main`. The push run
  cannot protect the merge, but it verifies the exact integrated commit and
  supports post-merge evidence. Whether that duplicate is worth its runner cost
  is a policy decision, not a correctness defect.
- CI's Monday schedule and its `main` push both execute only jobs whose event
  predicates permit them. The expensive PR compatibility matrix is skipped on
  push/schedule, materially limiting background cost.
- The PR Security workflow and trusted-main Security workflow intentionally
  share the display name `Security`. GitHub distinguishes their workflow IDs,
  but the identical name makes run-list diagnosis less clear.

## Operational automation

### Release

`release.yml` is manual-only and accepts an existing `v0.x.y` tag plus a
default-true `dry_run`. It checks governance drift, proves tag/main identity,
validates once, builds six retained assets, runs a four-cell macOS Claude
matrix, seals evidence and release metadata, and then either reports planned
effects or uses OIDC/attestation, GitHub Release, and separate Homebrew
publication jobs.

It is not a PR or `main` gate. It is a high-value build-once publication
transaction whose cost is expected to be high and infrequent.

- Last 10: **9 success, 1 failure**; median **3m50s**, range **47s–29m17s**.
- Latest [run 29978775463](https://github.com/yersonargotev/packy/actions/runs/29978775463):
  success in 29m17s (the run sat roughly 10m before its recorded start, so
  runner execution and queue time should be separated in billing analysis).
- The one recent [failure 29956171985](https://github.com/yersonargotev/packy/actions/runs/29956171985)
  failed all four Claude smoke matrix cells with `evidence command sequence is
  malformed`; later dispatches succeeded. It was a shared deterministic
  release-smoke defect rather than four independent failures.
- Historical tag-push runs in the ten-run window reflect an older workflow
  shape; the current YAML is `workflow_dispatch` only. Comparing their
  sub-two-minute durations with current evidence-heavy runs would be
  misleading.

### Synchronize pack source

`sync-pack-source.yml` is manual-only. It admits either protected-main
publication or a tightly named feature-branch `register_bundle` preparation,
then performs Inspect, Classify, sandbox Validate, and either effect-free
Prepare or write-capable Publish. Only Publish has `contents: write` and
`pull-requests: write`; AI classification has `models: read`. The workflow
serializes work per Pack/source and does not cancel an in-progress operation.

It is not CI. It is a fail-closed operational transaction that creates or
updates synchronization branches/PRs only after reacquisition and repeated
validation.

- Last 10: **4 success, 6 failure**; median **7m13s**, range **4m54s–44m10s**.
- Recent failures occurred at different maturation points:
  - [30211691397](https://github.com/yersonargotev/packy/actions/runs/30211691397):
    Classify failed after 7m14s.
  - [30215069237](https://github.com/yersonargotev/packy/actions/runs/30215069237)
    and [30215536642](https://github.com/yersonargotev/packy/actions/runs/30215536642):
    Inspect's disposable composite validation exposed failures in
    `internal/tools/syncpacksource` tests.
  - [30215963094](https://github.com/yersonargotev/packy/actions/runs/30215963094):
    Validate failed on a transient-looking local Git object copy (`No such file
    or directory`) in the sandbox tracer test.
  - [30216783345](https://github.com/yersonargotev/packy/actions/runs/30216783345):
    effect-free Prepare failed during test temporary-directory cleanup
    (`directory not empty`).
  - Latest [30219712500](https://github.com/yersonargotev/packy/actions/runs/30219712500):
    Publish failed after 44m10s because invoking `/usr/bin/gh` exceeded the OS
    argument-list limit.

Across the 46 observed lifetime runs, **21 succeeded and 25 failed**. In the
latest 15 Vercel-related runs, **4 succeeded and 11 failed**; the four successes
were feature-branch `prepare_only` proofs, while all seven protected-`main`
production dispatches failed.

The current v3 phase composition explains the full-path duration:

| Phase | Complete `./scripts/validate-packy.sh` executions |
|---|---:|
| Inspect | 1, while checking the staged composite result |
| Validate | 2: one inside `ApplyComposite`, then one against the applied tree |
| Prepare or Publish | 2: one inside the shared Apply prefix, then one against the applied tree |
| **Total** | **5** |

That complete script runs Vercel and Addy acceptance, formatting, builds, vet,
all allowlisted tests, and the race suite. Every `setup-go` occurrence in the
sync workflow sets `cache: false`, so each permission-separated job starts
cold. Normal PR CI already ran the same full validation authority before the
workflow code reached protected `main`.

The repeated failure pattern is not one persistent domain rejection. It is **late,
expensive discovery of harness/filesystem/process-boundary defects**, because
the full Packy suite is executed inside multiple lifecycle phases and again
before mutation. That repetition has strong fail-closed value, but poor
failure economics when deterministic size limits or flaky Git/tempdir tests
surface only after upstream inspection, classification, and prior validation.

## Recommendations for maintainer decision

1. **Keep four core merge gates:** `Validate Packy-owned code`, the exact-floor
   Claude package smoke, Governance authorization, and CodeQL. They protect
   distinct product, policy, and security boundaries and currently finish in
   about seven minutes or less.
2. **Remove Addy from the universal merge boundary.** `Addy 1.1.0 promotion
   gate` and its separate trusted collector are capability-specific promotion
   machinery, not baseline correctness for every Packy change. Either retire
   both workflows or run them only for Addy promotion/affected paths. This
   requires an atomic branch-protection and expected-governance-contract
   update; deleting YAML alone would deadlock protected `main`.
3. **Stop requiring the advisory Dependency review context.** Its current
   `warn-only` plus `continue-on-error` contract cannot reject a vulnerable
   dependency change. Keep it as cheap advisory evidence, or make it genuinely
   blocking; a required-always-green context adds policy surface without
   enforcement.
4. **Preserve the split Security workflows**, but rename their workflow display
   names to “Security (PR)” and “Security (main)” if operator clarity is worth
   a tiny YAML change. Keep required job context names stable unless branch
   protection is changed atomically.
5. **Disable Synchronize pack source before delivering another one-off repair.**
   Preserve its domain code and tests, but stop production dispatches while a
   replacement boundary is designed. A useful target is two independent,
   narrow content/provenance validations—read-only preparation and
   protected-main publication—rather than five executions of the entire
   repository suite. The final write adapter must be exercised effect-free
   before merge, including file-backed PR bodies. Accepted ADRs 0009, 0016, and
   0017 currently require the repeated boundaries, so this redesign needs a
   superseding ADR rather than an incidental YAML optimization.
6. **Use ordinary issue-bound PR delivery as the temporary Pack-source path.**
   Generate and validate the proposed bundle in a sandbox, then let normal PR
   CI and human review protect integration. This is less automated but avoids
   another day of sequential production-only discovery.
7. **Measure billable job minutes before deleting other coverage.** This audit has
   workflow elapsed time only. Export job-level runner type and billed minutes,
   especially for CI and Release macOS matrices, before making a cost decision.
8. **Retain canary, drift, Release, and both Security schedules for now.** Their
   purposes are not duplicated by required PR checks, and the available sample
   is too small to justify removal. Reassess after enough scheduled history
   exists to quantify issue signal and false positives.
9. **Do not compare old and current Release durations as one population.**
   Establish a new baseline after the current build-once/evidence workflow has
   several non-dry-run publications.

## GitHub-managed Pages deployment

The managed `pages-build-deployment` workflow is not checked in, but it is an
active Actions surface and currently creates avoidable operational backlog.
GitHub Pages serves the published schema URLs successfully, yet the live Pages
state reports `errored`, and the latest deployment is waiting for approval in
the `github-pages` environment. That environment requires the repository Owner
to approve every deployment from already-protected `main`; newer pushes cancel
older waiting deployments.

Keep Pages because Packy's schema `$id` values and ADR 0011 use its stable URLs,
but remove the redundant deployment reviewer after explicit Owner approval.
Protected `main` and the Pages branch policy already constrain the source. If
deployment frequency remains noisy, replace legacy branch publishing with a
checked-in Pages workflow filtered to schema/publication paths.

## Reproduction evidence

The primary read-only commands used were:

```sh
gh api repos/yersonargotev/packy/actions/workflows
gh api repos/yersonargotev/packy/actions/workflows/<id>/runs?per_page=10
gh run view <run-id> --repo yersonargotev/packy --json jobs
gh run view <run-id> --repo yersonargotev/packy --log-failed
gh api repos/yersonargotev/packy/branches/main/protection
gh api repos/yersonargotev/packy/rulesets?includes_parents=true
gh api repos/yersonargotev/packy/commits/main
```

The live API reported all nine checked-in workflows active, plus the
GitHub-managed `pages-build-deployment` workflow described above.

## Limitations and risks

- Ten runs is a deliberately small recent window; only six canary runs and one
  governance-drift run exist.
- Workflow elapsed time includes queue delay and parallelism and therefore
  cannot answer billing cost precisely.
- Run logs show the immediate failing command, not always the ultimate root
  cause; transient-looking filesystem failures are labeled as observations,
  not proven diagnoses.
- Branch protection and workflow state are live external state and can change
  after the research timestamp.
- The repository was active during the audit. This note is anchored to the
  stated `main` commit and API observations at collection time.
