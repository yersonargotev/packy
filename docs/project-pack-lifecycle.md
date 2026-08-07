# Project capability-pack lifecycle

Project installation and personal runtime activation are separate contracts.
Installation is shared through Git; activation records only the current user's
consent and workstation effects. Cloning or pulling a project never grants
trust, authenticates an account, exposes a secret, runs a hook, or mutates
personal host configuration.

## Guided two-phase flow

From anywhere inside a Git worktree, preview and install a Pack from the
current bundled catalog for a CLI surface:

```sh
packy pack install matty --surface codex --dry-run
packy pack install matty --surface codex
```

The interactive install applies the shared project contract first. If the
installed closure has personal runtime effects, Packy then offers a separately
previewed activation with its own approvals. Declining activation leaves a
successful installation and prints the exact command to continue later:

```sh
packy pack activate matty --surface codex --project --dry-run
packy pack activate matty --surface codex --project
```

Non-interactive installation never activates. A declarative-only pack reports
runtime activation as `not-required` and does not create empty personal state.

Installation writes reviewable, version-controlled files at the nearest Git
worktree root:

- `packy.json` records every direct Pack plus its surface, resource, and alias
  intent.
- `packy.lock.json` records one independent receipt per Pack and surface. Each
  receipt contains the exact Pack identity, selected resources, projected
  targets, digests, modes, contributors, and any personal-activation
  disclosures needed to inspect, activate, or remove that Pack. The lock has
  no project-wide graph or lifecycle-plan fields.
- `PACKY-NOTICES.md` carries mandatory Pack Source licensing and attribution
  contributions.
- Host-native project paths contain copied declarative resources and exact
  marked contributions.

Packy never stages, commits, resets, restores, or checks out Git content. It
may operate beside unrelated working-tree changes, but foreign targets, owned
drift, unsafe paths, and changes made after preview block mutation.

## Multiple Packs, selections, aliases, and surfaces

Omitting `--resource` installs the complete pack. Repeat `--resource` to select
operational roots; Packy locks their complete dependency, asset, and notice
closure:

```sh
packy pack install example-pack --surface codex \
  --resource skill:build \
  --alias skill:build=project-build \
  --dry-run
```

Aliases are explicit project intent. Packy never invents a collision name or
satisfies a project dependency from global workstation state. Install another
Pack with another `pack install` command; it receives a separate manifest
entry and receipt. There is no project-wide provider graph or cross-Pack
transaction.

Codex and OpenCode may contribute to one physical `.agents/skills` projection.
Its receipts record every contributor, and removing one surface preserves the
projection until its final contributor is removed. Claude Code retains its
host-native project representation. Add another surface with another install;
all installed surfaces share one exact pack version.

Update reconciles one Pack to the current bundled catalog across its installed
surfaces. Ordinary update blocks owned drift; `--force` restores only the
targeted receipt's owned content:

```sh
packy pack update example-pack --project --dry-run
packy pack update example-pack --project
packy pack update example-pack --project --force
```

## Status and automation

Project status always reports shared installation and personal runtime as two
independent axes:

```sh
packy pack status --project
packy pack status example-pack --surface codex --project --json
```

Installation is `absent`, `installed`, `drifted`, or `blocked`. Runtime is
`not-required`, `pending`, `active`, `inherited-global`, `stale`, `orphaned`,
`recovery-required`, or `blocked`. Compatible global effects may provide
`inherited-global` coverage without creating project receipts. Incompatible
global and project contracts produce an activation-scope conflict; Packy does
not hide it with precedence.

CI should validate the version-controlled contract without personal state:

```sh
packy pack status --project --require installed --json
```

Local workflows that need runtime behavior can require one pack and surface to
be usable:

```sh
packy pack status example-pack --surface codex --project --require usable --json
```

`usable` additionally requires fresh personal or inherited coverage, trust,
authentication, external requirements, and host readiness. It is intentionally
not a portable CI requirement.

## Offline operation, secrets, and recovery

Status, installed enforcement, personal activation and deactivation, and exact
owned uninstall use only the manifest, lock, bundled resources, and personal
receipts. They remain available offline. Packy never invents bytes from a
digest.

Project files may contain secret references such as environment-variable names,
but never secret values, OAuth tokens, credentials, or host trust decisions.
Previews, JSON reports, receipts, logs, fingerprints, and recovery evidence
follow the same rule. Authentication and OAuth remain host-owned personal work.

Project mutation journals the complete transaction before effects and publishes
the lock last. If Packy cannot prove either completion or rollback, status
reports `recovery-required`; new project mutation is blocked. Repeat the
originating mutation command to recover first and then obtain a fresh preview.
Structured mode emits a `project-recovery` event before the new preview.

## Deactivation, uninstall, and orphan cleanup

Personal deactivation removes only the current checkout's exact receipted
runtime contributions:

```sh
packy pack deactivate example-pack --surface codex --project --dry-run
packy pack deactivate example-pack --surface codex --project
```

Uninstall changes shared project intent. If the current user is active, the
interactive flow separately previews and approves personal deactivation before
shared removal:

```sh
packy pack uninstall example-pack --surface codex --dry-run
packy pack uninstall example-pack --surface codex
packy pack uninstall example-pack
```

A surface uninstall removes only that contributor; omitting `--surface`
removes the complete pack. Pulling somebody else's uninstall never mutates
personal configuration. Remaining receipts become `orphaned` and preserve the
exact evidence needed for explicit deactivation. Drifted personal content is
preserved and blocks cleanup; receipts retire only after verified absence.

## Global compatibility

Existing unqualified lifecycle commands remain global even inside a Git
worktree. `activate`, `deactivate`, `status`, and `update` select personal
project behavior only with `--project`; `install` and `uninstall` are inherently
project operations. Global and project contributions compose additively, and a
project cannot mask a global activation.

## Structured contracts and acceptance evidence

The immutable published project schema suite is under
`schemas/project/v1.0.0/`. It covers the manifest, lock, status, preview, apply,
failure, and recovery documents. Canonical negative fixtures live under
`internal/cli/testdata/project-contract/v1.0.0/negative/`; repository tests
compile the checked-in schemas offline, validate live producers, reject every
negative fixture, and assert that paths and secret-shaped values are absent.

The sandboxed CLI acceptance series `issue451` through `issue463`, plus the
multi-Pack receipt coverage for issue `519`, covers full and selected installs,
aliases, shared contributors, all three host representations, targeted updates,
guided activation, inherited and conflicting global coverage, stale
activation, uninstall, orphan cleanup, and recovery. `internal/capabilitypack`
fault tests prove sealed-plan and journal recovery invariants. Architecture
tests prove that `capabilitypack` remains the sole semantic caller of projection
application while Codex, OpenCode, and Claude Code retain project
representation ownership.
