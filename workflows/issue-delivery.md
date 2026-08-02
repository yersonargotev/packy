# Packy issue delivery

Status: Active

## Goal

Deliver one approved Packy issue through the repository's conventional
protected pull-request path, preserving the operator checkout and producing a
traceable GitHub record of authority, validation, review, integration, and
cleanup.

## Source contracts

- `docs/adr/0023-use-conventional-issue-delivery.md` owns the architectural
  decision to use one conventional issue workflow.
- `docs/agents/issue-tracker.md` owns issue-tracker and authorization
  conventions.
- `scripts/validate-packy.sh` is the repository validation authority.
- Release publication remains outside this workflow and follows
  `workflows/packy-release.md`.

## Skill shape

Implement this workflow as the project-local model-invoked skill
`.agents/skills/deliver-issue/SKILL.md`. Its input is exactly one GitHub issue
number. The skill reads this workflow as its orchestration source of truth and
composes the repository's existing diagnosis, implementation, test, and
two-axis review capabilities where applicable; it does not copy their internal
instructions or modify generic skills.

The primary agent owns qualification, the candidate, adjudication, commits,
GitHub mutations, merge, and final verification. It may delegate bounded
read-only investigation, focused implementation slices, and independent review
axes when repository delegation policy permits, but it never delegates final
authority or external-state ownership.

## Trigger

The user explicitly requests delivery of one numbered GitHub issue. The issue
must already carry `status:approved`; applying that label alone never starts a
run. The trigger names exactly one issue and does not authorize unrelated issue
work.

The explicit request authorizes the workflow to implement, push, open and
maintain the pull request, wait for required CI, merge through branch
protection, verify issue closure, and clean up its owned branch and temporary
state. No second routine confirmation is required.

## Checkpoints and exceptions

There is no routine checkpoint after the trigger. The workflow pauses with one
decision-ready exception brief only when safe continuation requires a product
or scope decision, broader authority, a change to the approved acceptance
criteria, an irreversible external effect outside repository integration, or
an override of a required repository control. Technical failures that can be
diagnosed and repaired within the approved issue remain autonomous.

## Workspace isolation

At startup, record the operator checkout's branch, HEAD, worktree status, and
fetched `origin/main`. Use the operator checkout only when it is clean, on
`main`, and exactly synchronized with `origin/main`. Otherwise create a
temporary worktree from the fetched protected-main commit and perform all
issue work there.

Never stash, reset, clean, discard, overwrite, or commit pre-existing operator
changes. When using a temporary worktree, require the operator checkout to
retain its exact recorded branch, HEAD, and status. When the clean operator
checkout is selected for work, return it to clean `main` and fast-forward it to
the verified merged result. Remove only workflow-owned temporary state after
the merge and remote-state verification succeed.

## Qualification

Fetch the named issue, its labels, comments, open dependencies, and referenced
normative material. Require `status:approved`, one clear objective, verifiable
acceptance criteria, explicit limits, and enough dependency and prerequisite
state to implement without inventing intent. Confirm that no other open pull
request or active owned branch already delivers the same issue.

Read the repository instructions, relevant accepted ADRs, domain docs, and
validation contract before changing files. For a reported bug whose cause or
reproduction is uncertain, diagnose it before planning implementation.

The workflow may collect facts and post factual diagnostic evidence, but it
must not silently rewrite the approved objective, criteria, limits, or
authority. Missing or materially ambiguous authority produces an exception
brief naming the exact gap and a recommended resolution.

**Complete when:** the exact issue authority, implementable acceptance surface,
dependencies, relevant repository contracts, and starting commit are known,
and no competing delivery exists.

## Local implementation loop

Create one short issue branch from the qualified starting commit. Plan an
acceptance matrix that maps every issue criterion to its owning implementation
seam and proof. Implement one coherent delta at a time, using test-driven
development where the changed behavior has a practical test seam.

For each delta:

1. implement the bounded change;
2. run formatting, `git diff --check`, and the smallest relevant tests or real
   commands with user paths sandboxed where applicable;
3. commit the coherent delta.

