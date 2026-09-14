# Packy roadmap

Packy `v0.2.0` establishes the current product: reviewed Packs, installed Pack
receipts, Codex/OpenCode/Claude Code support, proportional validation, native
GitHub integration controls, and an immutable binary release. Use the
manifest-backed Pack catalog through `packy list` for current membership and
versions. Claude Code support requires stable version **2.1.203 or newer**.

## Current checkpoint

- Selectable Packs and their independent versions come from the selected
  Catalog Snapshot.
- Global and project lifecycles operate on independent installed Pack receipts.
- Catalog maintainers use the supported authoring commands and whole-catalog
  validator.
- General CI uses formatting, vet, Packy-owned tests, and focused race coverage.
- Version tags publish four platform archives, `SHA256SUMS`, one GitHub
  Release, and a matching Homebrew formula.
- The sole current user adopts the generation through the
  [manual v0.2 reset](reset-v0.2.md).

## Future decisions

The following are outside the current product until a separate decision is
accepted:

- another CLI surface beyond Codex, OpenCode, and Claude Code;
- another reviewed Pack beyond the current manifest-backed catalog;
- a different binary distribution channel; or
- broader platform support beyond the published Darwin and Linux artifacts.

Current behavior belongs in the README and product guides. Current architecture
belongs in [ADR 0031](adr/0031-simplify-packy-around-reviewed-packs.md).
