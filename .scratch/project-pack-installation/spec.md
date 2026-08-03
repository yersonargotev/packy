## Problem Statement

Packy currently activates capability packs only in user-global CLI surfaces. That works for one workstation, but it cannot make a pack a reproducible dependency of a project: global skill links, personal activation state, host trust, credentials, and runtime receipts do not travel with Git. A collaborator who clones a repository therefore does not receive the same skills, instructions, agents, commands, MCP definitions, hooks, assets, provider choices, or exact pack version.

Project teams need a way to declare and vendor an admitted capability pack in a Git worktree while preserving each collaborator's personal authority over sensitive runtime effects. The project contract must be reviewable, deterministic, safe to update or remove, usable offline, and compatible with the existing global lifecycle. Cloning a repository must never imply consent to execute hooks, trust MCP servers, authenticate accounts, expose credentials, or mutate personal host configuration.

## Solution

Packy will add project installation as a lifecycle distinct from activation. `packy pack install` will declare an exact admitted pack version, lock its complete selected dependency closure, preview the changes, and copy representable resources into host-native project surfaces. The version-controlled project contract will consist of a human-reviewable manifest, a generated exact lock, mandatory Packy notices, and the materialized resource projections. Packy will not stage or commit Git changes.

Project activation will remain personal. `packy pack activate ... --project` will consume a valid project installation and obtain consent for sensitive runtime effects such as hooks, MCP access, plugins, authentication, and external requirements. Interactive installation may offer to continue into a separately previewed activation; non-interactive installation never enables runtime effects implicitly. Packs containing only declarative resources require no empty activation.

Global activation will retain its current default semantics. Global and project contributions compose additively, project installations remain self-contained, and compatible global runtime effects may satisfy project runtime readiness without creating project receipts. Incompatible contributions will be reported as scope conflicts rather than resolved through hidden precedence.

## User Stories

