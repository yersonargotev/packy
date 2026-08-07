# Context

> **Transition to Packy v0.2:** This glossary defines the accepted target domain model in [specification #513](https://github.com/yersonargotev/packy/issues/513) and [ADR 0031](docs/adr/0031-simplify-packy-around-reviewed-packs.md). The v0.1 implementation remains authoritative for current runtime behavior until the approved implementation tickets land.

## Glossary

### Packy
A command-line installer and configurator that manages capability packs for Claude Code, Codex, and OpenCode. Repository governance, upstream synchronization automation, and release publication are supporting development concerns rather than Packy product capabilities.

### Packy core
The always-available CLI behavior that discovers, installs, activates, updates, inspects, and removes capability packs. It is distinct from every optional pack, so changing a pack never disables the tool needed to manage it.

### CLI surface
An AI coding host that Packy can integrate with.

### Supported CLI surface
A CLI surface whose Packy-owned integration is shipped, documented, and covered by current validation and release evidence. The supported CLI surfaces are Codex, OpenCode, and Claude Code; a capability pack may target any declared subset of them.

### Target CLI surface
A CLI surface named by planning or a capability pack contract but not yet established as generally supported; target status alone never authorizes a support claim. Antigravity and GitHub Copilot CLI remain future candidates.

### Skill bundle
The curated set of agent skills Packy installs or exposes for a workflow. The current candidate bundle is based on Matt Pocock's engineering skills rather than Gentle AI's SDD stack.

### Skill source
The single resolved source from which Packy reads its skill bundle. Its origin may be an explicit operator override, the current Packy repository checkout, or the Installed Source, but every consumer uses the same selection.

### Pack source reference
Informational upstream repository and revision metadata recorded with a capability pack. The reviewed Pack content in Git is authoritative; a source reference grants no separate admission or synchronization authority.
_Avoid_: Pack Source, provenance lock

### Installed Source
The user-owned Packy checkout initialized for package-installed operation. It is one candidate for Skill Source selection and remains distinct from the selected Skill Source itself.

### Capability pack
A named, composable set of AI workflow capabilities that can remain available while being activated or deactivated as a unit. A capability pack may contribute skills, memory, tools, agents, rules, or other host-supported behavior; it is not a runtime configuration profile.

### Capability pack manifest
The single current data contract that owns a Pack's identity, version, description, selectability, surfaces, resources, bindings, intra-Pack dependencies, external requirements, concrete conflicts, and exclusions. Packs neither provide nor require capabilities from other Packs; Packy has no provider resolver, legacy manifest readers, or catalog metadata encoded separately in Go.

### Personal guidance pack
A publicly selectable capability pack that expresses one author's cross-project working preferences. Its resources remain independently selectable and may be activated globally or installed in a project.
_Avoid_: Personal rules package, private pack

### Argote pack
The personal guidance pack authored by Yerson Argote. It contributes independent engineering-principles, neutral-Spanish, and explanation-repair resources.
_Avoid_: Argote profile, Argote rules package

### Selectable pack catalog
The Packy-owned set of current capability-pack versions advertised by `packy pack list` and available for fresh activation or update. Its clean manifest generation begins with Addy, Argote, Engram, and Matty at pack version `1.0.0`; no unpublished Pack content ships alongside it.

### Pack resource
One host-independent intent contributed by a capability pack. A CLI-surface adapter may realize one pack resource as multiple host-specific artifacts; host-native schemas, paths, and package formats are projections rather than pack resources.

### Shared projection
A realization of a pack resource at one standard global or project target that more than one CLI surface may discover. It remains attributable to explicit activation or installation contributors, but discovery by another surface does not activate or install the pack for that surface.

### Pack requirement
A global prerequisite a capability pack consumes but does not contribute to a CLI surface. Packy core may detect an external tool, but acquisition requires a separately approved phase originating from explicit pack activation or update intent.

### Lifecycle resource
A pack resource that declares behavior triggered at CLI lifecycle points. It names the portable intent while each CLI-surface adapter owns its event names, handlers, trust model, and rendered artifacts; it is not a universal hook schema.

### Pack activation
The user's explicit consent to a previewed reconciliation of either a complete capability pack or selected resource roots and their declared dependencies on one CLI surface. Global activation owns global projections and runtime effects; project activation owns only the personal runtime effects of an existing project installation. Activation does not itself grant host trust, authenticate accounts, authorize unpreviewed executable code, or consent to destructive cleanup.

### Pack readiness
The progression from **configured** (Packy-owned projections are reconciled), through **authorized** (required human trust and authentication are complete), to **usable** (the host has loaded the capability under its runtime permissions). An active pack may remain pending human action between these stages.

### Packy health
The read-only assessment of Packy core plus a summary of active-pack drift, requirements, and readiness. Inactive packs do not degrade Packy health.

### Pack desired state
The complete logical outcome Packy computes from the active capability packs on each CLI surface, including required shared resources and readiness, before translating that outcome into host-specific artifacts.

### Pack ownership
Packy's recorded authority over a projected resource or config fragment. Ownership is established by an installed Pack receipt and determines whether Packy may update or remove the exact recorded content; it is distinct from the host's trust, authentication, and runtime authorization.

### Owned projection drift
The condition in which a path recorded by an installed Pack receipt no longer contains the recorded content. Ordinary update and removal stop before changing anything; an explicit destructive override may affect only paths whose ownership is established by that receipt and never grants authority over unrecorded content.

### Pack projection conflict
An attempted installation in which distinct Pack resources target the same path. It blocks the operation before any write even when the proposed bytes are identical; Packy does not select a winner or maintain shared ownership between resources.

### Installed Pack receipt
The minimal current-state record created by a successful global activation or project installation. It identifies the Pack and version, target CLI surface, projected paths, and content digests needed to update or remove only unchanged Packy-owned content. Global state and `packy.lock.json` use the same receipt model; receipts contain no upstream provenance, version history, compatibility evidence, or persisted reconciliation plan.

### Independent Pack composition
The coexistence of multiple directly selected Packs without relationships between them. Each operation installs, updates, activates, deactivates, or removes one Pack and owns one receipt; Packy performs no cross-Pack dependency resolution, provider selection, graph transaction, or implicit composition.

### External-effect receipt
A versioned record of one exact external configuration contribution that Packy initiated and freshly verified. It grants reversal authority only while the same contribution and host surface still match; a command fingerprint, tool installation, service, data, credential, or unrelated configuration is never a receipt.

### Pack observable contract
The complete user-visible behavior of a capability pack, including its skill content, declared resources, external requirements, and activation or update experience. A pack version describes this contract rather than its upstream source version or textual diff size.

### Reconciliation plan
An in-memory preview of the exact changes needed to move one requested Pack operation from freshly observed state toward Pack desired state. It exists only for the command invocation that displays and applies it; Packy persists the resulting installed Pack receipt, not plans, attempts, or recovery history.

### Memory layer
The Engram-backed persistence and recall behaviour Packy provides across supported CLI surfaces.

### Delegation layer
The subagent orchestration behaviour Packy exposes, including read-only exploration and bounded implementation workers where the host CLI supports them.

### Issue delivery
The ordinary pull-request path for Packy changes. Issues organize work; native GitHub branch protection, required CI, review, and human merge protect integration without a Packy-owned authorization system.

### Packy release
An immutable Packy distribution published from one version tag. A faulty published release is corrected by a newer version rather than mutation or recovery.

### Pack authoring workflow
The repository workflow for creating or changing one Pack: copy the standard Pack directory template when needed, add or edit reviewed resources, update the single manifest and maintainer-selected SemVer, then run the focused validator for that Pack. It performs no upstream import, synchronization, classification, compatibility analysis, or version-history archival.

### Installer/configurator
Packy's product shape: a tool that configures supported CLI surfaces with selected capability-pack behavior rather than an active runtime orchestrator present in every agent session.

### Golden path
Packy v0's primary success path: given an existing repository, configure Codex and OpenCode with Matt Pocock-style skills, Engram memory, and delegation conventions while keeping the initial prompt minimal.

### Activation scope
The boundary within which an explicit pack activation contributes its resources. An activation scope is either global or belongs to one project.

### Global activation
An explicit activation whose contribution applies to every project for the selected CLI surface.

### Project activation
The current user's explicit consent to enable the personal runtime effects of one project installation in one checkout. It is sealed to the project pack lock, may add to global activation but cannot mask it, and is not transferred through the repository; already-installed declarative resources do not require an empty project activation.

### Project root
The root of the nearest Git worktree that identifies one project's activation scope. Directories outside a Git worktree do not define a project root.

### Project installation
A version-controlled declaration and materialization of a capability pack's complete selected resource closure within one project. It makes the project's expected capabilities reproducible for collaborators but does not transfer personal trust, credentials, or runtime consent.

### Project pack manifest
The human-reviewable `packy.json` at the project root that declares the project's direct Packs, surfaces, and resource intent. Multiple Packs remain independent entries; the manifest is distinct from generated receipts and personal lifecycle state.

### Project pack lock
The Packy-generated `packy.lock.json` at the project root that records the exact current Pack version, selected resource closure, project projections, and content digests. It is committed with the project pack manifest, uses the installed Pack receipt model, and contains no provider resolution, upstream source authority, compatibility evidence, or history.

### Pending project activation
The state of a project installation whose shared resources are present but whose runtime effects still require the current user's explicit consent. It is an expected readiness state, not installation drift.

### Project installation state
The shared status of a project pack's declared and materialized resources, reported independently from personal runtime activation. Its states are absent, installed, drifted, or blocked.

### Stale project activation
A prior personal activation whose approved sensitive-resource identity no longer matches the project pack lock. It grants no authority to enable changed runtime effects and requires a newly previewed activation.

### Orphaned project activation
Personal activation state whose corresponding project installation has been removed. It preserves exact cleanup evidence but requires explicit deactivation and never authorizes automatic cleanup or renewed activation.

### Activation scope conflict
An incompatibility between global and project contributions for the same logical pack resource that prevents Packy from establishing one reliable effective capability for the current user. It blocks personal readiness without invalidating the project's version-controlled installation.

### Effective activation
The additive composition of compatible global and personal project activation that authorizes runtime effects within one project. Declarative project availability belongs to project installation, and compatible global activation can make runtime effective without a separate project activation.

### Inherited global activation
The personal runtime state in which an exact compatible global contribution satisfies one or more sensitive effects of a project installation. It creates no project receipt and becomes pending if the global contribution is removed.

### Packy Home
The single workstation root reserved for Packy-owned state. Domains may own separate files beneath it without sharing ownership of those files.

### Workstation snapshot
The immutable, normalized view of ambient workstation facts used by one Packy command invocation. It is created only when an operation needs workstation access, and every participant in that operation observes the same snapshot.
