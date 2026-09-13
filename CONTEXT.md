# Context

This glossary defines Packy's domain language. The accepted
architecture is recorded in [ADR 0031](docs/adr/0031-simplify-packy-around-reviewed-packs.md)
and [ADR 0033](docs/adr/0033-make-the-tui-the-primary-interactive-interface.md).
Readiness architecture is recorded in [ADR 0035](docs/adr/0035-make-pack-readiness-capability-driven.md).
Independent catalog authoring and publication are recorded in
[ADR 0039](docs/adr/0039-publish-an-independent-canonical-pack-catalog.md).

## Glossary

### Packy

A terminal installer and configurator for reviewed capability Packs on Codex,
OpenCode, and Claude Code. Its interactive and command-line interfaces expose
the same Pack lifecycle.

### Pack

A Git-reviewed capability bundle with one manifest, reviewed resources,
declared supported surfaces, and a maintainer-selected SemVer.

### Reviewed Pack catalog

A reviewed collection of independently selectable Pack versions. Each Pack
declares its own resources and supported surfaces.

### Catalog Project

The maintainer-controlled project that canonically authors the reviewed Pack
catalog's manifests, resources, adaptations, and provenance.

### Catalog Snapshot

An immutable, complete publication of the reviewed Pack catalog, identifying
the exact included Pack versions and content.

### Catalog Publication

The release of a reviewed Catalog Snapshot for consumption independently of a
Packy release.

### Catalog Refresh

The explicit selection of a newly acquired, validated Catalog Snapshot as the
available reviewed Pack catalog. It does not update installed Packs.

### Pack Import

The explicit introduction of selected external resources at a pinned revision
into the Catalog Project, with their provenance and notices.

### Upstream Refresh

A reviewed change to imported Pack resources against a selected newer upstream
revision. Exact copies and maintained adaptations retain distinct relationships
to their external sources.

### Orchestrate Pack

The Codex-only Pack that contributes the reviewed `$orchestrate` coordination
skill, its MIT notice, and the Pack-authored `coordinate-session` lifecycle.
It preserves Eric Provencher's attribution and distinguishes configured
projection from runtime usability.

### Pack manifest

The single `pack.json` contract for a Pack. It declares identity, version,
description, selectability, supported surfaces, resources, bindings,
intra-Pack dependencies, external requirements, readiness obligations,
conflicts, and resource provenance.

### External Source Project

A public repository contributing selected Pack resources at an exact upstream
commit. It establishes their provenance without authorizing catalog publication.

### Declared Pack Closure

A Pack manifest and the deterministic union of its declared resource and typed
surface-capability source roots.

### Pack resource

One host-independent capability contributed by a Pack. A surface adapter turns
it into one or more host-native projections.

### CLI surface

A supported host Packy can configure. The supported surfaces are Codex,
OpenCode, and Claude Code. Antigravity and GitHub Copilot CLI remain outside
the current product.

### Surface capability

A closed, reviewed, typed Pack-binding request for reusable host-native
behavior that is not implied by Pack, version, or resource identity.
`project-instruction` projects reviewed source as an independently owned marked
contribution in a project's native instruction document.
`opencode-primary-prompt` projects reviewed source as OpenCode's global primary
instruction document and registers that document in workstation OpenCode
configuration; project-native guidance remains owned by `project-instruction`.
`external-executable-acquisition` explicitly selects one reviewed acquisition
flow for a declared external requirement. It may install the shared executable
but never grants a tool authority to configure a CLI surface. Engram uses it to
retain its supported Homebrew flow without running tool-owned host setup.
`claude-composite-skill` projects a reviewed skill tree or command as a Claude
skill together with explicitly declared dependency and reference roles.
`claude-agent-document` projects a reviewed agent source, declared skill
dependencies, and portable authority as one native Claude agent document.

### Pack activation

The user's explicit consent to a previewed global Pack operation on one CLI
surface. Project installation and personal project activation are separate.

### Pack lifecycle

The previewed state transitions for one Pack, CLI surface, and global or
project scope. It includes inspection, consent, application, and verification.

### Project installation

The reviewed project intent and materialized projections for one Pack and
surface in a Git worktree.

### Project Pack manifest

The human-authored `packy.json` at a project root. It records direct Pack,
surface, and resource intent.

### Installed Pack receipt

The minimal current-state record for one Pack and surface: Pack identity and
version, reviewed readiness obligations and external-requirement names,
selected resource closure, projected paths, and content digests. It is the
authority for safe update or removal of unchanged Pack-owned content and for
offline readiness evaluation. Project receipts also seal committed projection
file modes for portable integrity verification. Receipts never contain runtime
evidence.

### Project Pack lock

The generated `packy.lock.json` containing one installed Pack receipt per Pack
and surface. It is committed with the project Pack manifest.

### Pack ownership

Packy's authority over an exact projected path established by an installed
Pack receipt.

### Owned projection drift

A receipt-owned path whose current content differs from its recorded digest.
Ordinary mutation stops before writing; force remains limited to paths in the
targeted receipt.

### Pack audit

The read-only, redacted trust report that composes workstation health, active
global Pack health, and portable verification of the current project's Pack
contract. It preserves readiness severity: unknown observations are
informational, warnings do not fail automation, and confirmed failures return
a non-zero status after the complete report is emitted.

### Pack projection conflict

An attempted operation in which distinct Pack resources target the same path.
It fails before mutation, even when the proposed bytes match.

### Catalog authoring workflow

The maintainer's preparation, validation, and review of Pack content changes in
the Catalog Project, followed by Catalog Publication of the reviewed result.

### External requirement

A host tool a Pack needs, such as Engram. Packy reports readiness without
turning external tools into Pack relationships.

### Readiness obligation

A reviewed, typed requirement evaluated by the capability-pack domain from
surface observations or an approved controlled runtime check. Existing external
requirements and receipt-backed projection integrity can produce obligations
without duplicating declarations in a Pack manifest.

### Readiness condition

The domain-owned result of one readiness obligation: its stable type,
configured, authorized, or usable dimension; true, false, or unknown value;
stable reason; user-facing message; scoped evidence references; observation
time; and validity identity.

### Readiness dimensions

The three independent readiness meanings. **Configured** means Packy's
reviewed projection is in the required state; **authorized** means the required
host authorization is established; **usable** means the required runtime
behavior is observed. A false condition dominates a dimension, otherwise an
unknown condition keeps it unknown, and a dimension is true only when all of
its conditions are true.

### Controlled runtime check

An explicit, approved operation separate from activation that records positive
or negative personal workstation evidence for host behavior Packy cannot
otherwise observe. Its evidence is stored only in Packy Home and is stale when
its Pack, surface, selected resources, projection, adapter, or observable host
identity changes.

### Issue delivery

The end-to-end integration of one approved issue through qualification, proof, protected review and merge, closure, and cleanup.

### Issue delivery policy

The repository-owned contract that adapts issue delivery to local approval,
validation, review, merge, and cleanup rules.

### Packy release

An immutable Packy distribution published from one version tag on `main`. A
faulty release is corrected by a newer version.

### Packy Home

The `~/.packy` root containing Packy's workstation receipts and state.
