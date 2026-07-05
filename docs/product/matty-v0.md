# Matty v0 product scope

Matty v0 is a macOS-first installer/configurator for a lightweight AI coding workflow.
It wires global Matt Pocock-style skills, Engram memory, and small Codex/OpenCode
prompt layers without becoming an always-on runtime orchestrator.

## Quick path

1. Use `matty install` to apply the golden-path setup.
2. Use `matty doctor` to inspect setup health without mutations.
3. Use `matty update` to refresh Matty-managed artifacts.
4. Use `matty uninstall` to remove only Matty-owned artifacts.

## Problem

The useful parts of Gentle AI, Matt Pocock skills, and Engram are valuable, but
stitching them together manually creates repeated config work and a heavy
always-on prompt surface. Matty makes that setup repeatable, inspectable,
updateable, and reversible while keeping startup instructions small.

## Product boundary

| Area | v0 scope |
| --- | --- |
| Role | Installer/configurator, not a runtime orchestrator. |
| Platform | macOS-first. Linux may be considered later but is not promised for v0. |
| CLI surfaces | Codex and OpenCode only. |
| Skills | Curated global bundle exposed as symlinks under `~/.agents/skills`. |
| Memory | Engram installed/updated through official mechanisms and configured through `engram setup`. |
| State | Small Matty state file at `~/.matty/config.json`. |
| Prompts | Small Matty-owned global prompt/config blocks only. |
| Safety | Preserve user, Engram, and Gentle AI content outside Matty markers. |

## User outcomes

- One command configures the preferred Codex/OpenCode workflow.
- Repos are not polluted with copied skills or local prompt files by default.
- Repeated install/update runs are idempotent.
- `doctor` is safe and read-only.
- `uninstall` removes Matty-owned symlinks, marker blocks, prompt entries/files, and state without uninstalling Engram or deleting Gentle AI content.

## Implemented v0 slices

| Slice | Outcome |
| --- | --- |
| 01 scaffold | Go+Cobra CLI with `install`, `doctor`, `update`, and `uninstall`, plus sandboxable command execution. |
| 02 state/dry-run | Minimal Matty state and dry-run planning for file, symlink, and external-command actions. |
| 03 skill symlinks | Global skill bundle synchronization under `~/.agents/skills`. |
| 04 Engram lifecycle | Homebrew-backed Engram install/update planning and delegated `engram setup codex/opencode`. |
| 05 Codex prompt | Matty-owned Codex marker blocks that preserve user, Engram, and Gentle AI content. |
| 06 OpenCode prompt | Matty prompt/config merge for OpenCode without clobbering existing JSONC config. |
| 07 doctor | Read-only health checks with actionable pass/warn/fail output. |
| 08 lifecycle | Complete idempotent update and safe uninstall behavior. |
| 09 hardening | README docs and end-to-end sandbox lifecycle tests. |

## Out of scope for v0

- TUI installer or model picker.
- Runtime profile manager.
- SDD workflow installation or SDD orchestrators.
- Repo-local docs/config by default.
- Claude Code, Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters.
- Automatic Gentle AI cleanup or migration.
- Vendoring the Engram binary.
- Installing only a tiny skill subset; v0 controls tokens through lazy routing.

## Verification

The repo-level verification remains:

```bash
go test ./...
```

Before using Matty against a real HOME, run the lifecycle in a sandboxed
HOME/config environment and verify `install`, `doctor`, `update`, and `uninstall`
behavior end to end.
