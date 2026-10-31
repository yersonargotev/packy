# Repository-only retention verification

The current candidate delivers #911 inside Packy. It imposes no change on the
Catalog's pstack adaptations. The maintainer requested this narrower ownership
after reviewing the cross-repository proposal.

## Scope decision

Preserving cited evidence is useful beyond Packy, but the proposed mandatory
Python preflight and bounded citation syntax have not been justified for all
pstack users. In particular, upstream `show-me-your-work` includes directory
citations and opaque evidence cells that this checker intentionally rejects.
See the [upstream format and examples](https://github.com/cursor/plugins/blob/1ba91fc25c0977741dd0e267849e6234b037ed0a/pstack/skills/show-me-your-work/SKILL.md).
Passing Packy's acceptance tests does not establish that broader product fit.

The repository owns `scripts/check-evidence-retention.py`, the retention policy,
and a regression invoked by ordinary Go CI. ADR 0039 still owns Pack resources;
this candidate changes no Pack or installed projection. Catalog PR #133 is no
longer a delivery prerequisite for Packy PR #915.

## Checker identity and regression

The local checker has SHA-256
`10e439dfecd02d11ac8217c1aedf6ed4dc20ae19e2010f3eff4f7301262b6386`.
Its bytes match the earlier Catalog candidate; this relocation changes ownership
and acquisition, not retention behavior. The regression runs the local script
and checks partial copies, complete copies with per-artifact hashes, missing
originals and destinations, wrong bytes, destinations in cleanup targets, absent
targets, unsupported citations, traversal, symlinks, aliases, and read-only state.
`TestEvidenceRetentionBeforeCleanup` launches the probe with `testprocess.Env`.

## Current controlled invocations

Four fresh Codex turns consumed the final local instructions and local checker.
Each report records their SHA-256 identities and has no missing observations or
changed source inputs. No Catalog or installed resource was supplied.

| Entry point | Partial retention | Complete retention |
| --- | --- | --- |
| Development | [PASS](development-partial/report.json), [commands](development-partial/commands.json.gz) | [PASS](development-complete/report.json), [commands](development-complete/commands.json.gz) |
| Release | [PASS](release-partial/report.json), [commands](release-partial/commands.json.gz) | [PASS](release-complete/report.json), [commands](release-complete/commands.json.gz) |

Both partial cases exited unsuccessfully and named all five unretained files;
there was no deletion attempt and all six originals remained unchanged. Both
complete cases returned zero before a separate exact-path deletion. Independent
post-run snapshots preserve the six retained copies, trail, handoff, operator
sentinel, unrelated worktree, Git refs, and worktree registrations. Each case
retains its prompt, reply, before/after snapshots, and compressed complete tool
commands beside its report.

Full raw events and validation logs remain outside disposable fixture state at
`/private/tmp/packy-911-local-evidence`. The earlier `routing-*` runs passed too;
the retained `final-*` runs include the complete local relocation instructions.
Fixture integration and release statements remain synthetic inputs. These
journeys do not claim real integration, release, or recovery of historical files.

## Limits

The existing citation subset, live-original requirement, snapshot semantics,
and separate ownership/deletion requirements remain in force. The previous
cross-repository evidence is retained in the parent directory as history.
Independent Standards and Spec assessments and final validation/check status
are bound to the resulting PR HEAD in the PR's current evidence summary.
A different-family trail review and optional dependency analysis are not claimed.
