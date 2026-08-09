# Project Pack lifecycle

Project installation and personal runtime activation are separate. Installation
writes reviewed project files; activation records only the current user's
consent and workstation effects. Cloning a project never grants trust,
authenticates an account, runs a hook, or mutates personal configuration.

## Install

From a Git worktree, preview and install one Pack for one surface:

```sh
packy install matty --surface codex --dry-run
packy install matty --surface codex
```

The project root receives:

- `packy.json`, the human-authored direct Pack and resource intent;
- `packy.lock.json`, one generated installed Pack receipt per Pack and surface;
- `PACKY-NOTICES.md`, reviewed notices required by installed resources; and
- the selected host-native projections.

Packy never stages or commits these files. Review and commit them with the
project change.

Omitting `--resource` installs the complete Pack. Repeat `--resource` to select
roots plus their intra-Pack dependencies. Install another Pack with another
command; it receives an independent manifest entry and receipt.

## Activate personal runtime effects

When an installed Pack has personal runtime effects, preview and activate them
separately:

```sh
packy activate matty --surface codex --project --dry-run
packy activate matty --surface codex --project
```

Non-interactive installation does not activate personal effects. Global and
project activations remain explicit and independently inspectable.

## Update and status

Update one project Pack to the version in the running Packy's bundled catalog:

```sh
packy update matty --surface codex --project --dry-run
packy update matty --surface codex --project
```

Each installed surface keeps its own exact Pack version. Updating one surface
leaves every other surface unchanged and still eligible for its own previewed
update.

Inspect shared installation and personal activation independently:

```sh
packy status --project
packy status matty --surface codex --project --json
```

Every update preflights all receipt-owned projections. Drift or a target
collision stops the operation before any write. `--force` remains limited to
paths in the targeted receipt.

## Deactivate and uninstall

Personal deactivation removes only unchanged runtime projections in the
selected personal receipt:

```sh
packy deactivate matty --surface codex --project --dry-run
packy deactivate matty --surface codex --project
```

Uninstall changes shared project intent:

```sh
packy uninstall matty --surface codex --dry-run
packy uninstall matty --surface codex
```

Omitting `--surface` removes the complete Pack declaration. Removing one Pack
does not alter another Pack's files or receipt. Changed or unrelated files are
preserved.

## Automation

Project JSON reports use the checked-in `schemas/project/v1.0.0/` suite. CI can
inspect the committed contract without using personal state:

```sh
packy status --project --require installed --json
```

Tests and manual checks that resolve project or workstation paths must sandbox
`HOME` and `XDG_CONFIG_HOME`.
