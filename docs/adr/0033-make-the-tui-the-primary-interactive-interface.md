---
status: accepted
---

# Make the TUI the primary interactive interface

Packy uses a Bubble Tea v2 TUI as its primary experience when `packy` runs
without arguments in an interactive terminal. Explicit commands remain the
stable non-interactive and accessible interface for scripts, structured output,
and terminals that cannot run the TUI; adopting Bubble Tea v2 raises Packy's
minimum Go version to 1.25.

The TUI is an adapter over the same `internal/capabilitypack` behavior as the
command interface. It discovers selectable Packs and supported CLI surfaces
from the reviewed catalog, operates on one Pack, surface, and global or project
scope at a time, and preserves the existing preview, phase consent, apply, and
verification boundary. It does not invoke Packy as a subprocess or own domain
behavior.

This decision replaces ADR 0031 only where that ADR characterizes Packy as
exclusively command-line and fixes the Reviewed Pack catalog to four named
Packs. The catalog is data-driven and may contain any reviewed, selectable Pack;
the remaining lifecycle, receipt, safety, project, and release decisions in ADR
0031 remain accepted.

## Consequences

Users may cancel navigation, selection, preview, and consent without effects.
After application begins, the TUI defers exit until the operation returns because
global and external effects do not share a guaranteed rollback boundary. Every
result is followed by fresh inspection; Packy reports rollback only when the
domain has verified it.
