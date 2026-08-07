# Packy

Packy is a lightweight macOS-first installer/configurator for explicit capability
packs. It discovers packaged capabilities and projects only the packs or resource
roots that an operator explicitly activates globally or installs in a project for
Codex, OpenCode, or Claude Code.

Packy is not a runtime orchestrator and does not copy workflow files into every project.

## Quickstart

Claude users must first install a stable Claude Code **2.1.203 or newer**.
Packy does not install or upgrade Claude Code. See the canonical
[Claude Code guide](docs/claude-code.md) for the global layout, migration,
readiness, preservation, and no-auth/no-model boundary.

Install the Packy binary from the Homebrew tap, initialize the package-installed
source, inspect the catalog, then preview and globally activate a chosen pack on
a chosen surface:

```sh
brew install yersonargotev/tap/packy
packy init
packy pack list
packy pack show engram
packy pack activate engram --surface codex --dry-run
packy pack activate engram --surface codex
packy pack status engram --surface codex
```

Installing a capability pack in a project is separate from installing or
upgrading the Packy binary. From inside a Git worktree, preview and apply the
shared, version-controlled project contract:

```sh
packy pack install <pack> --surface <surface> --dry-run
packy pack install <pack> --surface <surface>
```

Project installation does not grant personal runtime consent. When the pack has
runtime effects, activate them separately for the current user with
`packy pack activate <pack> --surface <surface> --project`. The detailed flow is
in the [project capability-pack lifecycle](docs/project-pack-lifecycle.md).

To activate only selected resource roots, repeat `--resource` for the roots the
surface should receive, preview the dependency closure, and then apply the same
selection explicitly:

```sh
packy pack activate <pack> --surface codex --resource <kind>:<id> --dry-run
packy pack activate <pack> --surface codex --resource <kind>:<id>
```

`packy init` is required for Homebrew/GitHub Release installs because package
managers install the binary only. It prepares Packy's Installed Source at
`~/.local/share/packy` and causes no CLI-surface changes. Catalog discovery is
also read-only: availability never grants activation intent. To upgrade the
binary later, use `brew upgrade packy` (or replace the GitHub Release binary),
rerun `packy init`, and explicitly preview `packy pack update` for each active
pack and surface. Maintainer release docs live in [docs/release.md](docs/release.md).

## v0 scope

Through explicit capability-pack activation, Packy v0 can manage:

- global skill symlinks under `~/.agents/skills`
- capability-pack activation and ownership state beneath `~/.packy`
- Codex instructions in `~/.codex/AGENTS.md`
- an OpenCode prompt file and reference under `$XDG_CONFIG_HOME/opencode`
- Claude Code global skills, instructions, and user-scoped Engram MCP setup
- separately approved external requirements and tool-owned host setup declared
  by an activated pack

Packy v0 is macOS-first. Linux and other agent adapters may be added later, but they are outside v0.

## Commands

```sh
packy init             # initialize the package-installed source checkout
packy doctor           # read-only core health and active-pack summary
packy pack list        # discover available packs without activation
packy pack show <pack>
packy pack install <pack> --surface <surface> --dry-run
packy pack install <pack> --surface <surface>
packy pack activate <pack> --surface <surface> --dry-run
packy pack activate <pack> --surface <surface>
packy pack update <pack> --surface <surface> --dry-run
packy pack status <pack> --surface <surface>
packy pack deactivate <pack> --surface <surface> --dry-run
packy pack deactivate <pack> --surface <surface>
packy pack uninstall <pack> --surface <surface> --dry-run
packy pack uninstall <pack> --surface <surface>
```

## Capability packs

Packy core remains available even when the optional `matty` capability pack is
inactive.

The selectable pack catalog currently contains `addy`, `argote`, `engram`, and `matty`.

The catalog supports the `codex`, `opencode`, and `claude` surfaces when a pack
explicitly declares them. Activation intent is per surface. A skill projected
to the standard shared `~/.agents/skills` target may become discoverable by
another compatible surface, but discovery is not activation on that surface.
Each activation that needs the shared projection remains a recorded contributor.

Before opting in, inspect the catalog and current host state without mutation:

```sh
packy pack list
packy pack show matty
packy pack status
packy pack status matty --surface codex
```

Then follow the explicit [manual capability-pack transition](docs/capability-packs.md).
It documents dry-run, typed approvals, readiness gating, update, reconcile,
recovery, and contributor-safe deactivation for all three supported surfaces.

For a version-controlled project dependency, follow the
[project capability-pack lifecycle](docs/project-pack-lifecycle.md). It covers
the guided installation/activation phases, Git and offline behavior, CI gates,
updates, shared projections, secrets, notices, recovery, and cleanup.

## Global paths

| Path | Purpose |
| --- | --- |
| `~/.agents/skills` | Packy-managed skill symlinks |
| `~/.packy/packs.json` | Capability-pack activation, contributor ownership, and recovery metadata |
| `~/.codex/AGENTS.md` | Codex prompt file containing Packy markers |
| `$XDG_CONFIG_HOME/opencode/opencode.json` | OpenCode config containing the Packy prompt reference |
| `$XDG_CONFIG_HOME/opencode/packy.md` | Packy-owned OpenCode prompt |
| `~/.claude/skills` | Claude personal-skill symlinks |
| `~/.claude/CLAUDE.md` | Global Claude instructions containing the Packy block |
| `~/.claude/agents` | Explicit Pack-owned Claude agent files |
| `~/.claude/settings.json` | Typed Pack-owned Claude command hooks |

If `XDG_CONFIG_HOME` is unset or relative, Packy uses `~/.config`.

## Safety model

- `init`, catalog discovery, and `doctor` do not activate packs or change CLI surfaces.
- `doctor` is read-only and separates Packy core health from active-pack readiness.
- `--dry-run` reports planned actions without writing files or running external commands.
- Capability-pack projections carry ownership for the explicit activation or project installation that contributed them; shared projections remain until their final contributor is removed.
- Deactivation removes only exact, unchanged projections owned by the selected activation and does not uninstall shared external executables.
- Packy warns about `gentle-ai:*` content but does not delete or rewrite Gentle AI-managed content.
- Tests use sandboxed `HOME` and `XDG_CONFIG_HOME`; they must not write to the operator's real home config.

## Out of scope for v0

- TUI installer or model picker
- runtime profile manager
- SDD workflow installation or SDD orchestrators
- repo-local docs/config by default
- Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters
- automatic Gentle AI cleanup or migration
- vendoring the Engram binary or installing a second copy under `~/.local/bin`; Homebrew owns Engram, and Packy only delegates setup/configuration
- installing only a tiny skill subset

## Verification

Use focused package tests while iterating. For a general local check, run the
sandboxed formatting, vet, and Go test path used by ordinary CI:

```sh
./scripts/validate-packy.sh
```

To select the changed Go packages and their reverse dependents automatically:

```sh
./scripts/validate-changed.sh             # base defaults to origin/main
./scripts/validate-changed.sh <base>      # optional branch, tag, or commit
```

The command reports `mode=focused` when it can safely format changed Packy Go
files and test their owning packages and reverse dependents (or skip tests for
documentation-only or empty changes). It reports `mode=general` and uses the
general path when the base or impact cannot be established safely, or a
cross-cutting or unknown path changed.

Focused checks target under ten seconds, general local validation under thirty
seconds, and required CI under three minutes. These are operational targets,
not wall-clock test assertions. CI adds the slower CLI and release packages plus
a race pass for the concurrent capability state store.
