# Evidence: generic issue-delivery Pack viability

This note records the primary-source evidence used by [issue
654](https://github.com/yersonargotev/packy/issues/654). It supports a later
admission decision; it neither registers a Pack Source nor materializes any
upstream content.

## Exact upstream candidate

The canonical upstream is
[`yersonargotev/issue-deliver-pack`](https://github.com/yersonargotev/issue-deliver-pack).
Its published [`1.0.0` release](https://api.github.com/repos/yersonargotev/issue-deliver-pack/releases/tags/1.0.0)
is a non-draft, non-prerelease immutable release whose target is exactly
[`0534b2af6c164d56bc8a95a81758749a721d29ae`](https://github.com/yersonargotev/issue-deliver-pack/commit/0534b2af6c164d56bc8a95a81758749a721d29ae).
The release describes the two generic skills, their structural/policy checks,
and CI. Its commit tree contains the selected roots `LICENSE`,
`deliver-issue`, and `setup-issue-delivery`, plus the excluded roots `.github`,
`.gitignore`, `AGENTS.md`, `README.md`, and `scripts`. [GitHub commit tree,
`0534b2af`](https://api.github.com/repos/yersonargotev/issue-deliver-pack/git/trees/0534b2af6c164d56bc8a95a81758749a721d29ae?recursive=1)

The upstream [README at that exact
commit](https://github.com/yersonargotev/issue-deliver-pack/blob/0534b2af6c164d56bc8a95a81758749a721d29ae/README.md)
identifies `deliver-issue` as the execution workflow and
`setup-issue-delivery` as the creator of the consumer-owned policy. It requires
each installed skill to be projected as a sibling directory with its complete
tree; it further says that a consumer must not install the setup skill without
the delivery skill. This is evidence for the candidate identity and intended
scope, not a replacement for Packy's admission contract.

## Generic core and repository policy

The generic core owns the reusable, repository-neutral delivery sequence:
explicit request for one approved issue; qualification; implementation and
proof; protected pull-request review; protected merge; closure; and cleanup.
The upstream [`deliver-issue` skill at
`1.0.0`](https://github.com/yersonargotev/issue-deliver-pack/blob/0534b2af6c164d56bc8a95a81758749a721d29ae/deliver-issue/SKILL.md)
defines that sequence and requires a repository-owned policy before Git or
GitHub state is changed.

Repository policy owns the local decisions that the generic core deliberately
does not know: approval label, tracker conventions, validation commands,
required CI, review criteria, merge method, cleanup rules, and the repository's
architecture and domain instructions. The upstream [`setup-issue-delivery`
skill at `1.0.0`](https://github.com/yersonargotev/issue-deliver-pack/blob/0534b2af6c164d56bc8a95a81758749a721d29ae/setup-issue-delivery/SKILL.md)
limits its write to `docs/agents/issue-delivery.md` and leaves those decisions
to the consumer.

For Packy, that boundary maps to the existing
[repository policy](../../../workflows/issue-delivery.md),
[GitHub tracker rules](../../agents/issue-tracker.md),
[domain-document routing](../../agents/domain.md), and the current
[single-source admission ADR](../../adr/0032-admit-single-source-packs-atomically.md).
The generic Pack must not become a second Packy workflow, rewrite the existing
project-local delivery skill, or carry Packy-specific approval and validation
policy upstream. Conversely, Packy retains authority over admission
configuration, the reviewed snapshot and notice, any future manifest/catalog
entry, and immutable Pack history.

## Codex materialization boundary

Codex treats a skill as a directory containing `SKILL.md` and optional
resources; the `name` and `description` metadata are required. It discovers
repository skills by scanning `.agents/skills` from the working directory to
the repository root, and does not merge duplicate skill names. [OpenAI,
*Build skills*](https://developers.openai.com/codex/skills/)

Therefore a Pack admission can materialize only the reviewed complete trees as
separate `deliver-issue` and `setup-issue-delivery` skill directories in the
Codex repository-skill surface. It must preserve their root identities and
relative references. It cannot safely flatten the two trees, partially project
a referenced subtree, merge either skill into Packy's `AGENTS.md`, or translate
the generic workflow into repository policy: each operation would change the
skill layout or ownership asserted by the upstream source. The setup skill's
own write restriction further means that executing it is distinct from Packy's
admission of reviewed content.

## GitHub controls remain repository-owned

GitHub branch protection can require status checks to pass and be up to date,
require pull-request review, require resolved conversations, and prevent force
pushes or deletion. [GitHub Docs, *About protected
branches*](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)
The upstream's live [`main` protection
representation](https://api.github.com/repos/yersonargotev/issue-deliver-pack/branches/main/protection)
shows a required, strict `validate` check, stale-review dismissal, resolved
conversations, and disabled force pushes and deletion.

Those controls prove that the upstream release was produced under its own
repository governance; they do not transfer merge authority to Packy or to a
skill. Packy's policy independently requires approved issue authority,
ordinary CI and CodeQL, two final review axes, resolved conversations, and a
human merge through protection. See its [qualification](../../../workflows/issue-delivery.md#qualification),
[pull-request review](../../../workflows/issue-delivery.md#pull-request-and-final-review),
and [freshness-and-merge](../../../workflows/issue-delivery.md#freshness-and-merge)
contracts. An admitted generic skill can invoke that policy; it cannot weaken,
bypass, or replace it.

## Viability conclusion and invalidation

The exact `1.0.0` candidate is viable as an independent canonical source for
the two generic skills: it has a stable immutable release, a complete
Codex-compatible sibling-directory shape, an explicit generic/policy boundary,
and upstream protected-branch controls. A future legal-admission record must
bind this exact commit, the upstream README identity, license rights and notice
obligations, the three selected roots, and all five exclusions before Packy
can redistribute or project it.

This conclusion is invalid if the candidate commit or release identity, selected
or excluded scope, README or license identity, skill directory layout, Codex
materialization model, upstream governance evidence, or the repository-policy
boundary changes. A changed candidate requires fresh source, legal, and
admission review rather than carrying this finding forward.