Local Standards and Spec review is proportional, not universal. Run it over
the commits since the preceding accepted review boundary when a delta is large,
complex, sensitive, crosses multiple owning seams, or would make delayed
feedback costly. Adjudicate every finding and repair accepted findings before
continuing. Small, well-bounded deltas may proceed with focused proof alone.

Do not rewrite pushed history. Before the first push, locally reviewed repair
commits may remain separate; preserving review boundaries is preferred over
collapsing evidence into one large commit.

After every acceptance criterion has implementation and focused proof, run
`./scripts/validate-packy.sh` once with sandboxed `HOME` and
`XDG_CONFIG_HOME` on the stable exact HEAD.

Build the actual Packy binary from that same HEAD into a temporary location with
`go build -o <temporary-directory>/packy ./cmd/packy` and run that exact binary
with disposable `HOME` and `XDG_CONFIG_HOME`. Exercise at least one end-to-end
scenario derived from the issue's acceptance criteria and verify its exit
status, observable output, and filesystem effects. Keep generated state outside
the repository and real user configuration.

When the issue changes no CLI behavior, still build the binary and run its
`version` command as a sanity check, then manually verify the actual changed
surface instead of inventing an unrelated CLI scenario. This workflow never
treats a manual check as authority for publication, real-user configuration,
or another irreversible external effect.

**Complete when:** every criterion is implemented and proved, every required
proportional local review is clean, the working tree is clean, and canonical
repository validation plus the applicable manual verification pass on the exact
candidate commit.

## Final candidate proof

Whenever a later phase requires repeating **final candidate proof**, run focused
proof for the changed surface, the applicable manual verification, canonical
repository validation, remote CI, and both complete final review axes on the
new exact HEAD. No result bound to an earlier candidate satisfies this
invariant.

## Pull request and final review

After local proof succeeds, push the issue branch without rewriting history and
open one ready pull request to `main`. Its body uses a GitHub-recognized closing
keyword for exactly the approved issue and summarizes the implementation,
acceptance evidence, canonical validation, and manual verification. Do not open
a draft PR merely to continue unfinished local implementation.

Run required CI and two independent final review axes in parallel. Standards
reviews the complete candidate against repository instructions, accepted ADRs,
domain conventions, and the documented smell baseline. Spec reviews the same
complete candidate against the approved issue and every referenced normative
source. Both bind their result to the exact pull-request HEAD.

The primary agent adjudicates every finding. Accepted findings are repaired in
new commits; pushed history is never rewritten. Any candidate change requires
final candidate proof on the new HEAD.

Publish one compact pull-request comment for the final HEAD containing its SHA,
the Standards and Spec results, acceptance-criteria coverage, canonical
validation result, and manual-verification result. Link detailed evidence or
logs instead of copying raw output.

**Complete when:** the ready pull request names and closes exactly the approved
issue, required CI and both final review axes pass on its unchanged HEAD, all
accepted findings are repaired, and the durable review comment records the
final evidence.

## Check interpretation

Every required branch-protection check must complete successfully on the final
pull-request HEAD. Inspect every advisory check as well. A purely operational
or infrastructure failure in an advisory check does not become an implicit
merge gate, but any substantive correctness, dependency, or security finding
is adjudicated like a review finding and must be repaired or explicitly
accepted by the authority that owns that boundary before merge.

## Freshness and merge

Immediately before merge, fetch and reobserve the issue, pull request, base,
head, reviews, conversations, checks, and branch-protection merge state. Require
the issue to remain approved, materially unchanged, and unblocked; the
pull-request HEAD to equal the reviewed and validated SHA; every required check
to pass; every substantive advisory finding to be resolved; and all review
conversations to be resolved. A material authority edit invalidates
qualification and returns the run to that phase.

If `main` advances but GitHub still reports the pull request mergeable and does
not require an update, do not create an unnecessary synchronization commit. If
branch protection requires the branch to be current, merge the fetched
`origin/main` into the issue branch. Never rebase or force-push published
history. Treat the resulting commit as a new candidate and repeat final
candidate proof.

