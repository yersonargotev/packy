# Packy

Packy is a lightweight installer and configurator for reviewed capability
Packs. It installs selected Pack resources for Codex, OpenCode, and Claude Code
without becoming an agent runtime.

The bundled Pack manifests are the canonical selectable catalog. Run
`packy list` to inspect the Pack IDs and versions available in the current
binary; each Pack uses one current manifest. Browse the generated
[Pack catalog](docs/packs/index.md) for purpose and resource details.

## Quickstart

Claude users need stable Claude Code **2.1.203 or newer**. Packy does not install
or upgrade Claude Code; see the [Claude Code guide](docs/claude-code.md).

Install Packy, prepare its package content, inspect the catalog, then preview
and activate one Pack:

```sh
brew install yersonargotev/tap/packy
packy init
packy list
packy show engram
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
packy status engram --surface codex
```

Running `packy` without arguments opens the full-screen interactive interface
when both standard input and standard output are terminals. Redirected,
pipelined, and other non-interactive no-argument runs print ordinary textual
help without terminal control sequences. Explicit commands such as `packy
list`, `packy version`, and every `--help` or `--json` invocation always retain
their command behavior.

The interactive interface starts with workstation health and keeps global and
current-project Pack state visibly separate. Use the arrow keys or optional
`j`/`k` aliases to navigate, `Tab` and `Shift+Tab` to move between areas,
`Enter` to activate the focused control, `Esc` to go back, `?` for contextual
help, `r` to reload observed state, and `q` or `Ctrl+C` to quit. Mutations are
available only through focused controls and immutable previews; no global
single-key shortcut applies a change. During Apply, ordinary exit is deferred
until Packy finishes and reloads observed state.

The interface uses a one-column layout on narrow terminals and shows a minimum
size message when it cannot safely display review details. Text and structure
carry the meaning of every status and control, so color, icons, animation, and
mouse input are never required. The explicit command interface remains the
accessible alternative in terminals that cannot run the full-screen view.

Before replacing a `v0.1.x` installation, follow the warning-first
[one-time v0.2 reset](docs/reset-v0.2.md). Packy intentionally provides no
automatic migration or cleanup command.

## Project installation

From a Git worktree, preview and install a Pack into the project:

```sh
packy install <pack> --surface <surface> --dry-run
packy install <pack> --surface <surface>
```

Project installation writes reviewed project intent and one receipt per Pack
and surface. Personal runtime effects remain a separate activation:

```sh
packy activate <pack> --surface <surface> --project --dry-run
packy activate <pack> --surface <surface> --project
```

See the [project Pack lifecycle](docs/project-pack-lifecycle.md) for update,
status, activation, deactivation, and removal.

## Resource selection

Omitting `--resource` selects the complete Pack. Repeat `--resource` to select
specific resource roots plus their intra-Pack dependencies:

```sh
packy activate <pack> --surface codex \
  --resource <kind>:<id> --dry-run
packy activate <pack> --surface codex \
  --resource <kind>:<id>
```

## Commands

```text
packy init
packy doctor
packy list
packy show <pack>
packy activate <pack> --surface <surface>
packy install <pack> --surface <surface>
packy update <pack> --surface <surface>
packy status [<pack>] [--surface <surface>]
packy deactivate <pack> --surface <surface>
packy uninstall <pack> [--surface <surface>]
```

Inspection and `--dry-run` do not mutate Pack state or CLI surfaces. Lifecycle
commands operate on one Pack and receipt at a time. Update targets the current
bundled Pack version; arbitrary versions and downgrades are unsupported.

## State and safety

Packy keeps workstation state beneath `~/.packy`. A successful global
activation or project installation records an installed Pack receipt with the
Pack identity and version, surface, selected resource closure, projected paths,
and content digests.

Before mutation, Packy checks receipt-owned paths for drift and checks proposed
paths for collisions. Ordinary update, deactivation, and removal stop without
writing when owned content changed. An explicit force option can affect only
paths already owned by the targeted receipt. Distinct Pack resources never
share ownership of one projected path.

Packy preserves unrelated files, credentials, Engram memory, and host-owned
configuration. Tests and manual verification must sandbox `HOME` and
`XDG_CONFIG_HOME`.

## Pack authoring

To add a Pack, copy [the Pack template](bundle/pack-template/README.md), edit
its reviewed content and one manifest, choose the Pack SemVer, and run the
focused validator. Full authoring details are in
[Capability Packs](docs/capability-packs.md).

## Verification

Run focused tests for touched behavior while iterating. Validate one Pack with:

```sh
./scripts/validate-pack-content.sh <pack-id>
```

Run the sandboxed general repository check with:

```sh
./scripts/validate-packy.sh
```

Run the focused dead-code and ineffectual-assignment check directly with:

```sh
./scripts/validate-static-analysis.sh
```

The general repository check includes this focused static analysis.

`./scripts/validate-changed.sh [base]` selects focused Go packages when safe
and falls back to the general validation path for cross-cutting changes.

Maintainer publication uses the [immutable release flow](docs/release.md).
