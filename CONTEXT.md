# Context

## Glossary

### Packy
A lightweight AI workflow toolkit inspired by Gentle AI, but intentionally centred on Matt Pocock-style skills, Engram memory, and explicit subagent delegation instead of SDD-first orchestration.

### Packy core
The always-available installer/configurator that manages capability packs and their lifecycle. Packy core is distinct from the optional `matty` capability pack, so deactivating that pack never disables the tool needed to manage it.
Without explicit pack or resource activation intent or explicit project installation intent, it may mutate only Packy-owned non-surface substrate and never a CLI surface.

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

### Installed Source
The user-owned Packy checkout initialized for package-installed operation. It is one candidate for Skill Source selection and remains distinct from the selected Skill Source itself.

### Capability pack
A named, composable set of AI workflow capabilities that can remain available while being activated or deactivated as a unit. A capability pack may contribute skills, memory, tools, agents, rules, or other host-supported behavior; it is not a runtime configuration profile.

### Personal guidance pack
A publicly selectable capability pack that expresses one author's cross-project working preferences. Its resources remain independently selectable and may be activated globally or installed in a project.
_Avoid_: Personal rules package, private pack

### Argote pack
The personal guidance pack authored by Yerson Argote. It contributes independent engineering-principles, neutral-Spanish, and explanation-repair resources.
_Avoid_: Argote profile, Argote rules package

### Selectable pack catalog
The Packy-owned set of current capability-pack versions advertised by `packy pack list` and available for fresh activation or update. Bundled or historical pack artifacts are not selectable merely because their bytes exist in the distribution.

### Pack resource
One host-independent intent contributed by a capability pack. A CLI-surface adapter may realize one pack resource as multiple host-specific artifacts; host-native schemas, paths, and package formats are projections rather than pack resources.

### Shared projection
A realization of a pack resource at one standard global or project target that more than one CLI surface may discover. It remains attributable to explicit activation or installation contributors, but discovery by another surface does not activate or install the pack for that surface.

### Composite Pack Source Bundle
The complete ordered set of two or more Pack Sources admitted together as the initial provenance of one previously absent capability pack. It is one sealed transactional value, not an additional persistent source identity.

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
Packy's recorded authority over a projected resource or config fragment. Ownership determines whether Packy may update or remove it and is distinct from the host's trust, authentication, and runtime authorization.

### External-effect receipt
A versioned record of one exact external configuration contribution that Packy initiated and freshly verified. It grants reversal authority only while the same contribution and host surface still match; a command fingerprint, tool installation, service, data, credential, or unrelated configuration is never a receipt.

### Pack observable contract
The complete user-visible behavior of a capability pack, including its skill content, declared resources, requirements, capabilities, and activation or update experience. A pack version describes this contract rather than its upstream source version or textual diff size.

### Pack compatibility
Whether a newer pack observable contract preserves the workflows and expectations of an active older version without an incompatible migration or newly mandatory user action.

### Decision-ready synchronization proposal
A source update whose exact identity, provenance, content changes, pack compatibility evidence, migrations, and validations are complete and unchanged, leaving human acceptance as the only remaining decision.

### Reconciliation plan
An immutable preview of the exact ordered changes needed to move one approved pack operation from freshly observed state toward pack desired state. Its identity covers the activation intent revision, relied-on observations, actions, and human-consent phases; changed inputs require a new plan and approval.

### Reconciliation attempt
One application of an approved reconciliation plan. An attempt ends verified when its outcome matches desired state, stale when its preconditions changed before any action, or recovery-required when completed and remaining effects must be reconciled from a fresh observation.

### Memory layer
The Engram-backed persistence and recall behaviour Packy provides across supported CLI surfaces.

### Delegation layer
The subagent orchestration behaviour Packy exposes, including read-only exploration and bounded implementation workers where the host CLI supports them.

### Governance expected-state contract
The versioned, reviewable description of Packy's accepted repository and
publication controls and the promotion or publication boundaries they protect.

### Governance observation
One read-only, sanitized projection of the effective controls bound to a
repository, protected ref, commit, workflow definition, and UTC collection
time.

### Governance drift
A confirmed, unclassifiable, missing, failed, or stale disagreement between the
governance expected-state contract and a governance observation.

### Issue delivery
The protected pull-request path for Packy changes. An approved issue provides
the change authority; required CI, traceable Standards and Spec review, and
resolved conversations protect integration. Release publication retains its
separate workflow and authority.

### Installer/configurator
Packy's v0 product shape: a tool that installs and configures Codex/OpenCode with the right skills, Engram memory hooks, and delegation conventions, rather than an active runtime orchestrator present in every agent session.

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
The human-reviewable `packy.json` at the project root that declares the project's direct pack, surface, and resource intent. It is distinct from generated resolution and personal lifecycle state.

### Project pack lock
The Packy-generated `packy.lock.json` at the project root that fixes the exact resolved pack closure and its project projections. It is committed with the project pack manifest and is not a hand-edited customization surface.

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
