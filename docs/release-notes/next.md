# {{TAG}} — Reviewed surface variants

Pack resources can retain one logical identity while selecting reviewed
surface-specific content, dependencies, and typed execution settings. Supported
surfaces inherit the common definition when no variant is declared; invalid
variants fail validation. Preview and inspection expose the selected definition
and effective provenance.

## Changes since the previous release

- Pack manifests and Catalog Snapshots use schema v3. Complete catalog
  validation includes common content and every variant's source and legal
  closure; this is a clean cut without a legacy reader or converter.
- OpenCode skills use native global `$XDG_CONFIG_HOME/opencode/skills`
  (defaulting to `$HOME/.config/opencode/skills`) and project `.opencode/skills`
  roots. Divergent same-name compatibility discovery with Codex/Claude remains
  blocked when isolation is unverified; separate paths alone do not prove host
  usability. See [discovery evidence and limits](../skill-discovery.md).
- Structured reports advance to show v7, global status/lifecycle v13, and
  project installation/update preview v4. Installed receipts retain their
  minimal content-integrity contract.
- `packy catalog import --variant-surface <surface>` adds a reviewed variant
  to an existing logical resource while preserving its common definition.
  Upstream Refresh includes variant origins, identifies adaptations by surface
  for reconciliation, and protects maintained adaptations and overlapping
  source trees from unintended replacement.
- Project notice contributions include the selected legal text as well as
  license and attribution. Notice variants must preserve the original legal
  text, license, and attribution.
- Receipt-owned MCP configuration can be updated globally and in projects for
  OpenCode, and in Claude projects, without being mistaken for unmanaged
  content; foreign entries and owned drift still block. Initial Codex project
  MCP installation and Claude
  project fingerprint verification are also corrected.

## Adoption before upgrading

Keep the previous engine and its referenced snapshots. Use it to inventory,
preview, deactivate old global/personal activations, and uninstall affected
project installations in every worktree before replacing the binary. This
includes old Codex/OpenCode shared `.agents/skills` installations. Follow the
[clean adoption procedure](../catalog-adoption.md); do not move or delete shared
skill directories wholesale. Personal data, credentials, Memory, unmanaged
content, and other hosts' installations remain protected.

The rollout requires an immutable engine release followed by complete catalog
validation and publication pinned to that engine. These notes do not announce
that catalog migration or publication has occurred. Reinstall explicitly only
when the compatible engine/catalog pair is available. No automatic migration
or old-root cleanup is introduced.

## Initialize and activate explicitly

After the compatible engine/catalog pair is available and any previous
installation has completed the clean handoff, initialize the catalog and
preview explicit activation:

```sh
packy init
packy list
packy activate engram --surface codex --dry-run
packy activate engram --surface codex
```

Claude Code **2.1.203 or newer** remains the engine's supported floor. The
separate discovery evidence records the exact newer binaries exercised; it
does not turn that floor into a claim of verified workflows on every version.
