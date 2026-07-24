# Vercel source identity, versioning, licensing, and compatibility policy

## Decision question

What exact-commit selection, redistribution disposition, Pack SemVer,
compatibility floor, migration, provenance, and manual synchronization rules
make a Vercel proposal decision-ready without consuming moving upstream state?

This decision consumes the pinned
[upstream inventory](vercel-upstream-inventory.md), the
[three-surface mapping](vercel-capability-surface-mapping.md), and the decided
[observable contract](vercel-observable-contract.md). It plans policy; it does
not implement schemas, register or synchronize sources, activate the Pack, or
publish a release.

## Independent identities

The policy keeps these identities separate:

1. the logical Pack `vercel`;
2. the three configured Pack Sources that contribute to it;
3. each source's exact acquired candidate and retained provenance;
4. the Packy-owned `vercel` Pack version describing the observable contract;
5. the Pack Source schema-suite version; and
6. the Packy application version.

An upstream frontmatter version, catalog version, branch name, archive name,
Pack version, or schema-suite version grants no authority to another namespace.

## Exact composite source identity

The first complete contract has three exclusively owned sources:

| Pack Source | Repository | First-candidate disposition |
| --- | --- | --- |
| `vercel-agent-skills` | `vercel-labs/agent-skills` | Pin `7c180d9044c9ae2b442b567aad4e42a28dd5ed62`. |
| `vercel-web-interface-guidelines` | `vercel-labs/web-interface-guidelines` | Select one full commit only after exact inventory and licensing evidence exist. |
| `vercel-writing-guidelines` | `vercel-labs/writing-guidelines` | Select one full commit only after exact inventory and licensing evidence exist. |

The effective candidate is the ordered set of all three exact commits, not only
the loader repository commit. The two guideline sources replace their loaders'
moving `main/command.md` reads with Packy-packaged, reproducible dependencies.
Until both exact secondary candidates are selected and admitted, no complete
first candidate exists.

`main` may be observed only to discover a candidate for human inspection. A
branch, floating ref, abbreviated SHA, generated ZIP, frontmatter version,
`skills.sh.json` value, or other catalog metadata is never synchronization
authority. A future refresh explicitly supplies a new full commit for every
changed source and retains the exact commit for every unchanged source.

Every selected candidate must prove repository and owner identities, commit,
tree, parents and verification evidence, acquired archive digest, selected
paths and file modes, per-file and complete snapshot digests, and legal
evidence. A missing or moved identity, candidate regression, discontinuous
provenance, changed bytes, or unverifiable exact commit blocks rather than
being normalized.

## Redistribution disposition

The inspected primary commit is not redistributable by Packy under the
available evidence. Four skills declare `license: MIT` in frontmatter, while
five do not; the root README's one-word `MIT` statement supplies neither a
complete permission text nor a clear notice and scope for every auxiliary
file. Public GitHub provenance and forkability are not a redistribution grant.

Implementation and publication therefore fail closed until authoritative
first-party terms cover:

- all nine selected skill trees;
- every selected script, rule, reference, library, schema, playbook, metadata,
  generated package-local aggregate, and other auxiliary file;
- the packaged bytes from both guideline repositories; and
- exact copyright, license, notice, attribution, and source-offer obligations.

There is no partial four-skill distribution: the observable contract requires
all nine skills atomically. Legal authority is bound to the exact selected
commits and bytes. A later upstream licensing commit triggers fresh candidate
selection and inspection; it is not applied retroactively to an earlier
candidate. External written permission must likewise identify its covered
repositories, material, versions, rights, and obligations.

License and notice material is retained as inert provenance and user-visible
legal metadata. It is validated and displayed where required but never becomes
a host capability or grants runtime authority.

## Pack SemVer and compatibility floors

The first complete observable contract is Pack `vercel` version `1.0.0`.
Upstream commits, frontmatter versions, schema-suite versions, and Packy
application releases do not choose this value.

A synchronization that changes provenance but preserves selected bytes and
observable legal obligations is a Pack no-op. Every selected-byte or
user-visible legal change receives semantic classification even when its text
diff is small.

- **Patch** preserves effective behavior, resources, names, invocations,
  projections, authorities, requirements, availability states, fallbacks,
  exclusions, legal obligations, and mandatory actions.
- **Minor** adds compatible observable behavior, such as an optional mode or
  verified fallback, without migration, new mandatory authorization, or other
  required user action.
- **Major** removes or renames a promised skill; changes an invocation,
  existing workflow, surface projection, or legal obligation incompatibly;
  turns an optional requirement, permission, or authority into a mandatory
  one; weakens a fallback; or otherwise requires migration, authorization, or
  a new mandatory user action.

