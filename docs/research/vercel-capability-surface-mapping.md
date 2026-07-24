# Vercel capability mapping across Packy and three CLI surfaces

## Research question

How does every dependency-closed capability at the pinned Vercel source
identity map to Pack resources and native or explicitly degraded projections on
Codex, OpenCode, and Claude Code, and which schema, adapter, or product
prerequisites remain?

## Scope and evidence boundary

This decision uses the immutable source contract documented in
[the Vercel upstream inventory](vercel-upstream-inventory.md): commit
[`7c180d9044c9ae2b442b567aad4e42a28dd5ed62`](https://github.com/vercel-labs/agent-skills/tree/7c180d9044c9ae2b442b567aad4e42a28dd5ed62),
tree `0557b732b3907e51bed3fd7898095f8097a0834e`. It covers the nine
dependency-closed directories and excludes all six ZIP duplicates.

The host facts come from current first-party documentation:

- [Codex skills](https://developers.openai.com/codex/skills/) are complete
  directories with `SKILL.md` plus optional scripts, references, and assets.
  Codex discovers user skills under `$HOME/.agents/skills`, supports symlinked
  skill directories, and invokes a skill explicitly as `$name`.
- [OpenCode Agent Skills](https://opencode.ai/docs/skills/) discovers global
  skills under `~/.config/opencode/skills/<name>/SKILL.md`, loads them through
  its native `skill` tool, and requires the containing directory to match the
  frontmatter `name`.
- [Claude Code skills](https://code.claude.com/docs/en/slash-commands) are
  complete directories under `~/.claude/skills/<name>`, support auxiliary
  files and symlinked personal skill directories, and expose `/name`.

Repository behavior was inspected through Packy's current manifest v3,
complete surface adapters, accepted ADRs, and focused tests. No host CLI,
upstream script, authentication, deployment, package manager, or consumer
project was executed or changed.

## Decision

The first `vercel` Pack contract has **nine Pack `skill` resources**. Each
resource owns one complete upstream directory after the six ZIP artifacts are
removed. Every resource has a native `skill` binding on Codex, OpenCode, and
Claude Code. There is no surface exclusion and no binding-level degradation.

This conclusion is intentionally narrow:

- Native projection means Packy can install the exact inert skill tree at the
  host's supported global discovery path.
- It does not mean the workflow's optional tools, authentication, network,
  external state, or service entitlements are available.
- It does not grant Packy authority to exercise those effects during
  synchronization, validation, installation, activation, or readiness checks.
- It does not make the two live guideline loaders reproducible or authorize
  redistribution.

Those runtime and publication conditions are product prerequisites described
below, not reasons to mislabel a structurally native skill projection as
degraded.

## Portable identity and binding rule

Portable resource IDs are `vercel`-namespaced and stable even where the public
upstream name is not:

| Upstream directory | Pack resource identity | Public binding name |
| --- | --- | --- |
| `composition-patterns` | `skill:vercel-composition-patterns` | `vercel-composition-patterns` |
| `deploy-to-vercel` | `skill:vercel-deploy-to-vercel` | `deploy-to-vercel` |
| `react-best-practices` | `skill:vercel-react-best-practices` | `vercel-react-best-practices` |
| `react-native-skills` | `skill:vercel-react-native-skills` | `vercel-react-native-skills` |
| `react-view-transitions` | `skill:vercel-react-view-transitions` | `vercel-react-view-transitions` |
| `vercel-cli-with-tokens` | `skill:vercel-cli-with-tokens` | `vercel-cli-with-tokens` |
| `vercel-optimize` | `skill:vercel-optimize` | `vercel-optimize` |
| `web-design-guidelines` | `skill:vercel-web-design-guidelines` | `web-design-guidelines` |
| `writing-guidelines` | `skill:vercel-writing-guidelines` | `writing-guidelines` |

Each resource uses the same public binding name on all three surfaces:

| Surface | Projection | Invocation | Mode | Sharing |
| --- | --- | --- | --- | --- |
| Codex | `skill` | `$<public-name>` | `native` | `exclusive` |
| OpenCode | `skill` | `<public-name>` through the native skill tool | `native` | `exclusive` |
| Claude Code | `skill` | `/<public-name>` | `native` | `exclusive` |

The public binding name, rather than the upstream repository directory, becomes
the host-visible installation directory. That makes OpenCode's directory/name
rule true for the five packages whose frontmatter name differs from the source
directory and keeps the same recognizable name on all three surfaces. An
operator-selected alias may later resolve a collision, but it changes only the
surface binding, never the portable resource identity or source tree.

`exclusive` is the safe first declaration. A future contract may declare a
shared projection only after proving another contributor has identical
canonical content and an identical observable contract.

## Capability-by-capability mapping

| Capability | Dependency closure kept inside the skill tree | Three-surface projection | Invocation-time contract |
| --- | --- | --- | --- |
| `vercel-composition-patterns` | `SKILL.md`, rules/support files, metadata, README, generated package-local `AGENTS.md` | Native complete-tree skill on all three surfaces | Reads and edits consumer React source; package-local `AGENTS.md` remains inert inside the on-demand skill and is never promoted to repository/global instructions. |
| `vercel-react-best-practices` | Complete 76-file directory | Native complete-tree skill on all three surfaces | Reads and edits React/Next.js source and may recommend packages or APIs. |
| `vercel-react-native-skills` | Complete 42-file directory | Native complete-tree skill on all three surfaces | Reads and edits React Native/Expo source and may recommend packages or APIs. The different README display name does not create an alias. |
| `vercel-react-view-transitions` | `SKILL.md` plus all four required `references/*.md` and remaining support files | Native complete-tree skill on all three surfaces | Reads and edits React/CSS source; references remain relative and available on demand. |
| `deploy-to-vercel` | `SKILL.md`, `resources/deploy.sh`, and `resources/deploy-codex.sh`; nested `Archive.zip` excluded | Native complete-tree skill on all three surfaces | May inspect Git and `.vercel`, run processes or Vercel CLI, package/upload source, link a project, commit/push, and deploy. Included scripts remain executable but inert until deliberate skill use and host/user approval. |
| `vercel-cli-with-tokens` | Single `SKILL.md` | Native skill on all three surfaces | May inspect token/environment state and use Git/Vercel CLI to link, deploy, inspect logs, or mutate environment/domain state. Tokens must never be copied into lifecycle output. |
| `vercel-optimize` | Entire 156-file application-like tree: `SKILL.md`, `lib/`, `scripts/`, schemas, doctrine, playbooks, and support material | Native complete-tree skill on all three surfaces | At use time requires Node.js 20+, Vercel CLI 53+, authentication and project linkage, and may require Observability Plus. It reads source and service data, runs processes, fans out investigations, and writes audit artifacts. Sequential investigation is the coherent fallback when a host cannot fan out. |
| `web-design-guidelines` | Single loader `SKILL.md` | Structurally native skill on all three surfaces | Fetches moving `main/command.md` from `vercel-labs/web-interface-guidelines`, then reads matched consumer files and emits a review. The loader is native, but its effective rule set is not pinned or offline-complete. |
| `writing-guidelines` | Single loader `SKILL.md` | Structurally native skill on all three surfaces | Fetches moving `main/command.md` from `vercel-labs/writing-guidelines`, then reads matched consumer files and emits a review. The loader is native, but its effective rule set is not pinned or offline-complete. |

No directory is decomposed into Pack `asset` resources. Packy's adapters
fingerprint and link a `skill` source as one complete tree, so nested rules,
references, scripts, schemas, metadata, and executable modes already
participate in source integrity, drift detection, ownership, application, and
removal. `asset` remains appropriate only for a separately modeled file that
must be materialized into a rendered consumer.

## Fit with current Packy contracts

### Manifest v3

The current schema already admits the selected shape:

- top-level surfaces are the sorted set `claude`, `codex`, `opencode`;
- nine sorted `skill` resources point at relative bundle directories;
- every skill has one sorted native binding for every declared surface;
- `requires`, `bindings`, and `surface_exclusions` are non-null;
- each source directory contains `SKILL.md`; and
- the source tree stays inside the bundle and contains no selected ZIP.

There is no schema change required to express or project the nine skill trees.
There is also no reason to add `command`, `agent`, `instruction`, `lifecycle`,
or `mcp_server` resources: upstream exposes nine Agent Skills, not separate
host resources.

### Complete surface adapters

The adapters already implement the required inert projection:

- `internal/codex.SurfaceAdapter` fingerprints the whole source tree and
  manages a skill link below Codex's skills directory.
- `internal/opencode.SurfaceAdapter` does the same below OpenCode's skills
  directory.
- `internal/claudecode.SurfaceAdapter` validates the skill closure, observes
  the exact symlink target and tree fingerprint, and manages the personal skill
  link.

The capability-pack layer retains collision blocking, contributor ownership,
sealed plans, stale-plan rejection, verification, and recovery under
[ADR 0005](../adr/0005-capability-pack-surface-adapter.md). The adapters do not
gain Vercel-specific policy.

## Remaining prerequisites

### Blocking before implementation or publication

1. **Redistribution authority:** publication remains blocked until Vercel
   supplies an authoritative license/notice or explicit permission covering
   the complete nine-package contract and its auxiliary files. Four
   frontmatter `license: MIT` fields and a root README that only says `MIT` do
   not supply a complete notice-bearing grant; five packages have no local
   license declaration.
2. **Transitive guideline disposition:** source/version policy must choose
   whether the two guideline loaders remain explicitly moving runtime
   authorities or whether their rule repositories are independently pinned,
   licensed, and packaged. A pinned Vercel loader SHA alone cannot support a
   reproducible or offline-complete claim.
3. **Immutable source proposal:** synchronization must bind the nine resource
   directories to the exact Vercel commit, exclude every ZIP, preserve file
   modes, and publish source-scoped provenance under
   [ADR 0012](../adr/0012-adopt-source-scoped-pack-source-provenance.md).

### Observable-contract decisions

1. Pack-level `requires.tools` is an activation prerequisite and has no version
   constraint. It must not be used to make Node, Vercel CLI, authentication, or
   entitlements activation-time requirements for all nine inert skills.
   The observable-contract ticket must decide the operator-visible,
   invocation-time representation for exact runtime prerequisites such as
   Node.js 20+, Vercel CLI 53+, project linkage, and Observability Plus. If
   exact per-resource/versioned disclosure must be machine-readable, that is a
   manifest schema addition; the current tree projection does not need one.
2. Current optional authority vocabulary can disclose filesystem, process,
   network, browser, subagent, package-manager, commit, and deploy effects, but
   optional modes are pack-wide. The contract must map those modes to the
   affected workflows and preserve coherent fallbacks without implying that
   activation granted runtime authority.
3. Authentication, secrets, environment mutation, Vercel project linkage,
   service metrics, and service entitlements need explicit operator wording.
   They may be represented by the existing coarse authorities plus product
   text, or motivate a later vocabulary extension; they must not be silently
   collapsed into “network.”

### Adapter and validation evidence

1. No adapter extension is required for filesystem projection, collision
   protection, fingerprinting, apply, update, deactivation, or recovery.
2. Filesystem presence is not proof that Codex, OpenCode, or Claude Code loaded
   or can use a skill. Publication therefore needs independent sandboxed host
   evidence for all nine names on each surface, including auxiliary-file
   access, executable-mode preservation without automatic execution, collision
   failure, update/no-op isolation, deactivation, and foreign-content
   preservation.
3. Codex and OpenCode currently report generic skill runtime usability as
   unobserved; Claude can accept runtime evidence but must not invoke the
   workflow to manufacture it. The validation ticket must define acceptable
   host discovery/load signals. If lifecycle status must become
   runtime-observed rather than publication-smoke-only, add a generic
   surface-evidence observer instead of Vercel-specific adapter branches.

## Product consequences

- The mapping ticket creates no new schema or adapter implementation ticket for
  the basic projection.
- The source/version/licensing ticket owns the blocking redistribution and
  transitive-source decisions.
- The observable-contract ticket owns runtime prerequisite, authority,
  fallback, and machine-readable disclosure decisions.
- The naming/conflict ticket owns aliases and any proposed sharing after exact
  content comparison.
- The validation ticket owns independent host discovery and lifecycle evidence.
- Implementation remains out of scope until those decisions produce one
  execution-ready specification.

## Verification targets for the later implementation

The eventual implementation must prove, with sandboxed `HOME` and
`XDG_CONFIG_HOME`:

1. exactly nine logical skill resources and 27 explicit native bindings;
2. exact public names and invocation forms on each host;
3. byte- and mode-complete trees with no ZIP artifact;
4. relative access to every required rule, reference, script, library, schema,
   and playbook;
5. no process, authentication, network, Git, Vercel, package-manager, or
   deployment effect during sync, validation, installation, activation,
   update, status, or deactivation;
6. atomic collision blocking and preservation of foreign or drifted content;
7. independent host loading evidence rather than cross-surface inference; and
8. fail-closed publication while license or selected transitive-source evidence
   is incomplete.

## Conclusion

All nine pinned Vercel capabilities map cleanly to existing Pack manifest v3
`skill` resources and existing native skill-directory projections on Codex,
OpenCode, and Claude Code. The route does not require a Vercel-specific adapter
or a different resource kind. The remaining fog is product contract, source
authority, reproducibility, precise runtime-prerequisite disclosure, and
independent host evidence—not filesystem projection.
