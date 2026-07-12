# Capability packs and manual transition

Capability packs are opt-in additions managed by **Matty core**. Matty core is
the always-available `matty` installer/configurator; it is independent of the
optional pack named `matty`. Leaving that pack inactive—or deactivating it—does
not remove the CLI or prevent pack inspection and management.

The initial Matty-owned catalog is deliberately limited:

| Pack | Purpose |
| --- | --- |
| `matty` | Matty workflow skills and behavioral guidance |
| `engram` | Engram memory guidance, MCP declaration, and lifecycle intent |

Both packs support only the `codex` and `opencode` surfaces. Remote and
third-party sources, marketplaces, signing, `web`, `mobile`, version selection,
downgrades, unattended Apply, and background runtime management are not
supported.

## Inspect before changing anything

Discovery and status are inspection-only. Run these before deciding whether to
activate a pack:

```sh
matty pack list
matty pack show matty
matty pack show engram
matty pack status
matty pack status matty --surface codex
matty pack status engram --surface opencode
```

The overview covers every pack/surface pair. Targeted status adds activation
intent, the latest reconciliation attempt, projection evidence, blockers,
readiness, and pending human actions.

## Manual transition from pre-pack Matty

There is **no automatic migration** from the earlier `matty install` model.
Matty does not translate old state, write transition configuration, or adopt
existing files merely because they resemble pack output. Inspect first, then
explicitly preview and activate the pack on each surface you choose:

```sh
matty pack activate matty --surface codex --dry-run
matty pack activate matty --surface codex

matty pack activate matty --surface opencode --dry-run
matty pack activate matty --surface opencode
```

`--dry-run` creates only a fresh Preview: it requests no approval and performs no
mutation. Apply requires an interactive terminal. The rendered plan separates
reversible local work, executable/external effects, and destructive cleanup;
Matty asks for a plan-bound approval for each required consent kind. There is no
generic `--yes` approval.

If status reports unmanaged, ambiguous, or drifted content, preserve it and
resolve the reported pending human action. Activation does not silently adopt
such content. Do not delete or rewrite the old installation merely to force an
Apply; decide explicitly which content remains user-managed.

## Lifecycle examples

Every mutation is scoped to one surface; activating on Codex never implies
activation on OpenCode.

```sh
# Explicit activation
matty pack activate engram --surface codex --dry-run
matty pack activate engram --surface codex

# Update means the current version in the local Matty-owned catalog
matty pack update engram --surface codex --dry-run
matty pack update engram --surface codex

# Repair one active pack, or all active packs on one surface
matty pack reconcile engram --surface codex --dry-run
matty pack reconcile --surface codex --dry-run

# Contributor-safe removal
matty pack deactivate engram --surface codex --dry-run
matty pack deactivate engram --surface codex
```

Approvals are tied to the exact immutable plan and consent phase. If Apply
rejects a stale plan, it executes no actions. Inspect the changed precondition,
then repeat the same verb (for example, `matty pack update engram --surface
codex`) to obtain a fresh Preview and provide fresh approvals. Matty does not
retry automatically.

After a partial attempt marked `recovery-required`, repeat the originating
`activate`, `update`, or `deactivate` command. Matty freshly inspects the host and
previews a recovery plan; it does not replay the historical plan or reuse its
approvals.

## Apply success is not readiness

A verified Apply can exit successfully while authorization, trust, reload, or
runtime loading is still pending. Readiness is derived separately from fresh host
observation:

- **configured**: the Matty-owned projections are reconciled;
- **authorized**: required login, trust, and permissions are observed complete;
- **usable**: the host has loaded the capability under its runtime permissions.

Inspect pending actions and use the independent automation gate:

```sh
matty pack status engram --surface codex
matty pack status engram --surface codex --require usable
```

The second command exits nonzero until usability is freshly observed. Login,
trust, permissions, reload/restart, runtime loading, and external installation or
setup are human/host boundaries. Matty reports them as pending human actions; it
does not represent them as generic approvals, receipts, or verified Apply
actions, and it does not complete them automatically.

## Ownership and cleanup limits

Matty updates or removes only projections whose ownership and unchanged
fingerprint it has verified. It preserves unmanaged, ambiguous, and drifted
content for human review. Shared resources remain while any active pack is still
a contributor; destructive cleanup is limited to unchanged, verified,
last-contributor projections and requires its own typed approval.

Deactivation never deletes host credentials, Engram memory (including Engram's
data directory), or external data. Logout, credential removal, and external-tool
uninstallation remain separate actions owned by the relevant host or tool.
