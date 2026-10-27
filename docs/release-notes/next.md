# {{TAG}} — Agents and Claude conventions and plugin export

## Changes since the previous release

The previous stable release is `v0.2.31`.

- Replace the `codex` surface with the shared `agents` convention and retire
  OpenCode. Agents supports global and project skills and project instructions;
  Claude supports its native instructions, skills, and reviewed agent documents.
  Retired surface values and direct MCP, hook, and command projections are
  rejected rather than translated. There are no aliases or automatic migrations.
- Preserve complete skill trees, including optional `agents/openai.yaml` metadata.
  Required custom agents have explicitly reviewed client and scope projections:
  Codex roles require `--client codex` and supported global scope; Claude roles
  use reviewed native documents. Unsupported required roles fail before mutation.
  Ordinary Agents skills do not configure Codex. Projected role files do not
  prove that a client loaded or executed them.
- Export one Pack as an Agent Plugins package (`--surface agents`) or a Claude
  plugin (`--surface claude`) with `packy export`. Reviewed format compatibility
  controls selection; explicit subsets include required same-Pack dependencies,
  supporting resources, attribution and notices. Incompatible selections fail
  as a whole. Claude invocation references are explicitly reviewed; Agent Plugins
  does not imply a universal invocation syntax. Packages contain skills, not
  native role registration or instructions, and report external role
  prerequisites without asserting they are satisfied. Export is offline, does not
  activate a Pack or change client settings, and produces deterministic content.
  See the [plugin export guide](https://github.com/yersonargotev/packy/blob/main/docs/plugin-export.md).
- The Packy repository's own project configuration now selects the reviewed
  `packy-cli` discovery skill instead of the full Matty installation.

## Adoption and catalog rollout

Before replacing the previous binary, use it to inventory, preview and remove
all affected Packy-owned global and project installations. Keep referenced
sources and snapshots and preserve unrelated files, credentials, and host
configuration. Follow the [clean adoption procedure](https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md).
Do not perform the handoff until both a compatible released engine and complete
compatible catalog are available.

Release the engine first, then validate the entire compatible catalog against
that immutable engine before catalog publication. Do not publish a mixed-generation
snapshot. Catalog Refresh remains explicit and does not update existing
installations; retained receipts continue to reference their snapshots. The
engine release does not publish the catalog or modify its sources.

After the compatible catalog is available and the handoff is complete, initialize
and explicitly preview activation:

```sh
packy init
packy activate engram --surface agents --dry-run
packy activate engram --surface agents
```

Claude Code **2.1.203 or newer** remains the supported floor. Plugin generation
proves package content, not client installation, authorization, or runtime execution.

## Release verification

The immutable release workflow validates and tests the same staged archives on
Darwin and Linux for amd64 and arm64 before publication. The release includes
four archives, `SHA256SUMS`, source identity via `packy version --json`, and a
matching Homebrew formula. See the [release guidance](https://github.com/yersonargotev/packy/blob/main/docs/release.md).
