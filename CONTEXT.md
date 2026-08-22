# Context

This glossary is the current domain language for Packy `v0.2.0`. The accepted
architecture is recorded in [ADR 0031](docs/adr/0031-simplify-packy-around-reviewed-packs.md)
and [ADR 0033](docs/adr/0033-make-the-tui-the-primary-interactive-interface.md).
Readiness architecture is recorded in [ADR 0035](docs/adr/0035-make-pack-readiness-capability-driven.md).
Managed Pack authoring and promotion are recorded in [ADR 0038](docs/adr/0038-promote-releases-from-managed-pack-projects.md).

## Glossary

### Packy

A terminal installer and configurator for reviewed capability Packs on Codex,
OpenCode, and Claude Code. Its interactive and command-line interfaces expose
the same Pack lifecycle.

### Pack

A Git-reviewed capability bundle with one manifest, reviewed resources,
declared supported surfaces, and a maintainer-selected SemVer.

### Reviewed Pack catalog

The current set of selectable bundled Pack manifests. Each manifest owns its
Pack version and supported surfaces, and each Pack is independently selectable.

### Orchestrate Pack

The accepted Codex-only Pack that contributes the exact upstream
`$orchestrate` coordination skill and its MIT notice. Its canonical source is a
stable release of `yersonargotev/orchestrate-skill`; Packy preserves Eric
Provencher's attribution and treats configured projection separately from
runtime usability.

### Pack manifest

The single `pack.json` contract for a Pack. It declares identity, version,
description, selectability, supported surfaces, resources, bindings,
intra-Pack dependencies, external requirements, readiness obligations,
conflicts, and Managed Pack provenance. Existing catalog manifests retain their
legacy fields only while the seven Packs migrate.

### Managed Pack Project

The one public, maintainer-controlled repository that authors one Pack through
a root schema v1 `pack.json`, canonical bundle-relative resources, declared
origins, and immutable self-contained Pack releases.

### External Source Project

A public repository named by a Managed Pack origin at one exact commit. It
contributes provenance bytes but does not authorize a Pack identity or release.

### Managed Pack Registry

Packy's reviewed one-to-one mapping from each Pack ID to its canonical Managed
Pack Project. It lives outside the end-user bundle.

### Declared Pack Closure

The root `pack.json` plus the deterministic union of every resource and typed
surface-capability source root declared by the manifest.

### Pack Admission Record

The append-only, Packy-owned record for one admitted Pack version. It pins the
immutable project release and Git identities plus manifest, closure, file mode,
and content digests, and is not part of the end-user bundle.

### Managed Pack Promotion

Packy's private operation that validates one registered immutable Managed Pack
release and returns no change, a typed rejection, or one protected proposal.

### Pack Source

One upstream provenance declaration for selected Pack resources and legal
notices. Its configuration identifies a stable release, while its lock records
the exact selected content used to reproduce and review a bundle generation.
It is the legacy catalog-maintenance model retained only until every current
Pack migrates to a Managed Pack Project.

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

### Pack authoring workflow

Copy the standard Pack template when needed, add or edit reviewed content,
update the one manifest and maintainer-selected SemVer, then run the focused
validator for that Pack.

### Single-source Pack admission

The atomic initial transition that creates one previously absent Pack and its
one Pack Source as a complete bundle generation. It admits the source and
selected content, legal notice, Pack manifest, catalog entry, and initial
history together or publishes none of them.

### Composite Pack Source Bundle

The two-or-more-source unit used to admit a previously absent Pack whose
initial provenance spans multiple Pack Sources. It is distinct from
single-source Pack admission.

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
