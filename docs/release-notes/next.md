# {{TAG}} — Explicit capability-pack activation

This release makes capability-pack lifecycle the sole authority for every
Codex, OpenCode, and Claude Code projection. The cutover is intentionally
incompatible: the former root lifecycle commands are removed with no aliases,
automatic migration, or adoption of their state.

## Operator transition

The sole current operator must remove the old installation's classic artifacts
manually before using this release. Packy will neither read nor delete old
classic state, prompts, links, MCP configuration, or ownership records; any
leftovers are unowned and normal collision protections preserve them.

After manual cleanup, install or upgrade the binary and start fresh:

```sh
brew upgrade packy
packy init
packy pack list
packy pack show engram
packy pack activate engram --surface codex --dry-run
packy pack activate engram --surface codex
packy pack status engram --surface codex
```

Repeat activation explicitly for each desired pack, resource selection, and
surface. Initialization and catalog inspection cause no surface effects.

## Repository controls

- Packy's required checks now protect `main` through the reviewed pull-request
  path, with force pushes and branch deletion denied.
- Release and Homebrew authority remain separated behind their protected
  environments. The tap credential exists only in the `homebrew` environment;
  there is no repository-level fallback credential.
- Version tags are protected from routine movement and deletion, and this is
  the first Packy version published after future-release immutability is
  enabled.

## Publication integrity

- The release is built once from one exact protected-main commit and carries
  deterministic SHA-256 checksums, an SPDX SBOM, and verified provenance.
- GitHub publication completes and is independently read back before the
  separately approved Homebrew update can begin.
- Published release assets, body, and associated version tag are immutable
  workflow outputs. Normal correction uses a new monotonically increasing
  version. The platform still exposes residual Owner capability to edit a title
  or notes, but Packy's publication and recovery paths never use it; destructive
  release deletion is an Owner-only break-glass action and permanently prevents
  reuse of the deleted version tag.

## Operator impact and limitations

- Claude Code stable **2.1.203 or newer** remains the supported floor, and
  existing installations remain on **state schema v2**.
- **matty 3.0.0** has a complete Claude Code contract.
- **engram 2.0.0** remains **degraded** on Claude Code where generic lifecycle
  translation is unsupported.
- Pack health now distinguishes Packy core health from readiness of active packs.
- A shared projection can be discovered by another compatible surface without
  activating the pack there; each surface's intent remains explicit.
- Packy remains macOS-first. Darwin Homebrew installs are the supported user
  path; Linux artifacts remain published for future support.
