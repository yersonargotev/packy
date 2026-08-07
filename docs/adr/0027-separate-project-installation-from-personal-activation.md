# ADR 0027: Separate project installation from personal activation

## Status

Accepted.

## Context

A project must be able to reproduce a capability pack for collaborators without treating a clone as consent to execute hooks, trust MCP servers, authenticate accounts, or expose credentials. A project-only symlink or personal activation record cannot distribute the pack, while committing active runtime authority would bypass each collaborator's consent.

## Decision

Packy separates project installation from project activation. Installation declares, locks, previews, and materializes the complete selected resource closure in version-controlled project paths, including non-secret MCP definitions and hook artifacts, but never stages or commits Git changes. Activation is a separate personal operation that previews and obtains the current user's consent before enabling runtime effects such as hooks or MCP access; trust, authentication, OAuth, and credentials remain personal and never enter the project declaration or lock.

The canonical project declaration is the human-reviewable `packy.json` at the Git root. Packy's generated exact resolution is `packy.lock.json` beside it. Both are version-controlled; neither reuses the personal global `~/.packy/packs.json` state contract, and Packy does not hide the project dependency contract beneath a dot-directory.

This decision refines ADR 0022: explicit project installation intent authorizes version-controlled project-surface projections. Activation intent remains mandatory for global projections, personal mutations, and runtime effects.

A cloned installation may therefore be ready for declarative discovery while remaining pending project activation for the current user. Packy must report that state explicitly rather than classifying it as drift or implying that installation granted runtime consent.

In an interactive session, installation may offer to proceed immediately into a separately previewed project activation. Declining leaves installation successful and reports the exact activation command. Non-interactive installation never enables runtime effects without explicit activation intent.

Project deactivation and uninstallation remain distinct. Deactivation removes only the current user's runtime authority and personal effects while preserving the project declaration, lock, and materialization. Uninstallation changes the shared project dependency and removes only still-owned projections; when the current user is activated, the interactive flow separately previews and completes deactivation before uninstallation.

Project installation performs complete preflight before mutation. A non-composable target that already exists without Packy ownership blocks the complete installation even when its bytes match; Packy neither adopts nor overwrites it. Composable host files may retain foreign content around exact Packy-owned contributions, but missing, changed, or ambiguous ownership markers also block rather than granting authority.

When multiple CLI surfaces natively discover the same project target, Packy materializes one shared projection and records every installation contributor. Adding a contributor does not rewrite an already-correct projection, and removal preserves it until the last contributor is removed. Host-specific formats remain separate projections.

Project installation remains independent of the installer's global state. It may materialize and record the project's selected version when an incompatible version of the same pack is globally active, but Packy reports an activation scope conflict and blocks personal readiness until the global and project observable contracts are compatible or one contribution is removed. Exact compatible contributions may coexist; Packy does not invent precedence or silently rename resources.

A project installation is declaratively self-contained. Its lock fixes and its project projections materialize the complete transitive pack-resource closure; a global contribution never satisfies a project dependency. External requirements that cannot be vendored remain declared readiness work for each user's personal activation.

Materialized project resources are Packy-owned generated projections, not mergeable customization points. Any manual content change is blocking drift: status reports it, update and uninstall preserve it and refuse mutation, and the operator must restore the locked content or move the customization into an independently owned resource or derived Pack Source before retrying.

The committed lock is portable ownership evidence, not standalone deletion authority. On every mutation Packy validates the manifest-to-lock relationship, re-derives canonical targets through the selected host adapter, constrains them to supported project surfaces, verifies exact fingerprints and contributors, and requires an explicit previewed operation. Absolute, escaping, arbitrary, malformed, or drifted targets are rejected; composable files grant authority only over exact Packy-owned contributions.

Personal activation is sealed to the exact locked identity of sensitive resources and requirements. A change to a hook, MCP definition, plugin, executable, or other sensitive effect makes the prior activation stale and grants no authority to enable or execute the changed effect; Packy reports the sensitive diff and requires a newly previewed activation. Purely declarative changes do not imply renewed runtime consent. Hook execution must bind to an immutable identity or freshly verify the approved digest so approved paths cannot silently execute changed bytes.

The project manifest and lock use exact pack versions. An omitted CLI version resolves once to the catalog-current admitted version and records that exact value; reinstall does not float it. Version ranges, moving refs, and `latest` are not project dependency intent. Project update is the explicit, previewed operation that changes the manifest version, lock identity, and materialized projections.

Each project fixes one version of a capability pack across all installed CLI surfaces. Surfaces remain separate selections and projections of that common host-independent contract, but cannot resolve different versions. Adding a surface reuses the locked version; changing it is one project-pack update whose preview covers every installed surface.

Project resource selection remains per surface. Omitting resource roots selects the complete pack for that surface; explicit roots pull their complete transitive closure. The lock records the resolved closure and materialization is the union of all roots and contributors. Incidental discovery of a shared projection by another surface does not create installation or activation intent for that surface.