1. As a project maintainer, I want to install an admitted capability pack in a Git project, so that collaborators receive its declarative resources when they clone the repository.
2. As a project maintainer, I want installation to copy resources instead of linking to my home directory, so that the project remains portable across workstations and operating environments.
3. As a collaborator, I want cloned skills and instructions to be immediately discoverable by the selected CLI surface, so that I do not need Packy for purely declarative use.
4. As a collaborator, I want hooks and MCP definitions to remain inactive until I consent personally, so that cloning never grants runtime authority.
5. As a project maintainer, I want installation and activation to be separate operations, so that shared project changes and personal effects have independent ownership and recovery.
6. As an interactive user, I want installation to offer a guided activation step, so that the safe two-phase model remains easy to use.
7. As an automation author, I want non-interactive installation never to activate runtime effects, so that CI cannot fabricate consent.
8. As an existing Packy user, I want unqualified lifecycle commands to retain global semantics, so that existing scripts do not change meaning inside a Git project.
9. As a project user, I want project lifecycle to require an explicit project selector, so that personal and global state cannot be confused.
10. As a project maintainer, I want Packy to discover the nearest Git worktree root, so that invocation from subdirectories addresses one project installation.
11. As a user with multiple clones or worktrees, I want each checkout to have independent personal activation state, so that consent is never transferred accidentally.
12. As a user who moves a checkout, I want Packy to require fresh activation, so that path changes fail safely.
13. As a project maintainer, I want a visible human-reviewable pack manifest, so that direct dependency intent is clear in code review.
14. As a project maintainer, I want an exact generated lock, so that every collaborator resolves the same pack graph and projections.
15. As a collaborator, I want manifest and lock schemas to be versioned, so that incompatible Packy versions fail before mutation.
16. As a collaborator, I want read-only status never to migrate project files, so that inspection remains effect-free.
17. As a project maintainer, I want omitted CLI versions to resolve once to an exact admitted version, so that repeated installation cannot float.
18. As a project maintainer, I want updates to be explicit, so that pack observable contract changes always receive review.
19. As a multi-surface project, I want one pack version shared by all installed surfaces, so that shared resources and dependency closure remain coherent.
20. As a multi-surface project, I want each surface to retain its own selected resource roots, bindings, aliases, and readiness, so that host capabilities can differ without version divergence.
21. As a project maintainer, I want to install a complete pack by default, so that the common path is concise.
22. As a project maintainer, I want to select resource roots explicitly, so that I can install a supported subset with all declared dependencies.
23. As a project maintainer, I want transitive resources fixed and materialized locally, so that global workstation state never satisfies project dependencies.
24. As a project maintainer, I want provider choices persisted in the manifest, so that different workstations cannot resolve different capability providers.
25. As a project maintainer, I want aliases persisted per surface, so that native naming collisions are resolved deliberately and reproducibly.
26. As a user, I want Packy to block ambiguous provider choices, so that it never chooses an arbitrary provider.
27. As a user, I want Packy to block unrepresentable selected resources, so that it never claims a partial observable contract is complete.
28. As a project maintainer, I want declared degradations shown in previews and locks, so that host-specific behavior remains reviewable.
29. As a multi-surface project, I want compatible hosts to share one physical projection, so that vendored resources are not duplicated unnecessarily.
30. As a multi-surface project, I want shared projections to track every contributor, so that removal preserves resources still in use.
31. As a project maintainer, I want mandatory notices materialized with the pack, so that vendoring preserves admitted attribution obligations.
32. As a security-conscious maintainer, I want projects limited to admitted catalog or history versions, so that a repository cannot introduce arbitrary source authority.
33. As a security-conscious collaborator, I want project configuration to contain only secret references, so that tokens and credentials never enter Git, locks, receipts, previews, or logs.
34. As a collaborator, I want OAuth and host-managed credentials to remain owned by the host, so that Packy does not duplicate sensitive state.
35. As a project maintainer, I want a complete dry-run before installation, so that every file, contribution, requirement, degradation, and blocker is visible.
36. As a project maintainer, I want existing foreign targets to block rather than be adopted, so that Packy cannot infer ownership from matching bytes.
37. As a project maintainer, I want composable files to preserve content outside exact Packy contributions, so that project guidance and configuration coexist safely.
38. As a developer with unrelated changes, I want Packy to operate in a dirty working tree when its targets remain safe, so that unrelated work does not block installation.
39. As a developer, I want concurrent target changes after preview to invalidate the plan, so that Packy never applies stale observations.
40. As a project maintainer, I want project mutation to be recoverable across multi-file failures, so that partial effects cannot be reported as success.
41. As a project maintainer, I want Packy to avoid Git staging, commits, resets, restores, and checkouts, so that source-control decisions remain mine.
42. As a project maintainer, I want manual edits to Packy-owned vendored resources reported as blocking drift, so that updates cannot overwrite team changes.
43. As a project maintainer, I want to reconcile the complete project from the manifest, so that merges, missing files, and fresh checkouts have one idempotent convergence command.
44. As a CI operator, I want to require an installed project contract without personal activation, so that CI can validate reproducibility in a sandbox.
45. As a local user, I want to require usable runtime state for one pack and surface, so that trust, authentication, requirements, activation, and conflicts are enforced.
46. As an offline collaborator, I want status, activation, deactivation, and exact uninstallation to work from the lock and vendored resources, so that routine lifecycle does not depend on the network.
47. As a collaborator with a missing vendored file, I want Packy to acquire only the exact admitted source or block, so that it never invents content from a digest.
48. As a user, I want a compatible global activation reported as inherited runtime coverage, so that Packy does not request redundant consent.
49. As a user, I want incompatible global and project contributions reported as scope conflicts, so that Packy does not hide host-specific precedence.
50. As a user, I want removing global coverage to leave project runtime pending, so that Packy never activates local effects automatically.
51. As a user, I want sensitive lock changes to make prior project activation stale, so that approved paths cannot silently execute new code.
52. As a user, I want declarative-only packs to report runtime activation as not required, so that status does not invent work.
53. As a user, I want project status to report installation and runtime activation independently, so that installed, pending, active, stale, blocked, inherited, and drifted states are unambiguous.
54. As a user, I want project deactivation to remove only personal runtime effects, so that it cannot uninstall shared resources.
55. As a maintainer, I want project uninstallation to remove shared intent and exact owned projections, so that it cannot alter unrelated or drifted content.
56. As a user with active runtime effects, I want uninstall to guide me through a separate deactivation first, so that personal registrations are not stranded.
57. As a collaborator who pulls an uninstall, I want remaining personal receipts reported as orphaned, so that cleanup is explicit and evidence-based.
58. As a user, I want unresolved recovery to block new project mutation, so that one failed transaction cannot be overwritten by another.
59. As a maintainer, I want project update to cover all installed surfaces, so that one pack cannot diverge by surface.
60. As a maintainer, I want surface-specific uninstall to remove only that contributor, so that other surfaces and dependencies remain intact.

## Implementation Decisions

