Status: ready-for-agent

# PRD: Matty v0 installer/configurator

## Problem Statement

The user wants the useful parts of Gentle AI without the heavy always-on prompt, SDD-first workflow, and habit of copying managed files into many places. They want a lightweight tool named Matty that sets up a global AI coding workflow using Matt Pocock skills, Engram memory, and subagent delegation conventions for Codex and OpenCode first.

Today, assembling that stack manually means coordinating separate tools and global config surfaces: installing skills, installing/configuring Engram, wiring Codex/OpenCode prompts, and checking conflicts with existing Gentle AI config. That setup should be repeatable, inspectable, updateable, and reversible.

## Solution

Matty v0 is a macOS-first Go+Cobra CLI that acts as an installer/configurator, not a runtime orchestrator. It installs a curated global skill bundle as symlinks under `~/.agents/skills`, installs/updates Engram through official mechanisms, delegates Codex/OpenCode Engram wiring to `engram setup`, injects only small Matty-owned global prompt blocks, and records minimal state in `~/.matty/config.json`.

The default workflow is non-interactive:

- `matty install` applies the golden path.
- `matty doctor` verifies the setup without mutating files.
- `matty update` refreshes Engram, skill links, prompts, and Matty state.
- `matty uninstall` removes only Matty-managed artifacts.

Matty coexists with Gentle AI by detecting `gentle-ai:*` managed content and warning about possible duplicate instructions, but it never deletes or rewrites Gentle AI content by default.

## User Stories

1. As a developer, I want one command to configure Codex and OpenCode for my preferred AI workflow, so that I do not manually stitch together skills, prompts, and Engram.
2. As a developer, I want Matty to install skills globally, so that my project repos are not polluted with copied skill files.
3. As a developer, I want skills exposed through symlinks, so that updates are simple and duplicate content is avoided.
4. As a developer, I want Matty to install Engram, so that the memory layer is present when Matty configures prompts that refer to it.
5. As a developer, I want Matty to update Engram, so that I can keep the memory integration current from the same tool.
6. As a Codex user, I want Matty to add a tiny global instruction layer, so that Codex knows to use global skills and Engram without loading a huge prompt.
7. As an OpenCode user, I want Matty to add equivalent global configuration, so that OpenCode follows the same workflow conventions.
8. As a developer, I want Matty to reuse `ask-matt` as the skill router, so that it does not invent a separate router when Matt Pocock's flow already exists.
9. As a developer, I want Matty to install the full chosen skill bundle, so that I can use engineering, productivity, loop-me, and wayfinder workflows.
10. As a developer, I want Matty's always-on prompt to stay small, so that startup token cost remains materially lower than Gentle AI.
11. As a developer, I want Matty state in one global file, so that doctor/update/uninstall can know what Matty manages.
12. As a developer, I want `matty doctor` to be read-only, so that I can inspect setup health safely.
13. As a developer, I want `matty install --dry-run`, so that I can preview global changes before applying them.
14. As a developer, I want Matty to delegate Engram's Codex/OpenCode setup to Engram, so that Matty does not duplicate fragile agent-specific logic.
15. As a developer with Gentle AI already installed, I want Matty to warn but not delete anything, so that I can migrate intentionally.
16. As a developer, I want uninstall to remove only Matty-owned artifacts, so that it is safe and predictable.
17. As a maintainer, I want tests to use sandboxed HOME paths, so that development never mutates real user configuration.
18. As a maintainer, I want idempotent install/update behavior, so that repeated runs do not duplicate prompt blocks or symlinks.
19. As a maintainer, I want a clear command structure, so that future CLI surfaces can be added later without expanding v0 scope.
20. As a future Matty user, I want Claude Code, Antigravity, and GitHub Copilot CLI to remain possible future adapters, so that v0 decisions do not close that door.

## Implementation Decisions

- Matty v0 is a Go CLI using Cobra.
- v0 is macOS-first. Linux compatibility may be designed for but is not promised.
- Matty is an installer/configurator, not an always-on runtime orchestrator.
- Matty uses a global-first configuration model: skills and agent prompts are managed in global user config surfaces, not copied into every repository.
- Matty owns a small state file at `~/.matty/config.json`; it stores metadata such as Matty version, managed skill names, skill source targets, configured CLI surfaces, and last update/check metadata. It must not become a large prompt store.
- The default skill bundle is all Matt Pocock `engineering` skills, all `productivity` skills, plus selected in-progress skills `loop-me` and `wayfinder`.
- Skills are materialized as symlinks under `~/.agents/skills`.
- The small Matty global prompt reuses `ask-matt` as the router rather than creating a new `matty-router` skill in v0.
- Engram is installed and updated by Matty using official Engram mechanisms; on macOS the first supported mechanism is Homebrew.
- Codex/OpenCode Engram integration is delegated to official `engram setup codex` and `engram setup opencode` commands.
- Matty prompt injection uses Matty-owned managed markers such as `<!-- matty:skills-router -->` and only updates those blocks.
- Matty must not edit or remove `gentle-ai:*` or Engram-owned content; it only detects and warns.
- `matty install` is non-interactive by default and supports `--dry-run`.
- `matty doctor` is read-only.
- `matty update` refreshes managed state/config without duplicating artifacts.
- `matty uninstall` removes Matty-managed symlinks, Matty marker blocks, and Matty state; it does not uninstall Engram or Gentle AI by default.

## Testing Decisions

- Tests must use sandboxed HOME/config paths and must never write to the operator's real home directory.
- The highest useful seam is the CLI command boundary with injected filesystem paths and command runner abstractions, allowing install/doctor/update/uninstall to be tested end-to-end in a temporary HOME.
- Unit tests should cover marker block insertion/update/removal, symlink planning/application, state file read/write, conflict detection, and command-runner decisions.
- Integration-style tests should run commands against a temporary HOME and fake external binaries/command runners where needed.
- Good tests assert external behavior: created files, symlink targets, command plan output, doctor statuses, idempotency, and dry-run non-mutation. They should not overfit internal helper function structure.
- Prior art can be borrowed from Gentle AI and Engram tests for sandboxed HOME, managed markers, config merging, and setup command verification.

## Out of Scope

- TUI installer or model picker.
- Runtime profile manager.
- SDD workflow installation or SDD orchestrators.
- Repo-local docs/config by default.
- Claude Code, Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters in v0.
- Automatic Gentle AI cleanup or migration.
- Vendoring the Engram binary.
- Installing only a tiny skill subset; v0 installs the chosen full bundle and controls tokens through lazy routing.

## Further Notes

- Matty should be explicit about what it manages and should make rollback safe.
- The first implementation should scaffold the CLI and test harness before touching real integration behavior.
- Every issue should be implementable in a fresh session using this PRD plus that issue file.