`packy pack install <pack>` changes only that Pack's project intent, receipt, and projections. Installation always requires a Pack argument; Packy does not build or apply a project-wide reconciliation transaction. A named install may restore that Pack's missing exact owned projection, but blocks on changed or foreign content and never advances a version implicitly.

Project mutation stages exact desired content, backups, and a durable recovery journal before applying an immutable plan in deterministic order. It publishes the project lock only after projection verification. A failure must either restore and verify the prior state or preserve evidence as recovery-required; unresolved recovery blocks new project mutation, and the next project installation command handles recovery before new intent. Packy never uses destructive Git operations as rollback.

Project mutation does not require a globally clean working tree. Unrelated changes and foreign content around an intact composable contribution are preserved; owned-resource drift and target collisions block. Plans seal complete observed target content and reject concurrent change after preview. Packy neither stages nor commits files and runs no Git restore, checkout, reset, or equivalent discard operation.

Project status has two independent axes. Shared installation is absent, installed, drifted, or blocked; personal runtime activation is not-required, pending, active, stale, or blocked. Declarative resources may be usable immediately from a valid installation, and a pack with no personal runtime effects does not require or offer an empty activation.

Project status supports two enforcement levels. `--require installed` validates the manifest, lock, complete materialized closure, modes, contributions, and absence of drift without consulting or requiring personal activation and is safe for sandboxed CI. `--require usable` additionally requires one pack and surface to have current personal activation, compatible global contributions, trust, authentication, external requirements, and observed runtime readiness.

The project lock is self-contained for offline verification, personal activation, deactivation, and exact owned uninstallation. It records immutable Pack Source and contract identity, selections, the transitive logical resource graph, bindings, contributors, fingerprints, modes, and safe logical projection identities; adapters still re-derive permitted physical targets. Adding or updating dependencies and reconstructing absent bytes may acquire the exact source, but Packy never invents content from a digest when acquisition is unavailable.

The project manifest and lock have independently versioned published schemas. Unsupported schemas or declared minimum Packy capability fail closed before mutation; read-only status never migrates them. A supported newer CLI may read older formats, but schema migration is an explicit previewed mutation. Generator version is diagnostic metadata rather than authority.

Project contracts may carry sensitive inputs only as declared references such as environment-variable names or host-owned authentication requirements. Installation never resolves secret values, and project files, locks, previews, receipts, fingerprints, logs, and recovery evidence never persist or disclose them. Personal activation observes only the minimum readiness fact or delegates authentication to the host. An adapter that requires a literal secret in a version-controlled projection is unsupported and blocks.

Required licensing and attribution are mandatory members of the installed closure. Packy materializes exact per-pack contributions in the version-controlled root `PACKY-NOTICES.md`, records their origin, contributors, and digest in the lock, and protects them with the same composable ownership rules. Selection cannot omit required notices, and removal preserves each contribution until its last dependent is removed.

Every selected resource must be representable on its installed surface through a declared native binding or explicit degradation. Installation records degradations in preview and lock, includes non-projecting dependencies such as assets and notices in their consumer closure, and blocks the complete surface selection when any operational resource is structurally unrepresentable. A temporarily unavailable external requirement may leave personal readiness pending but cannot justify silently omitting project intent.

Native-name collisions block unless the operator supplies an explicit surface alias. Aliases remain attached to stable logical resource identities, are persisted in the project manifest and lock, and change only through a previewed transition. Packy never invents names or prefixes. Contributors to one physical shared projection must agree on its alias; divergent aliases are a conflict rather than authority to duplicate the resource silently.

Capability-provider choice is durable project intent. An unambiguous provider may be proposed, but multiple valid providers leave installation decision-required until the operator selects one explicitly. The manifest records the logical choice and the lock fixes its exact provider pack, version, resource, and transitive closure. Global availability never satisfies the project, and Packy does not silently substitute a provider when resolution changes.

When a project installation disappears while personal activation receipts remain, Packy reports orphaned project activation. Observation performs no cleanup, usable enforcement fails, and new activation is blocked until explicit project deactivation uses still-valid receipts to remove exact personal contributions. Drift is preserved, and receipts retire only after verified absence; loss of the shared installation does not grant destructive authority.

An exact compatible global activation may satisfy project runtime effects resource by resource and is reported as inherited-global. Packy creates no project receipt for inherited authority, keeps uncovered effects pending, and treats incompatible contributions as activation scope conflicts. Removing global coverage returns the affected effects to pending without activating them locally.

Project manifests may depend only on Packy catalog or history versions admitted through the Pack Source workflow. They cannot declare arbitrary URLs, repositories, moving refs, or machine-specific source overrides. The lock records exact admitted provenance and may reacquire only that immutable authority; registering or synchronizing a new source remains a separate governed workflow.

