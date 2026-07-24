# Vercel naming, conflicts, and composition policy

## Decision question

How do `vercel`-namespaced portable identities project upstream public names
across Codex, OpenCode, and Claude Code, how are collisions and qualified
aliases handled, and under what evidence may multiple Packs share one visible
projection?

This decision consumes the
[three-surface mapping](vercel-capability-surface-mapping.md), the
[observable contract](vercel-observable-contract.md), and the
[source, versioning, licensing, and compatibility policy](vercel-source-versioning-policy.md).
It plans naming and composition policy; it does not implement model or schema
changes, activate the Pack, synchronize a source, or publish a release.

## Separate naming layers

Every selected Vercel skill has one stable portable Pack identity:

```text
(pack_id=vercel, resource_kind=skill, logical_id=vercel-<capability>)
```

The current manifest spelling is `skill:vercel-<capability>`. The Pack ID,
resource kind, and logical ID establish portable intent; a host path, projected
directory, invocation syntax, public binding, or alias does not.

The nine first-contract identities and default public bindings remain:

| Portable resource | Public binding |
| --- | --- |
| `skill:vercel-composition-patterns` | `vercel-composition-patterns` |
| `skill:vercel-deploy-to-vercel` | `deploy-to-vercel` |
| `skill:vercel-react-best-practices` | `vercel-react-best-practices` |
| `skill:vercel-react-native-skills` | `vercel-react-native-skills` |
| `skill:vercel-react-view-transitions` | `vercel-react-view-transitions` |
| `skill:vercel-cli-with-tokens` | `vercel-cli-with-tokens` |
| `skill:vercel-optimize` | `vercel-optimize` |
| `skill:vercel-web-design-guidelines` | `web-design-guidelines` |
| `skill:vercel-writing-guidelines` | `writing-guidelines` |

The namespaced logical IDs prevent unrelated Pack resources from becoming one
portable resource merely because their public names or bytes happen to match.
They do not force the Pack namespace into every user-facing invocation.

## Default public projections

Each surface preserves the exact upstream public binding when that binding is
free:

| Surface | Native invocation |
| --- | --- |
| Codex | `$<public-name>` |
| OpenCode | native skill tool with `<public-name>` |
| Claude Code | `/<public-name>` |

Invocation syntax is host translation rather than a different logical
capability. Pack activation order, lexical Pack order, source order, matching
bytes, or a more qualified portable identity grants no projection precedence.
Packy never silently overrides or automatically renames an occupant.

## Surface-local collision detection

Every complete surface adapter freshly inspects and normalizes its occupied
namespace before a plan is sealed. The observation covers:

- host-reserved and native names;
- projected paths, directories, filenames, configuration keys, and other
  host-visible targets;
- unmanaged external resources; and
- projections and contributor ownership recorded by Packy.

The adapter owns host path, case, name, and cross-kind normalization. It reports
observed facts but does not choose a winner. Capability-pack compares the
complete desired composition, applies collision and ownership policy, and
produces blockers.

An unresolved collision involving any of Vercel's nine mandatory skills blocks
the complete Vercel activation or update on that surface before effects. It
does not invalidate a coherent Vercel composition on another surface. The
blocker identifies the portable resource, normalized occupied target, observed
owner or reservation, and explicit available resolution paths.

## External ownership boundary

An unmanaged or ambiguously owned host resource remains external even when its
target, tree, modes, and bytes equal Vercel's desired projection. Packy does
not adopt, overwrite, rename, or delete it implicitly.

The operator may remove the external occupant outside the sealed operation or
approve a different surface-local alias. Any ownership-transfer workflow is a
separate explicit capability and cannot weaken inspection, collision, or
cleanup policy for this Pack.

## Explicit qualified aliases

A collision may be resolved by an explicit user-selected alias for exactly one
surface and one portable Vercel identity. Packy may suggest:

```text
vercel-pack-<public-name>
```

The suggestion is never selected automatically. The user may choose another
valid kebab-case name. Packy normalizes and collision-checks the chosen alias
like every other projection; if it is reserved or occupied, the operation
remains blocked until the user chooses a free alias or changes the desired
composition.

The alias is local activation intent. It is sealed into preview, approval,
apply, verification, recovery, status, and update plans. It changes only the
effective surface binding and native invocation; it does not change the Pack
resource identity, source contribution, provenance, selected bytes, or Pack
version, and it never leaks to another surface.

An alias remains attached to its portable logical identity across compatible
updates. Every lifecycle operation freshly revalidates the effective target
and its ownership assumptions. Removal or incompatible replacement of the
logical identity requires an explicit migration rather than an inferred alias
transfer.

## Exclusive first contract

All nine first-contract Vercel bindings remain `exclusive`. Equal public names,
rendered bytes, or complete trees do not permit Vercel to share a projection
with another Pack under version `1.0.0`; the overlap is a collision resolved by
an alias or composition change.

Changing a Vercel binding from exclusive to shared is a future versioned
observable-contract decision. It is admissible only when all of the following
are proven:

1. both exact Pack contracts explicitly and mutually declare the projection
   shareable;
2. the normalized surface projection identity is identical;
3. canonical trees are identical, including paths, structure, bytes, modes,
   and other projection-relevant metadata;
4. observable behavior is identical, including dependencies, requirements,
   authorities, effects, fallbacks, readiness semantics, and failure behavior;
   and
5. Packy records every contributor and the verified projection fingerprint in
   ownership state.

An unmanaged resource cannot participate in implicit sharing. Later divergence
in any precondition blocks update without choosing a winner or overwriting an
occupant. Deactivation removes one contributor and retains the unchanged
projection while another verified contributor remains; destructive cleanup is
available only to the last verified contributor under the ordinary consent
contract.

## Required model and schema consequences

This policy requires a general separation among:

- portable Pack identity;
- per-surface public binding and optional alias;
- normalized observed host occupancy;
- explicit exclusive or shared declarations;
- contributor ownership and verified projection fingerprints; and
- logical capability and observable-behavior contracts used to justify
  sharing.

The current runtime primarily composes resources by `kind:id` and can share
identical declarations when every overlapping binding says `shared`. That
behavior does not by itself satisfy this policy's evidence gate for distinct
namespaced portable identities. Implementation must add or reuse the general
identity-to-projection seam; it must not introduce a Vercel-only path or infer
sharing from byte equality.

Capability-pack remains the sole owner of portable composition, collision,
alias, sharing, ownership, lifecycle, blocker, and cleanup policy. Codex,
OpenCode, and Claude Code adapters own host translation, normalization, fresh
inspection, and authorized application under ADR 0005.

Any Pack Source schema change follows ADR 0011 as a new complete immutable
suite. Existing schema bytes and meanings are not reinterpreted. Source
ownership remains exclusive under ADR 0012 even when runtime projection
contributors are deliberately shared; equal bytes never transfer source
provenance.

## Answer

Vercel's nine stable, namespaced portable skill identities preserve their exact
upstream public names on every conflict-free surface. Fresh normalized
inspection has no precedence or implicit ownership: any reserved, unmanaged,
ambiguous, or incompatible collision blocks the complete affected surface
before effects while leaving other surfaces independent. The user may resolve a
collision only through an explicitly approved surface-local alias, with
`vercel-pack-<public-name>` as a suggestion rather than an automatic choice.
All first-contract bindings remain exclusive. Future sharing requires mutual
versioned declarations, identical normalized projection and canonical tree,
identical observable behavior, and recorded contributor ownership; equal bytes
or external occupancy alone never suffice.
