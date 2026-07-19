# Addy observable contract and exclusions

## Decision question

Which capabilities inventoried from `addyosmani/agent-skills` are required,
optional per surface or runtime, or excluded from the first `addy` observable
contract, and when is a Codex or OpenCode projection coherent enough to
activate?

This decision consumes the pinned
[upstream inventory](addy-upstream-inventory.md) and the
[Packy and host mapping](addy-capability-mapping.md). It does not implement,
synchronize, activate, publish, or release the pack.

## Contract boundary

The first `addy` contract is the complete compatible upstream workflow system
**without hooks**. Its portable identity comprises:

- all 24 skills, including `using-agent-skills`;
- every skill-local support file and the seven shared references needed for
  dependency-closed skill content;
- all four personas, modeled as agents rather than global instructions;
- all eight logical commands, modeled once rather than treating the 24
  upstream host projection files as separate capabilities; and
- the exact upstream release and commit identity plus the required MIT notice
  and attribution as bundle metadata.

These elements are one promised workflow graph. A first release containing
only skills, a partial catalog, or commands without their required personas is
not the `addy` contract decided here.

## Required surface projections

| Portable capability | Codex | OpenCode | Contract classification |
| --- | --- | --- | --- |
| 24 skills and their skill-local files | Native skill projection | Native skill projection | Required on both surfaces |
| Seven shared references | Dependency-closed skill assets or an explicit, verified materialization | Same | Required on both surfaces; absence is a blocker |
| Four personas | Native custom-agent projection through a future Packy `agent` intent and adapter | Native agent projection through the same portable intent | Required on both surfaces |
| Eight logical commands | Declared degradation to same-named workflow skills, invoked as `$name`; Packy must not claim `/name` support | Native custom commands preserving `/name` | Required behavior on both surfaces; host syntax intentionally differs |
| MIT notice, attribution, and exact source identity | Retained in the redistributed bundle; no runtime projection | Same | Required bundle metadata, not an activated capability |

The Codex command projection is a declared degradation, not an optional
omission. It is coherent only when the logical name, prompt behavior,
composition, arguments, and user-visible invocation difference remain
observable. OpenCode's native `/name` form is part of its required projection.

## Optional runtime availability

The workflows mention tools and effects whose availability varies by host,
workstation, session, and operator permission. Browser or network access,
package managers, subagents, and commit or deployment authority are therefore
**invocation-time availability**, not global pack activation requirements.

For each such affordance:

1. Packy still projects the complete workflow and reports unavailable modes or
   effects.
2. A documented coherent fallback may run when one exists. For example, a
   quick static web-performance review can remain available without the deep
   browser-backed mode.
3. When no fallback exists, only that invocation fails, with the missing tool
   or permission named explicitly; the rest of the pack remains available.
4. No adapter may silently delete prompt behavior merely because its current
   runtime cannot provide the requested tool.

The `idea-refine.sh` helper is required content of its owning skill, including
its file mode and disclosed filesystem effect. Its execution is not required
for acquisition, activation, or base readiness, and Packy must never execute it
during synchronization or projection.

## Explicit exclusions

### Excluded from the first contract

All four upstream hooks are excluded rather than marked optional:

- the registered Claude `SessionStart` hook; and
- the three unregistered opt-in hook scripts.

They are unnecessary for skill discovery on either target surface, consume
Claude-specific payload and control semantics, and carry context-injection,
filesystem, network, or process effects. A future contract may reconsider them
only as explicit opt-in resources after their event protocol, trust step,
effects, host translation, and validation evidence are specified. Their
source bytes create no projection action, trust prompt, or pending activation
step in this contract.

### Retained only as inert evidence

The following upstream material is excluded from consumer projection and
activation:

- the root `AGENTS.md` and host setup guides;
- plugin and marketplace manifests and the `.opencode/skills` projection
  symlink; and
- evals, validators, tests, CI definitions, and fixtures.

Selected validation material may be retained as inert evidence. Retention for
provenance or validation never grants runtime authority. The MIT license and
attribution are the exception to disposal, not to activation: they must remain
in the bundle but are not projected as a capability.

## Coherence and activation rule

Coherence is **atomic per CLI surface** and semantic rather than file-count
based. A surface is admissible for activation only when Packy can plan every
required projection for that surface, close every content dependency, and
make every declared degradation and pending human action observable.

The following conditions block activation on the affected surface:

- any of the 24 skills or required local/shared assets is absent;
- any of the four personas or eight logical workflows cannot be represented;
- a required name collides and Packy cannot preserve it under the separately
  decided conflict policy;
- OpenCode cannot provide a required native `/name` command;
- Codex cannot provide the declared same-named `$name` workflow skill; or
- Packy knows a required projection cannot be loaded or authorized and cannot
  represent the condition as a truthful pending human action.

Expected syntax asymmetry, an explicit pending trust/reload action, excluded
hooks, or unavailable optional runtime affordances do not make the projection
incoherent. A pack may be **configured** while authorization or host loading is
pending, but it may not be reported **authorized** or **usable** until fresh
surface evidence establishes those stages. Filesystem presence alone is not
usability evidence, consistent with the readiness model in `CONTEXT.md` and
[ADR 0005](../adr/0005-capability-pack-surface-adapter.md).

No surface may activate a subset such as 23 skills, omit `/ship`, or flatten a
missing persona into generic instructions. Failure on one surface does not
invalidate a coherent projection on the other; activation and readiness remain
surface-scoped.

## Consequences for later tickets

- The naming and composition policy must protect the 24 skill names, four
  agent identities, eight logical command names, and the special `/ship` and
  `/webperf` persona compositions without silent overwrite or flattening.
- The activation prototype must show the atomic required set, Codex command
  degradation, excluded hooks, optional runtime modes, collisions, and the
  configured-to-authorized-to-usable progression.
- The validation matrix must prove dependency closure, exact resource counts
  and composition, both host projections, explicit degradation, inert
  exclusions, license retention, and negative cases for partial catalogs and
  missing runtime affordances.
- Current Pack schema and adapters cannot encode this contract. Later
  specification work must account for `agent`, `command`, dependency-asset,
  and notice intent plus both host projections; published schema evolution must
  follow the immutable-suite policy in
  [ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md).

## Answer

The first `addy` observable contract requires the complete dependency-closed
24-skill, four-agent, eight-workflow system on both Codex and OpenCode, with
surface-native projections where available and one explicit Codex command
degradation. Tool-backed modes are optional at invocation and must remain
observable. All hooks and source-maintainer or validation artifacts are
excluded from activation, while selected evidence and required MIT attribution
remain inert in the bundle. A projection activates only as a complete semantic
unit for its surface; silent omissions and undeclared degradation are never
coherent.
