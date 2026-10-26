# Context

This glossary defines Packy's domain language. The accepted
architecture is recorded in [ADR 0031](docs/adr/0031-simplify-packy-around-reviewed-packs.md)
and [ADR 0033](docs/adr/0033-make-the-tui-the-primary-interactive-interface.md).
Readiness architecture is recorded in [ADR 0035](docs/adr/0035-make-pack-readiness-capability-driven.md).
Independent catalog authoring and publication are recorded in
[ADR 0039](docs/adr/0039-publish-an-independent-canonical-pack-catalog.md).
The two projection conventions and explicit custom-agent client targets are
recorded in [ADR 0045](docs/adr/0045-focus-surfaces-on-agents-and-claude.md).

The engine supports the Agents (`agents`) and Claude (`claude`) projection
conventions. Plugin concepts below describe the accepted target design; plugin
implementation and catalog adoption remain separate delivery work. Antigravity
and GitHub Copilot CLI remain outside its supported client integrations.

## Glossary

### Packy

A terminal installer and configurator for reviewed capability Packs on the
Agents and Claude surfaces. Its interactive and command-line interfaces expose
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

### On-demand Skill Access

Obtaining one qualified, reviewed skill, its declared skill dependencies, and
their supporting/legal resources for
an explicit CLI surface without installation or workflow execution.

### Qualified skill

The identity of a logical skill within its owning Pack, written `<pack>:<skill>`.

### Prepared skill resources

Packy-owned copies of a reviewed skill's effective resource tree and declared
supporting/legal resources, retained independently of installed projections.

### Access reference

An explicitly reviewed reference from a skill instruction document to the
prepared instruction document of its own skill or an explicitly declared skill
dependency, qualified by its owning Pack.

### Pack Import

The explicit introduction of selected external resources at a pinned revision
into the Catalog Project, with their provenance and notices.

### Upstream Refresh

A reviewed change to imported Pack resources against a selected newer upstream
revision. Exact copies and maintained adaptations retain distinct relationships
to their external sources.

### Orchestrate Pack

The reviewed Pack contributing the `orchestrate` coordination skill and its
MIT notice, preserving Eric Provencher's attribution. Projected content does
not establish a consuming client's runtime delegation capability.

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

A supported projection convention for reviewed Pack resources. The supported
surfaces are `agents` and `claude`; a surface does not by itself identify or
establish compatibility with a particular runtime client.

### Agents surface

The shared instruction and skill convention using `AGENTS.md` and
`.agents/skills`, independent of a specific runtime client.

### Claude surface

The Claude-native instruction and skill convention using `CLAUDE.md` and
`.claude/skills`.

### Native plugin projection

A client-consumable plugin package produced from reviewed Pack resources in
the Claude plugin format or the Agent Plugins format. Package validity alone
does not establish installation, authorization, or runtime usability.

### Runtime client

A concrete application, such as Codex or Claude Code, that consumes projected
Pack content. A shared CLI surface does not imply support for every client's
custom-agent format or execution capabilities.

### Custom-agent projection

A reviewed definition of a workflow role in one explicitly identified runtime
client's native format and scope. It is distinct from metadata inside a skill
and remains a declared resource dependency when a workflow requires that role.
Agents currently supports explicit global Codex roles through a reviewed
`client_projection` binding and a client-selected lifecycle. Unsupported client
or scope combinations fail before mutation; shared Agents skills select no
runtime client. Claude native agent documents declare client `claude` and their
reviewed global/project scopes; selecting the Claude surface identifies that
client. Unsupported required scopes fail before mutation. Direct MCP, hooks,
commands, and obsolete lifecycle resources are not Pack resources in the current
contract.

### Plugin export

The explicit production of a standalone native plugin projection from one
reviewed Pack and its selected skill dependency closure. An export does not
create Pack installation or client activation state.

### Plugin compatibility

The explicitly reviewed suitability of a skill and its authored references for
one plugin format. Native installation support alone does not establish plugin
compatibility or compatibility with every consuming client.

### Surface capability

A closed, reviewed, typed Pack-binding request for reusable host-native
behavior that is not implied by Pack, version, or resource identity.
`project-instruction` projects reviewed source as an independently owned marked
contribution in a project's native instruction document.
`external-executable-acquisition` explicitly selects one reviewed acquisition
flow for a declared external requirement. It may install the shared executable
but never grants a tool authority to configure a CLI surface. Engram uses it to
retain its supported Homebrew flow without running tool-owned host setup.
`claude-composite-skill` projects a reviewed skill tree as a Claude skill
together with explicitly declared dependency and reference roles.
`claude-agent-document` projects a reviewed agent source, declared skill
dependencies, and portable authority as one native Claude agent document.
`skill-template` projects a reviewed skill tree with an effective native identity
and explicitly authored references to other skills in the same Pack.

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

### Surface variant

A reviewed adaptation of one Pack resource for a particular CLI surface,
preserving the resource's kind, logical identity, and purpose.

### Common definition

The reviewed resource definition shared by supported surfaces that do not
declare a specific variant.

### Effective resource

The definition of a logical Pack resource applicable to one CLI surface.

### Skill template

A reviewed skill source whose explicit native-name references are rendered
against the effective bindings of its own selected Pack. Its installed result is
a generated, receipt-owned tree; its reviewed Catalog source remains immutable.

### Host coexistence

The ability to keep adaptations for multiple CLI surfaces installed and have
each host discover its intended definition without damaging another installation.
