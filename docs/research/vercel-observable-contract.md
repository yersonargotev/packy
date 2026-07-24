# Vercel observable contract and exclusions

## Decision question

Which inventoried Vercel capabilities are mandatory, optional, degraded, or
excluded in the first atomic observable contract on Codex, OpenCode, and
Claude Code, and what user-visible behavior governs every exclusion,
degradation, prerequisite, and unavailable mode?

This decision consumes the pinned
[upstream inventory](vercel-upstream-inventory.md) and
[three-surface mapping](vercel-capability-surface-mapping.md). It plans the
contract; it does not implement, synchronize, activate, publish, or release
the `vercel` pack.

## Contract boundary

The first `vercel` contract requires all nine dependency-closed skills:

| Pack resource | Public skill name |
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

Every selected skill retains its complete non-ZIP directory tree, including
required rules, references, scripts, libraries, schemas, playbooks, metadata,
generated package-local `AGENTS.md`, and file modes. A package-local
`AGENTS.md` remains inert inside its on-demand skill tree and is never promoted
to repository or global instructions.

`web-design-guidelines` and `writing-guidelines` are mandatory rather than
optional or excluded. Their current loaders fetch moving `main` content from
separate repositories, so publication remains blocked until the effective rule
sources are independently pinned, licensed, and packaged as reproducible
dependencies. The first contract never claims that a pinned loader alone makes
either capability reproducible or offline-complete.

## Required surface projections

All nine skills have native complete-tree bindings on each surface:

| Surface | Required projection | Invocation |
| --- | --- | --- |
| Codex | Nine native skills | `$name` |
| OpenCode | Nine native skills | Native skill tool with `name` |
| Claude Code | Nine native skills | `/name` |

There are no optional skills, surface exclusions, or binding-level
degradations. Host invocation syntax differs without changing the logical
contract.

Native projection means Packy can reconcile the inert skill tree into the
host's supported location. It does not prove that the host loaded the skill,
that an external tool is installed, that a Vercel account is authenticated, or
that the operator authorized an invocation-time effect.

## Invocation-time requirements and modes

Tools and external state used by a skill are requirements of the affected
invocation or mode, not activation prerequisites for the nine-skill pack.
They include:

- Node.js 20+ and Vercel CLI 53+ where `vercel-optimize` requires them;
- Vercel authentication, project linkage, service metrics, and entitlements
  such as Observability Plus;
- network and process execution;
- Git inspection, commits, and pushes;
- token and environment inspection;
- package installation or package-manager execution;
- consumer-project reads and writes;
- environment, domain, or project mutation;
- uploads and preview or production deployments; and
- subagent investigations.

Packy preview, status, and evidence must expose these facts in structured,
per-capability and, where needed, per-mode form. Each disclosure names:

1. the affected skill and mode;
2. tools and minimum versions;
3. authentication, linkage, service, or entitlement prerequisites;
4. possible authorities and effects;
5. the verified fallback, if one exists; and
6. the behavior when a prerequisite is unavailable or unverified.

The current pack-wide optional-mode and unversioned tool vocabulary cannot
express that precision. The execution-ready specification must therefore
include the required schema and domain evolution; it must not misrepresent a
resource-scoped prerequisite as a requirement of every skill.

## Fallback and failure policy

Packy never invents a fallback, deletes prompt behavior, simulates support, or
reports a simulated external effect as success.

A fallback is admissible only when the workflow already defines it and
validation proves that it preserves the requested logical outcome. In
particular:

- `vercel-optimize` may replace subagent fan-out with sequential investigation
  when it preserves the same investigation objective and output contract; and
- `deploy-to-vercel` may use only its included, declared alternative routes.

If a requested mode lacks an indispensable tool, version, authentication,
linkage, entitlement, metric source, network permission, or mutation
permission, that invocation stops before effects and names every missing or
unverified prerequisite. Other skills and modes remain available.

Acquisition, synchronization, packaging, validation, activation, update,
reconciliation, status, and readiness observation never execute upstream
scripts or skill-directed runtime authority: they do not authenticate, link
consumer projects, request tokens, inspect service data, modify consumer
projects, perform skill-directed Git operations, or deploy. This restriction
does not prohibit Packy's inert Git acquisition, exact-source inspection, or
maintainer-owned branch and pull-request publication workflow.

