# Controlled progress counts

The [measurement plan](measurement-plan.md) preceded this run. Run
`python3 -B scripts/probes/release-progress-counts.py` to repeat it. The fake
`gh` runs against eight snapshots, including two release-wait snapshots and
one homebrew-wait snapshot. Its watcher prints synthetic compact state lines.
The reconstructed baseline issues `gh run watch 123 --repo yersonargotev/packy
--compact --interval 45 --exit-status` once, then one `gh run view` and one
pending-deployments `gh api` per snapshot. The manual run output retains the
run ID, tag, SHA, status, and numeric job ID and status. The pending output
retains environment name, ID, and eligibility. The fixture filters these
records before counting output. It does not reproduce the real gh renderer or
historical session.

| Controlled count | Reconstructed baseline | Observer |
| --- | ---: | ---: |
| Progress entry-command invocations | 17 | 1 |
| `gh` progress process invocations | 17 | 11 |
| Progress output bytes | 2,532 | 942 |
| Pending-deployment reads | 8 | 3 |

The entry-command counts are the subprocess calls made by the measurement
script, not Codex tool-call counts. Both paths additionally make two
approval-check entry commands with six `gh`
reads in total. No diagnostic reads occur in this successful scenario. Five executions produced identical counts and passed their completion checks.
The fixed eight-snapshot timeline, including three waiting observations, bounds
this comparison. The counts omit approval-check output. The number of internal HTTP requests is
unknown. The result does not measure elapsed time.
