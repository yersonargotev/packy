# {{TAG}} — Supported Catalog tooling

Catalog maintainers can validate, construct, publish, and exercise reviewed
Catalog content using the installed Packy executable, without a Packy source
checkout or Go runtime. Each release binary identifies its version and exact
source commit, and publication requires the complete functional contract to
pass on all four supported release targets.

## Changes since the previous release

- `packy catalog validate` validates a complete Catalog Project and optional
  `--baseline`, including provenance, exact-copy content, and runtime fitness.
  It keeps resources inert and provides readable output and versioned `--json`
  results with identifiable errors and reliable exit statuses.
- `packy catalog build` constructs complete snapshot assets with the executing
  binary's embedded builder identity, preserving baseline enforcement,
  input-change detection, and deterministic construction. Callers cannot
  independently relabel the builder.
- `packy catalog publish` publishes or verifies prepared immutable Catalog
  Snapshots through authenticated `gh`, preserving unchanged retry and draft
  recovery with versioned JSON. Official preparation and attestation remain
  workflow-owned.
- `packy catalog candidate` exercises complete local snapshot assets through
  the normal executable in an explicitly disposable authoring workspace.
  Preview, project install, verification, and uninstall preserve interactive
  consent and ownership; local candidates never select an official catalog or
  enable host execution, global operations, or personal activation.
- `packy version --json` exposes the release version and exact source commit.
  Release builds compile the recorded Git tree and validate embedded identity.
- Release publication requires the same full Catalog CLI contract to pass
  against each staged Darwin/Linux amd64/arm64 binary on a matching native
  runner. The tested archives are published unchanged; workflow artifacts retain
  contract results, identities, and checksums for Catalog adoption.
- Source-only Catalog validation/construction tools and the publisher script
  are removed. Use the supported `packy catalog` commands instead.

## Catalog adoption

See the [Catalog command documentation](https://github.com/yersonargotev/packy/blob/main/docs/catalog-project.md)
for arguments, JSON contracts, publication authority, and candidate-workspace
limits. Catalog content releases remain independent of Packy releases. This
engine release does not publish or select new Catalog content automatically.

For a new installation, acquire the official catalog and preview activation
before changing a CLI surface:

```sh
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the engine's supported floor. The
separate discovery evidence records the exact newer binaries exercised; it
does not turn that floor into a claim of verified workflows on every version.