## Secret boundary

Tokens and secret environment values never appear in:

- preview or status output;
- plans, confirmations, or pending actions;
- validation or readiness evidence;
- logs, errors, or operational artifacts;
- displayed command lines; or
- fingerprints from which the value can be recovered.

Packy may report only presence, absence, or redacted identity. An authorized
invocation may pass a credential through a secure process mechanism, but Packy
must not copy it into generated content, persist it, or convert it into
observable command text. Upstream examples containing token-bearing command
lines do not weaken this rule.

## Source exclusions

The following are excluded from consumer projection:

- the five ZIP files directly under upstream `skills/`;
- `skills/deploy-to-vercel/Archive.zip`; and
- source-maintainer build tooling, tests, evaluations, CI configuration,
  catalog configuration, and other repository material outside the nine
  selected capability trees.

The six archives are duplicate distribution artifacts, and several are stale
relative to their same-commit directories. They never override or supplement
the selected directory bytes. Source-maintainer material is not a consumer
capability and grants no runtime authority.

Preview and evidence list these source exclusions explicitly. They are not
surface exclusions and do not make compatibility degraded.

## Coherence, compatibility, and readiness

Coherence is atomic per surface. A surface can activate only when Packy can
plan all nine mandatory projections, close every packaged dependency, preserve
every required public name under the separately decided conflict policy, and
represent every requirement and effect truthfully.

Any missing, incompatible, collided, or dependency-incomplete mandatory skill
blocks activation on that surface as a whole. Packy never activates a subset
of eight or fewer skills. A blocked Codex projection does not invalidate an
otherwise coherent OpenCode or Claude Code projection.

Compatibility and readiness remain distinct:

- **complete compatibility**: all nine resources have native bindings and
  there are no surface exclusions or binding degradations;
- **configured**: all nine Packy-owned projections are reconciled;
- **authorized**: required host trust and loading permissions are satisfied;
  and
- **usable**: current evidence shows the host discovered and loaded all nine
  skills.

The pack may be usable while a particular invocation mode is `unavailable` or
`unverified`. Per-mode availability is reported separately as `available`,
`unavailable`, or `unverified`, with its reason and fallback. Missing
invocation-time prerequisites never hold the whole pack below `usable`. If the
host cannot discover or load any mandatory skill, the complete projection may
remain configured but the surface cannot be reported usable; Packy does not
hide that failure by reporting a partially loaded subset.

Implementation and publication remain blocked until authoritative
redistribution terms cover the complete contract. Publication is stricter than
one-surface activation: the first selectable pack additionally requires the two
external guideline rule sets to be reproducible and licensed and independent
acceptance evidence to prove the mandatory contract on Codex, OpenCode, and
Claude Code.

## Consequences for later tickets

- Source and compatibility policy must resolve immutable identity, licensing,
  and packaging for the two external guideline rule sets without weakening the
  nine-skill promise.
- Naming and composition policy must protect all nine public names and block
  unmanaged or incompatible collisions atomically.
- The activation prototype must show the complete per-surface contract,
  source exclusions, structured invocation requirements and authorities,
  redaction, per-mode availability, fallbacks, and pre-effect failures.
- The validation matrix must prove exact counts and trees, native discovery and
  loading on all three hosts, inert lifecycle behavior, secret redaction,
  source exclusions, atomic blockers, and negative cases for missing
  prerequisites and simulated success.
- The execution-ready specification must include per-resource and per-mode
  requirement vocabulary rather than using pack-wide activation requirements.

## Answer

The first `vercel` observable contract is nine mandatory, dependency-closed
skills with native bindings and no surface exclusion or binding degradation on
Codex, OpenCode, and Claude Code. Each surface activates atomically, while
surface failures remain independent. External tools, accounts, entitlements,
permissions, and effects are invocation-scoped and disclosed structurally;
only verified coherent fallbacks are allowed, missing indispensable
prerequisites stop only the requested mode before effects, and secrets remain
fully redacted. Six duplicate ZIPs and source-maintainer material are excluded
from projection. Publication remains fail-closed until licensing,
reproducibility, and independent three-surface acceptance are complete.
