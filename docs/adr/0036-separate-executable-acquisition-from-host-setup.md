---
status: accepted
---

# Separate executable acquisition from host setup

Packy models reviewed acquisition of a declared external requirement with the
`external-executable-acquisition` surface capability. The capability may offer
an exact shared-executable installation, but it never authorizes the external
tool to configure a CLI surface.

This replaces `external-host-setup`. Packy previously coupled Engram's reviewed
Homebrew acquisition to `engram setup`, which also installed global
instructions, prompts, plugins, hooks, and MCP configuration outside Packy's
reviewed projections. Keeping acquisition while removing setup preserves the
explicit executable requirement and gives the capability-pack domain one small
interface for the only external effect Packy still owns.

Engram's current selective-memory mode is a reviewed Codex skill. It does not
promise MCP retrieval, session lifecycle, automatic compaction recovery, or
setup-equivalent behavior; a future full-memory mode requires a separate
decision.