Personal project activation state, approvals, receipts, sensitive lock identity, and recovery live beneath `~/.packy/projects/`, with the exact layout derived by the owning module from the workstation snapshot. No personal lifecycle state is written to the repository, its Git metadata, a gitignored project file, the manifest, or the lock.

The personal checkout key is a digest of the canonical Git worktree root. Separate clones and worktrees therefore receive separate activation state, while invocation from subdirectories or through equivalent paths resolves consistently. Moving a checkout creates a new identity and requires reactivation; Packy does not transfer consent automatically from orphaned state.

Existing lifecycle commands remain global when `--project` is absent; being inside a Git worktree never changes their meaning. `--project` explicitly selects personal project lifecycle and project status. Installation and uninstallation are inherently project operations and require a Git worktree. Project status combines the shared installation axis, personal runtime axis, and observed global conflicts without changing existing default output.

Because one project pack version spans every installed surface, `update <pack> --project` updates the complete pack without a surface selector and previews every affected surface. Installation adds or changes one surface selection; uninstallation with a surface removes only that contributor, while uninstallation without a surface removes the complete project pack. Removing the last surface removes the pack entry and any now-unrequired closure.

`internal/capabilitypack` remains the sole semantic owner of global lifecycle, project installation, and personal project activation. It reuses one catalog, resource graph, ownership model, sealed-plan lifecycle, readiness model, and complete `SurfaceAdapter` seam, with cohesive internal decomposition rather than a second public installation engine. Host modules continue to own global and project path derivation, representation, inspection, and projection application; `internal/cli` remains only the composition and command adapter.

## Consequences

- Collaborators receive reviewable pack resources through Git without inheriting another user's authority.
- Hooks may be distributed by installation but cannot execute through Packy-managed registration until each user activates them.
- MCP definitions may be shared with environment placeholders, while secrets and authentication remain outside the repository.
- Installation and activation retain separate previews, ownership evidence, recovery, update, and removal semantics even if the CLI later offers a guided flow across both operations.
- A guided command may connect the two operations without collapsing their consent or recovery boundaries.
- `deactivate --project` cannot silently uninstall shared project resources, and `uninstall` cannot strand the current user's runtime registrations without a guided cleanup decision.
- A blocked projection cannot leave a partial materialization or a declaration and lock that claim an incomplete pack.
- Shared project projections avoid duplicate vendored trees without merging surface-specific representations.
- A collaborator can commit the project's intended dependency despite a personal global conflict, while Packy refuses to claim ambiguous runtime behavior is ready.
- Cloning the repository reproduces the declarative dependency closure without relying on the installing user's global packs.
- Pack updates remain deterministic and cannot overwrite team changes hidden inside vendored projections.
- A fresh clone can safely update or uninstall exact Packy projections without inheriting the original installer's personal receipts.
- Pulling a changed project lock cannot silently extend a collaborator's prior runtime consent.
- Repeated installation and independent clones cannot resolve different pack contracts from the same manifest.
- Shared projections and transitive dependencies cannot combine incompatible per-surface versions of one pack.
- Removing one root or surface preserves every resource still required by another root, dependency, or contributor.
- Manifest edits, merges, and fresh checkouts use one idempotent convergence operation rather than a separate synchronization vocabulary.
- Status and command summaries cannot collapse a valid installation and personal runtime consent into one ambiguous active flag.
- CI can enforce the shared project contract without inheriting or fabricating a user's runtime authority.
- A committed project remains inspectable and removable when a catalog or network source is temporarily unavailable.
- Older clients cannot silently reinterpret, downgrade, or partially apply a newer project contract.
- Reproducible project configuration cannot become a credential-distribution channel.
- Vendoring a pack cannot silently discard the Pack Source's admitted downstream notice obligations.
- A project cannot claim a complete Pack observable contract while quietly dropping unsupported behavior.
- Host-visible names remain reproducible user intent rather than machine-dependent collision resolution.
- Every collaborator resolves the same provider graph instead of inheriting workstation-dependent choices.
- Pulling an uninstall commit cannot silently mutate another collaborator's personal host configuration.
- Compatible global runtime setup is reused without obscuring which project effects still need personal consent.
- A cloned project dependency cannot bypass Packy's source, provenance, content, and legal admission controls.
- Project files remain a portable shared contract while Packy retains one workstation-owned namespace for personal lifecycle state.
- Path changes fail safely toward fresh consent rather than applying one checkout's approval to another.
- Existing scripts retain global command semantics while project mutation and consent remain explicit.
- Project update cannot imply unsupported per-surface version divergence.
- Multi-file failure cannot be reported as success or hidden behind an unverifiable best-effort rollback.
- Unrelated work does not block installation, while exact target safety remains fail-closed.
- Shared lifecycle invariants remain local to one deep domain module instead of being duplicated across global and project engines.