Adding a selected resource has at least a minor mechanical floor. Removing one
has a major floor. Packy calculates the exact next canonical three-part SemVer
from the engine-owned floor. Classification evidence may raise but never lower
that floor and cannot choose an arbitrary version.

## Compatibility evidence and migration

The introduction from no Pack to `1.0.0` is initial registration and activation,
not a historical migration.

Every later affected change carries evidence bound to the exact three-source
candidate set, sealed plan, repository base, current and proposed Pack
versions, changed observable aspects, classifier identity, and Codex, OpenCode,
and Claude Code consequences. A major decision must include:

- a concrete migration from the active older contract;
- nonempty mandatory human actions;
- effects and risks on all three surfaces;
- handling for aliases, collisions, permissions, authentication, entitlements,
  and external state;
- verification of the resulting contract; and
- recovery behavior that never simulates success.

Patch and minor evidence cannot carry or conceal a migration or mandatory
action; if either is necessary, the change is major. Upstream provenance never
authorizes Packy-owned state mutation or replaces Packy compatibility history.
If any surface lacks a coherent and independently verifiable route, the
proposal is not publishable.

## Source-scoped provenance and atomic registration

Each configured source owns one canonical
`bundle/sources/<source-id>.lock.json` and its complete contribution to Pack
`vercel`. Every portable binding has exactly one source owner. Equal bytes do
not imply shared ownership or ownership transfer.

The complete ordered lock set produces `lock_set_sha256` under ADR 0012. Plans,
classification evidence, validation, publication, and recovery diagnostics
seal both every affected source-lock digest and the complete lock-set digest.
Any source or bundle-generation change makes an older proposal stale.

The first Vercel admission must register all three sources and their closed
contributions in one sealed complete-bundle transaction. Three sequential
partial registrations are forbidden because no intermediate eight-or-fewer
skill contract is valid. This requires a separately decided schema and domain
contract for composite Pack Source registration; it is an implementation
prerequisite, not an exception to existing transaction, recovery, or freshness
ownership.

After registration, one-source synchronization is permitted only when it seals
the complete lock set and revalidates the entire nine-skill Pack. A routine
proposal cannot transfer bindings between sources, leave a moving dependency,
or commit an incomplete contract.

## Manual synchronization admission

Vercel uses the existing manual
**Inspect → Classify → Validate → Publish** operation with no automatic trigger,
Pack-specific shortcut, direct bundle copy, or bootstrap writer.

1. The maintainer supplies the exact three-commit set.
2. **Inspect** resolves and seals identities, continuity, static inventory,
   licensing, dependency closure, selected changes, digests, affected Pack,
   mechanical floor, base, and preconditions. A legal or closure blocker stops
   the operation.
3. **Classify** supplies complete exact-plan evidence for the logical contract
   and all three surfaces through the explicitly selected classification mode.
4. **Validate** independently reacquires all exact candidates in disposable
   roots, applies the complete result, executes no upstream content, and runs
   Packy's complete validation authority.
5. **Publish** reacquires the candidates again, reproduces the exact result
   tree and diff, reruns validation, and freshly observes provenance,
   repository base, configuration, branch, pull-request ownership, and managed
   metadata before its first write.

Initial success is exactly one non-draft, auto-merge-disabled,
human-mergeable pull request on `sync/vercel`. A schema-valid no-op is possible
only for later synchronization of already configured sources. A changed
commit, byte, license fact, base, binding, source lock, lock set, branch, or
pull-request state invalidates decision readiness and requires a fresh Inspect;
evidence is never patched forward.

Acquisition, inspection, classification, validation, publication, activation,
update, status, and readiness observation do not authenticate to Vercel,
execute upstream scripts, fetch moving prompt dependencies, mutate consumer
projects, perform skill-directed Git operations, or deploy. Packy's own inert
Git acquisition and maintainer-owned proposal branch remain allowed.

## Consequences for the Wayfinder

Two questions are now precise and must be resolved before final validation or
the execution-ready specification:

1. inventory and pin the exact complete bytes, provenance, and licensing
   authority of the two guideline rule repositories; and
2. design the schema and domain contract for atomically registering a sealed
   set of Pack Sources in one complete-bundle transaction.

Neither question authorizes synchronization or implementation during this
decision.

## Answer

The Vercel Pack begins at `1.0.0` only from a license-authorized, exact
three-repository candidate set: the pinned primary commit plus separately
pinned guideline commits. Each source has exclusive source-scoped provenance,
while one complete lock-set digest and one atomic initial registration preserve
the nine-skill contract. Pack SemVer follows observable compatibility rather
than upstream metadata; incompatible changes require major migrations and
mandatory actions across all three surfaces. Every candidate enters only
through inert manual Inspect, Classify, Validate, and Publish gates, and any
moving, incomplete, stale, or legally unsupported state blocks before
publication.
