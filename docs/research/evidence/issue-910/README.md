# Issue 910 implementation evidence

Date: 2026-10-30. This is non-normative implementation evidence for
[issue 910](https://github.com/yersonargotev/packy/issues/910) and its
[current Agent Brief](https://github.com/yersonargotev/packy/issues/910#issuecomment-6534339587).
The user authorized implementation and a PR. Merge, auto-merge, and release
publication remain outside this run.

## Changes and ownership

The local delivery gate and workflow are removed. Existing poteto playbooks
consume [repository policy](../../../agents/development.md) at their existing
verification and PR review points. The policy supplies independent Standards
and Spec assignments, candidate identity, repository checks, and integration
authority. It adds no phase sequence or delivery coordinator.

[verify-packy](../../../../.agents/skills/verify-packy/SKILL.md) follows
create-verification-skill. Its five-entry map covers CLI inspection, project
verification, lifecycle preview, the TUI dashboard, and agent guidance.
The existing projection-batch specialist is reused conditionally.
[release-packy](../../../../workflows/packy-release.md) retains publication
ownership and hands corrections to normal development, stopping at the PR.
Resumption requires observed integration, fresh proof, and exact approval.
Pack-owned pstack resources, Catalog content, installed projections, and
publication executables are unchanged.

## Real application exercise

The [initial exercise](verification/README.md) built the actual Packy binary,
ran Doctor, drove dashboard/help/quit in a private PTY, checked absent Packy
state, and confirmed its transcript survived cleanup. That exercise records
its dirty source identity and skill-file hashes. It precedes later skill repairs.

Independent review reproduced a descendant-process leak in the PTY helper and
an unstated working directory. The repaired helper kills only its owned
process group after either leader outcome. The [success and failure controls](verification/helper-cleanup-controls.json)
retain their [rerunnable recipe](verification/check-pty-cleanup.py). Run it
from the repository root after creating `/tmp/packy-910-run`.
Both controls found the descendant absent and retained their transcripts.
The final clean-candidate exercise and independent review records are retained
outside disposable state and identified in the PR evidence summary.

## Controlled agent invocations

The [opt-in probe](../../../../scripts/probes/development-routing.md) launches
real fresh Codex turns. Its inert forge, Git, validation, and artifact boundaries
serve labelled continuation inputs. Prepared review and Shipping verdicts prove
consumption and ordering, not actual review execution. Actual independent
reviews of this change are separate.

Each scenario retains the request, reply, report, actual JSONL tool trace, and
probe and instruction hashes. JSONL traces are losslessly gzip-compressed.
Decode one with `gzip -dc routing/<scenario>/events.jsonl.gz`.
Probe snapshots retain the exact fixture revision used for each group.
Full reports, including instruction manifests, are retained as `report.json.gz`.
Their readable result summaries include the original report digest.
`invocation-manifest.json` names the probe worktree revision, not the final Packy candidate.
Only fixture-owned files change. No real GitHub repository, tag, release, merge,
or operator configuration is mutated by the fixtures.

| Case | Observed boundary | Evidence |
| --- | --- | --- |
| development | PR for human merge after verification and both review inputs. | [result](routing/development/result.json), [tool trace](routing/development/events.jsonl.gz) |
| release-correction | Inherited note scope recorded and approved, PR retained pending integration. | [result](routing/release-correction/result.json), [tool trace](routing/release-correction/events.jsonl.gz) |
| pending | Unmerged correction retained without repeated work or publication. | [result](routing/pending/result.json), [tool trace](routing/pending/events.jsonl.gz) |
| integrated | Observed main integration, fresh proof, v1.2.4/new-commit approval. | [result](routing/integrated/result.json), [tool trace](routing/integrated/events.jsonl.gz) |
| integrated-posttag | Published v1.2.4 retained, fresh v1.2.5/new-commit approval. | [result](routing/integrated-posttag/result.json), [tool trace](routing/integrated-posttag/events.jsonl.gz) |
| shipping | Separate integration request, verifier consumed before protected merge request. | [result](routing/shipping/result.json), [tool trace](routing/shipping/events.jsonl.gz) |
| broader | Unapproved broader scope stops before correction work. | [result](routing/broader/result.json), [tool trace](routing/broader/events.jsonl.gz) |
| posttag | Unapproved post-tag scope stops before correction work. | [result](routing/posttag/result.json), [tool trace](routing/posttag/events.jsonl.gz) |
| competing | Competing PR identities stop duplicate correction work. | [result](routing/competing/result.json), [tool trace](routing/competing/events.jsonl.gz) |

Early fixture runs exposed read-only state-write failures, an incorrectly
accepted failed Shipping call, unknown read endpoints, and a tag-listing false
positive. The retained [fixture repair notes](routing/fixture-repairs.md)
distinguish those failures from candidate routing. Final assertions require
successful tool results, evidence consumption before actions, and structured
mutation checks. Unresolved required evidence cannot pass.

## Validation and review

The [assembly validation](assembly-validation.log) and
[complete Go suite](assembly-go-test-all.log) passed with sandboxed user paths.
Both changed skills passed the host skill validator. Local skill links,
feature headings, Python syntax, executable helper mode, and `git diff --check`
were checked. Exact final candidate results, independent Standards and Spec
reviewer identities, required CI, and CodeQL belong to the PR evidence summary.
No GitHub review approval or merge readiness is inferred from local agent review.

## Decisions and limits

| Principle read this session | Decision it changed |
| --- | --- |
| Subtract Before You Add | Remove the delivery gate and active route instead of wrapping them. |
| Laziness Protocol | Keep repository obligations in one policy consumed by existing stages. |
| Build the Lever | Retain the PTY helper and opt-in model-driven routing probe. |
| Model the Domain | Represent fixture scenarios as data and evaluate observed calls. |
| Separate Before Serializing Shared State | Give implementation slices separate branches, worktrees, and evidence. |
| Prove It Works | Drive the real binary, inspect actual traces, and retain proof after cleanup. |
| Test Behavior, Not Implementation | Remove obsolete prose assertions and test authority decisions through tools. |

The [decision trail](decisions.tsv) records assembly choices. The initial row's
typed timestamp is corrected by a later append-only entry.
The seed map is representative. Catalog-dependent inspection, installation,
and activation journeys were not exercised by the dashboard proof and remain
subject to their documented prerequisites. This task changes agent guidance,
not CLI or domain behavior. Its changed guidance was exercised through the
controlled invocations.
No confirmed reviewer from a different model family was available in the host
configuration. Independent assessments use confirmed host models. No
cross-family panel or unavailable external review is claimed.