Merge through branch protection using a merge method currently enabled by the
repository. Do not bypass a gate, use administrator override, or merge a SHA
other than the final evidenced pull-request head.

**Complete when:** GitHub reports the pull request merged through protection
and its merge commit contains the exact final candidate head.

## Verify and clean up

After merge, read the pull request and merge commit back from GitHub and verify
that the merge contains the exact final candidate. Require the approved issue
to be closed by the pull request. If GitHub did not apply the recognized closing
keyword despite the verified merge, close the issue explicitly with a link to
that pull request.

Delete only the workflow-owned remote and local issue branch after verified
merge. Remove its temporary worktree and disposable test state. Recheck that
the operator checkout retains its exact recorded branch, HEAD, and status when
a temporary worktree was used. When the operator checkout was the selected
workspace, return it to clean `main` and fast-forward it to the verified
protected-main result without discarding any unexpected change.

Produce a success brief containing links to the issue and pull request, the
final candidate and merge SHAs, required CI, final Standards and Spec review,
manual verification, issue closure, branch deletion, temporary-state cleanup,
and operator-state preservation. A cleanup failure leaves the run incomplete
and is retried only against resources whose ownership is still unambiguous.

**Complete when:** merge and issue closure are verified, workflow-owned local
and remote resources are removed, operator state is preserved, and the success
brief supports every completion claim.

## Resume and ownership

Git and GitHub are the only durable run state; this workflow creates no custom
ledger, evidence schema, or hidden lifecycle store. Use the deterministic branch
shape `codex/issue-<number>-<short-slug>`.

At every trigger, observe the issue, local and remote branch, pull request,
commits, checks, reviews, and merge state before acting. Adopt one uniquely
matching existing branch or pull request whose identity and history are
compatible with the named issue, then continue from the earliest missing or
invalid proof. Never create a second branch or pull request for the same
delivery.

Unexpected competing identities, incompatible history, unclear ownership, or
multiple matching pull requests produce an exception brief. The workflow does
not overwrite human-authored content, force-push, close a competing pull
request, or guess which identity should win.

## Retries and failures

Diagnose every failed local command, CI check, or GitHub operation before
retrying it. Retry an unchanged-HEAD operation automatically at most once and
only when observable evidence classifies the failure as transient. A second
transient failure produces an exception brief rather than an unbounded rerun.

A deterministic failure is never retried unchanged. Repair it within the
approved scope in a new commit and invalidate candidate-bound proof as described
above. Permission failures, branch-protection rejection, conflicting remote
state, and ambiguous ownership are decision-ready exceptions, not transient
errors.

## Surface-owned assurance

Every issue follows this same workflow; there is no repository-wide delivery
risk profile. During qualification and after every scope change, identify the
policies owned by the affected security, migration, governance, publication, or
other sensitive surface. Add only the specialist proof or review those policies
explicitly require, and bind it to the same final candidate HEAD.

When no owning policy requires specialist assurance, the normal acceptance
proof, manual verification, canonical validation, required CI, and final
Standards and Spec review are sufficient. Issue delivery never grants authority
to execute a release, real migration, production configuration change, or
other external irreversible effect merely because its repository code merged.

## Communication and briefs

Do not post progress comments to the issue. The pull-request body carries the
implementation and proof summary; one final evidence comment records the exact
reviewed HEAD. Respond normally to actual review conversations, but do not emit
periodic status comments or raw logs.

Present exception briefs to the user in the active conversation. Update the
GitHub issue only after a confirmed decision materially changes its durable
authority, scope, criteria, dependency state, or resolution. An exception brief
states the blocking fact, affected phase, evidence link, available choices, and
one recommended answer. A success brief uses the compact contents defined in
**Verify and clean up**.

## Definition of done

A run succeeds only when **Verify and clean up** is complete. The workflow is
not complete at commit, push, pull-request creation, green CI, review approval,
or merge alone. A decision-ready exception brief pauses an incomplete run until
the user resolves the missing authority; an implementer must then resume from
the earliest phase invalidated by that decision.
