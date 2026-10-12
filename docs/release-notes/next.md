# {{TAG}} — Isolated Catalog transitions and Git-safe project skills

## Changes since the previous release

The previous stable release is `v0.2.27`.

- `packy catalog candidate` now supports explicit baseline-to-candidate
  transitions within one owned disposable workspace. `transition
  --expect-archive-sha256 <current-digest>` validates and retains the proposed
  snapshot without changing installed projections, receipts, or project intent.
  Retained baseline snapshots continue resolving surfaces awaiting an update.
- Candidate workspaces support targeted project updates and isolated global
  `activate`, `update`, and `deactivate` operations. Global writes stay under
  the workspace's disposable home and configuration roots. Normal selection,
  effective dependencies, coexistence, ownership, drift checks, and interactive
  lifecycle consent remain enforced; JSON output does not grant consent.
- Candidate lifecycle operations revalidate workspace ownership, the Git
  boundary, and every retained source, including after interactive approval.
  Changed ownership markers, redirected roots, stale transition identities,
  incompatible inputs, and retained-source drift fail without lifecycle mutation.
  Resources remain inert; candidates do not select official catalogs, fabricate
  attestations, authenticate hosts, execute models, or offer personal project
  activation.
- Disposable candidate workspaces now use ownership marker schema version 2.
  Create a new workspace for older candidates; their previous workspace format
  is rejected. `transition --json` has a documented versioned report for
  success and rejection.
- Project skill directories use `0755`, including Claude composite skills, so
  sealed project receipts remain valid after a Git commit and fresh checkout.
  Personal Claude composite skill directories retain `0700`.
- The Packy contributor project now tracks the complete Matty 4.0.0 Pack for
  Codex and Claude Code, with shared intent, lock receipts, skill projections,
  and its upstream MIT notice.
- Native release contracts now cover retained candidate transitions and both
  project and disposable global lifecycles on Darwin/Linux amd64/arm64.

## Catalog adoption

See the [Catalog command documentation](https://github.com/yersonargotev/packy/blob/main/docs/catalog-project.md)
for supported syntax, JSON reports, process/PTY requirements, and isolation and
trust boundaries. Catalog content releases remain independent of Packy releases;
this engine release does not publish or select new Catalog content automatically.
Downstream adoption of this supporting release requires its own reviewed Catalog
release declaration before retiring legacy probes.

For a new installation, acquire the official catalog and preview activation
before changing a CLI surface:

```sh
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the engine's supported floor.