- The capabilitypack module remains the sole semantic owner of global lifecycle, project installation, and personal project activation. It will reuse one catalog, resource graph, ownership model, sealed-plan lifecycle, readiness model, and complete surface-adapter seam.
- Host modules own global and project layout derivation, host-native representation, inspection, and projection application. The CLI module remains the command and composition adapter.
- The canonical project root is the nearest Git worktree root. Project mutation is unavailable outside a Git worktree.
- The shared project contract uses `packy.json`, `packy.lock.json`, `PACKY-NOTICES.md`, and host-native copied resource projections. Packy never stages or commits them.
- `packy.json` records direct pack intent, one exact version per pack, installed surfaces, selected resource roots, aliases, and provider choices. `packy.lock.json` records the immutable admitted source identity, complete transitive logical graph, bindings, degradations, contributors, fingerprints, modes, sensitive identities, and safe logical projection identities.
- Manifest and lock have independently versioned published schemas with minimum Packy capability requirements. Unsupported formats fail closed; migration is an explicit previewed mutation.
- Project installation is self-contained and cannot rely on global packs for dependency satisfaction. Admitted Pack Sources are the only dependency authority; arbitrary URLs and machine overrides are rejected.
- Project installation uses complete preflight, an immutable plan, staged exact content, backups, a durable journal, deterministic application, verification, and recovery-required when neither completion nor rollback can be proven. The lock is published only after projection verification.
- Committed lock data is portable ownership evidence only when the current adapter re-derives a permitted project target and exact content, contributors, and manifest relationship verify. Drift and foreign content block destructive mutation.
- Skills and other declarative resource trees are regular copied content. Source symlinks remain rejected by Pack Source admission. Required notices are mandatory closure members.
- Multiple surfaces share a physical projection only when they natively discover the same target and agree on its alias. Host-specific projections remain separate.
- Every selected operational resource requires a native binding or declared degradation. Missing external runtime requirements can leave activation pending but cannot justify omitted project intent.
- Project status has a shared installation axis and a personal runtime axis. Installed enforcement is offline, read-only, and CI-safe; usable enforcement includes personal activation and fresh host readiness.
- Personal project state lives beneath Packy Home under a digest of the canonical worktree root. Separate clones and worktrees do not share consent, and moved checkouts require reactivation.
- Project activation is sealed to sensitive lock identity. Secret values are never project data or Packy receipts. Compatible global runtime contributions may be inherited resource by resource; incompatible ones block readiness.
- `install` and `uninstall` are project-only commands. Existing lifecycle remains global by default; `--project` selects project activation, deactivation, status, or update. Project update operates on the complete pack and all installed surfaces.
- Interactive install may offer a separately previewed project activation. Non-interactive install never activates implicitly. Declarative-only packs do not require activation.
- Deactivation removes current-user runtime effects. Uninstallation changes shared project intent. Orphaned activation and unresolved recovery require explicit evidence-based cleanup.

## Testing Decisions

- The primary test seam is the sandboxed CLI acceptance seam: execute real commands from temporary Git worktrees with isolated workstation inputs, inspect actual project and personal state, and assert human output, structured output, exit behavior, previews, and filesystem effects.
- Good acceptance tests assert observable lifecycle behavior rather than private helper calls: exact project files and modes, preserved foreign content, absence of unauthorized effects, state transitions, recovery guidance, and repeatability.
- Capabilitypack facade tests cover invariants that require deterministic fault injection: sealed-plan freshness, compare-and-swap state, transaction fault points, contributor accounting, provider resolution, sensitive identity, and recovery outcomes.
- Surface-adapter contract tests cover host-specific project layouts, structural merges, exact removal, aliases, degradations, MCP and hook representation, and target containment for Codex, OpenCode, and Claude Code.
- Architecture tests continue to prove that capabilitypack is the only semantic caller of projection application and that the CLI does not acquire domain ownership.
- Structured-output schema tests validate every new human-independent report and reject canonical negative fixtures.
- Prior art includes the current CLI pack tracer tests, issue acceptance tests, surface adapter tests, structured-output schema tests, architecture ownership tests, and lifecycle recovery fault tests.
- Every ticket keeps focused tests and `go test ./...` green. Final qualification runs `./scripts/validate-packy.sh`, including race detection, as the repository validation authority.

## Out of Scope

- Project-local opt-outs that mask a compatible global activation.
- Arbitrary directories that are not Git worktrees.
- Floating version ranges, `latest`, branches, or moving source references.
- Arbitrary project-declared Pack Sources or per-machine source overrides.
- Different versions of the same pack per CLI surface.
- Automatic aliases, provider substitution, or silent resource omission.
- Automatic merging or adoption of manually modified vendored resources.
- Automatic Git staging, commits, resets, restores, or checkout cleanup.
- Storing secrets, OAuth tokens, credentials, or host trust decisions in project files.
- Automatic activation, deactivation, or personal cleanup triggered only by cloning or pulling Git changes.
- Automatic migration of personal activation after moving a checkout.
- Runtime installation without a valid project installation.
- A second public project-installation engine outside the capabilitypack module.

## Further Notes

- ADR 0026 defines additive global and project activation without project-local masking.
- ADR 0027 separates version-controlled project installation from personal runtime activation and refines the activation-only mutation rule for explicit project installation intent.
- The primary research compared official Codex, Claude Code, OpenCode, Vercel Skills, and GitHub CLI mechanisms. All three supported hosts provide versionable project surfaces, while trust, credentials, plugin caches, and runtime approval remain personal.
- Delivery should proceed as tracer-bullet tickets through the approved issue workflow. No ticket may weaken existing global behavior or bypass protected issue delivery and repository validation.
