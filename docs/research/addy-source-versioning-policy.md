# Addy source identity, versioning, and compatibility policy

## Decision question

How should Packy select exact Addy upstream identities, translate upstream
change into Pack versions, classify observable compatibility and migrations,
and admit a synchronization proposal through the existing manual Pack Source
workflow?

This decision consumes the repository-local
[evidence](addy-source-versioning-evidence.md), the pinned
[upstream inventory](addy-upstream-inventory.md), and the decided
[observable contract](addy-observable-contract.md). It plans the policy; it does
not configure, synchronize, activate, publish, or release Addy.

## Identity namespaces

The policy keeps four identities independent:

1. Pack Source `addy`, configured for `addyosmani/agent-skills`;
2. the exact acquired upstream candidate and its retained provenance;
3. the Packy-owned `addy` Pack version describing its observable contract; and
4. the version of any Pack Source schema suite used by synchronization
   artifacts.

An equal-looking number in two namespaces creates no authority between them.
In particular, upstream release `0.6.4`, the host-manifest value `1.0.0`, Pack
version `1.0.0`, and schema-suite `v1.0.0` have separate meanings.

## Source selection and exact identity

Normal Addy refresh intent selects the latest published stable release. The
first known candidate is release/tag `0.6.4` at commit
`98967c45a42b88d6b8fb3a88b7ff6273920763d6`, but the configured selector does
not freeze ordinary future refreshes at that release.

Every inspection must resolve the selected release into an exact candidate and
seal at least the repository and owner identities, release and tag metadata,
the complete tag-to-commit resolution, full commit SHA, tree and parent
identity, verification evidence, acquired archive digest, selected-resource
digests, and complete snapshot digest. The lock and proposal retain those
facts. A moved tag, repository or owner identity, discontinuous provenance,
candidate regression, or changed content blocks rather than being normalized.

`main`, another branch, an abbreviated SHA, a floating ref, and the version
fields inside upstream host manifests are never synchronization authority.
Prereleases require their exact published prerelease tag. Commit selection is
explicit and requires a full lowercase SHA; it is used for an intentional pin,
inspection-bound human evidence, or an exact retry rather than silently
changing normal stable-selection policy.

## Pack version policy

The first complete Addy observable contract is Pack version `1.0.0`. This is a
Packy version, not a relabeling of upstream release `0.6.4` or an adoption of an
embedded host-manifest version.

Later versions classify the change from the active prior Addy observable
contract:

- **Patch** preserves the workflows and expectations of the prior contract.
  It may correct or clarify selected content only when the effective skills,
  agents, workflows, surface projections, invocation forms, requirements,
  fallbacks, and mandatory actions remain compatible.
- **Minor** adds compatible observable behavior. Existing workflows and
  invocations remain available, and the addition introduces no migration,
  newly mandatory authorization, or other required user action.
- **Major** removes or renames a promised skill, agent, or logical command;
  changes an existing workflow or surface projection incompatibly; changes a
  declared degradation such as Codex `$name`; makes a formerly optional mode
  or tool mandatory; or otherwise requires migration, authorization, or a new
  mandatory user action.

Upstream release numbers and textual diff size do not select the Pack version.
Packy owns the mechanical floor and calculates the exact next canonical SemVer;
classification evidence cannot choose an arbitrary version.

## Compatibility evidence and migrations

Every selected upstream change that affects `addy` receives semantic
classification against the prior observable contract in addition to Packy's
mechanical floor. Evidence is complete for the affected Pack and is bound to
the exact plan, base, candidate, current version, proposed version, classifier
identity, and changed observable aspects.

The changed aspects must cover consequences for both Codex and OpenCode,
including required resources, dependency closure, logical composition,
invocation syntax, declared degradation, runtime requirements, exclusions,
and mandatory human actions. A classifier may raise but never lower the
mechanical floor.

A major classification must provide a concrete migration and a nonempty set of
mandatory actions. The migration explains how an active older contract reaches
the new contract without treating upstream provenance as Packy-owned migration
authority. Patch and minor evidence cannot carry or conceal a migration or
mandatory actions; if either is necessary, the result is major.

AI classification is proposed evidence, not maintainer acceptance. Human
classification is an explicit inspection-first flow and must remain bound to
that inspection's exact plan, base, and candidate. Neither mode may invent a
bypass, change modes implicitly, or weaken engine-owned admission.

## Manual proposal admission

Addy uses the existing manual **Inspect → Classify → Validate → Publish** Pack
Source operation. There is no Addy-specific shortcut, including for the
initial `1.0.0` introduction.

1. **Inspect** resolves and seals the exact candidate, provenance, selected
   changes, affected Pack, mechanical floor, base, and preconditions. An exact
   no-op stops here.
2. **Classify** supplies complete evidence for the sealed plan through the
   explicitly selected AI or human mode.
3. **Validate** reacquires and applies the exact candidate in a disposable,
   sandboxed checkout, executes no upstream content, and runs the complete
   Packy-owned validation authority.
4. **Publish** reacquires and applies the exact candidate again, proves the
   expected diff and validated result tree, and freshly reobserves provenance,
   repository base, branch and pull-request ownership, configuration, head,
   and managed metadata before its first write.

Success is either a schema-valid no-op or one open, non-draft pull request that
remains decision-ready for the exact plan, candidate, base, result tree, head,
provenance, and pull-request state. Auto-merge remains disabled and a maintainer
decides whether to merge. Any changed binding invalidates readiness and
requires a fresh Inspect; evidence is never patched forward.

## Required follow-on specification

The no-bypass rule is a required target, not a claim that the current bundle
model can already admit Addy. Current configuration accepts multiple sources,
but production synchronization still owns one repository-global
`bundle/sources.lock.json`. Before the initial Addy proposal can be admitted,
the final specification must define independent per-source provenance and
transaction ownership without weakening the complete-bundle transaction,
freshness, or recovery contracts.

The same specification must introduce the Pack/resource intents and immutable
schema evolution already required by the observable-contract and capability
mapping decisions. Those implementation prerequisites do not authorize a
bootstrap exception for Addy bytes.

## Answer

Packy tracks Addy's latest stable release as normal intent but admits only an
exact, provenance-sealed candidate. The Addy Pack starts at `1.0.0` and evolves
by compatibility of its Packy observable contract, independently of upstream
and schema-suite versions. Every affected change receives floor-respecting,
surface-complete evidence; incompatible changes require a major version,
concrete migration, and mandatory actions. The initial and all later proposals
must pass the unchanged manual Inspect, Classify, Validate, and Publish gates,
ending only in a human-mergeable decision-ready PR or a verified no-op.
