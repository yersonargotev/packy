# {{TAG}} — Reviewed skills on demand

## Changes since the previous release

The previous stable release is `v0.2.30`.

- Replace the `codex` surface with the shared `agents` convention. Skills support
  global and project scope, instructions are project-only, and complete skill
  metadata and executable content remain intact. Required Codex roles use an
  explicit reviewed global `client_projection` and `--client codex`; ordinary
  Agents skills do not write Codex settings. Unsupported role scopes fail before
  mutation, and role files do not prove runtime delegation. Retired `codex`
  contracts require the previous-binary handoff rather than an alias or migration.

- Retire OpenCode from the CLI, TUI, catalog contracts, installation state, and
  discovery. Supported surfaces are `agents` and `claude`.
  Retired surface values and OpenCode capabilities fail without mutation; no
  aliases, legacy readers, or automatic migration are provided. Before replacing
  an OpenCode-capable binary, use it to preview and remove its owned global and
  project installations. Preserve unrelated files and retained content until the
  [explicit handoff](https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md)
  is complete.

- Obtain a qualified skill with `packy get skill <pack>:<skill> --surface agents`
  or `--surface claude`, with readable output or `--json`. Explicit host selection
  resolves the reviewed Effective Resource; installed copies and aliases do not
  replace the catalog resource. On-demand Skill Access supports Agents and
  Claude.
- Discover current local candidates with `packy list [--json]` and
  `packy show <pack> --surface agents|claude [--json]`. Inspection exposes qualified
  identities, host-effective descriptions and provenance, native names, direct
  relationships, reviewed compatibility, and known prerequisites. Incompatible
  skills remain visible with a reason. Keep qualified identities and ask the user
  when several candidates match; installed state does not rank candidates.
- Preparation includes the complete transitive same-Pack and cross-Pack skill
  dependency graph, supporting assets, notices, provenance, and executable modes.
  Each Pack retains its own version and source ownership. Reviewed access
  references resolve to usable relative entry-document paths within one atomic
  prepared procedure tree; native skill calls are not rewritten or registered.
- Carry the returned immutable snapshot identity into nested requests with
  `--snapshot <snapshot-id>`. Previously returned paths survive explicit Catalog
  Refresh, while new independent requests follow the newly selected local
  snapshot. Missing or invalid retained content fails actionably without revision
  substitution. There is no automatic expiration or eviction; manual deletion
  can invalidate retained paths. Modified prepared trees fail reuse instead of
  being silently overwritten.
- Versioned `skill-discovery` and `skill-access` JSON reports describe the
  selection, availability, dependencies, prerequisites, and resource locations.
  Retrieval failures have identifiable codes and no successful partial result;
  discovery retains the existing `list`/`show` stderr and nonzero-exit conventions.
  Help and generated shell completion expose `get skill`, `--surface`, and
  `--snapshot` through the public command tree. See the
  [structured-output contracts](https://github.com/yersonargotev/packy/blob/main/docs/structured-output.md)
  and [access protocol](https://github.com/yersonargotev/packy/blob/main/docs/on-demand-skills.md).

## Obtain resources, then follow the procedure

Initial catalog acquisition and later network refresh remain explicit:

```sh
packy init
packy list --json
packy show <pack> --surface agents --json
packy get skill <pack>:<skill> --surface agents --json
packy catalog refresh
packy get skill <pack>:<dependency> --surface agents --snapshot <returned-snapshot-id> --json
```

The bracketed identities are placeholders: choose a skill whose current catalog
declaration supports reviewed on-demand access. Substitute `claude` for Claude
Code. A nested dependency can belong to another Pack; keep the initiating host
and snapshot identity.

After explicit acquisition, discovery and access work offline and outside Git.
An absent local catalog explains how to acquire it. Access does not refresh or
download implicitly, require content runtimes, execute catalog scripts, install
external tools, activate a Pack, create native projections or receipts, or change
project files or installed copies. Native invocation continues using the installed
copy, including local modifications.

Read the returned `entry_document` and supporting files under `resource_root`.
Known prerequisites are reported, not verified: the consuming agent checks them
and follows the skill's approvals, domain assumptions, and retirement instructions.
Choose the intended target project separately from immutable resource locations,
and use separate writable scratch/output directories. Prepared files establish
resource availability, not native registration, runtime readiness, or successful
workflow execution.

## Catalog authoring and independent adoption

Pack manifest v3 supports reviewed `on_demand` declarations with the
`skill-tree-v1` protocol, explicit prerequisites, optional standalone
`entry_document` and `reference_files`, and typed qualified skill dependencies.
Same-Pack `requires` edges retain their native installation meaning. Reviewed
`{{packy.access:<skill>}}` and `{{packy.access:<pack>:<skill>}}` references in listed
documents resolve to the selected prepared entry document; arbitrary Markdown,
scripts, and native `{{packy.skill:...}}` references are not heuristically adapted.
Host variants resolve before dependency closure. Native templates and Claude
composite skills need a distinct reviewed standalone access document.

The released executable recognizes this complete vocabulary through existing
`catalog create`, `catalog import`, `catalog upstream-refresh`,
`catalog validate`, `catalog build`, `catalog publish`, and retained
consumption boundaries. Authors can exercise complete candidate snapshots with:

```sh
packy catalog candidate --snapshot /absolute/assets --workspace /absolute/new-workspace -- get skill <pack>:<skill> --surface agents --json
```

The disposable candidate interface also supports host-specific inspection while
preserving native installation's Git, consent, ownership, and drift guards.
Malformed or incompatible complete catalogs, invalid graphs, and undeclared
references fail without successful partial results or corrupting prior usable
state. Older engines reject the new vocabulary; publish this engine before
Catalog content that requires it. Catalog consumers can select one immutable
released executable without an adjacent Packy checkout or development binary.

The official bootstrap Pack and the reviewed adaptations of the 117 Matty,
pstack, and Warp skills remain separate Catalog-owned deliveries. This engine
release does not publish them, change the Catalog's engine declaration, or update,
migrate, activate, or refresh live user installations. Traditional installation
remains available through the existing preview and consent flow. Claude Code
**2.1.203 or newer** remains the supported floor.

For traditional native installation, acquire the catalog with `packy init`, then
preview the Pack activation before applying it:

```sh
packy activate engram --surface agents --dry-run
packy activate engram --surface agents
```

## Release verification

The existing immutable release workflow runs the same executable Catalog
contract against staged Darwin amd64, Darwin arm64, Linux amd64, and Linux arm64
archives on compatible runners before publication. The shared cases cover both
hosts, exact resources/modes/notices, dependencies and references, refresh and
retained paths, offline/non-Git access, absent content runtimes, inert scripts,
modified installed copies, and failure/state preservation. Each target retains
identity, checksum, artifact-validation, and functional test evidence. Publication
uses those tested archives without rebuilding them; compilation or version-only
checks do not replace the functional gate.

Protected integration, exact-version/source publication approval, immutable tags
and assets, protected release/Homebrew deployments, and installed-package
verification remain required. The release provides four archives, `SHA256SUMS`,
source identity through `packy version --json` and these notes, and a matching
Homebrew formula. See the [release and installation guidance](https://github.com/yersonargotev/packy/blob/main/docs/release.md).
