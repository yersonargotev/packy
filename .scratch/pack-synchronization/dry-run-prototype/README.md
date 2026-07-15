# PROTOTYPE: Matty pack synchronization dry run

This is a disposable, read-only logic prototype for validating the accepted
synchronization contracts. It is not connected to Matty's CLI, a GitHub
workflow, or any production path. It never writes under `bundle/` and never
executes upstream content.

Run the real main-path inspection from the repository root:

```sh
.scratch/pack-synchronization/dry-run-prototype/run.sh
```

The command uses a clean temporary directory and sandboxed `HOME` and
`XDG_CONFIG_HOME`, queries public GitHub endpoints without credentials, safely
inspects the exact release tarball as inert data, compares it with the real
`matty` pack, and writes review-only JSON/Markdown under `artifacts/`.

The acceptance matrix is deterministic rather than a high-fidelity GitHub
simulation. Its purpose is to expose which layer owns each decision and which
contractual terminal state should result.

**Question answered:** does the accepted contract produce a safe,
understandable plan for the current real `matty` pack and the latest stable
`mattpocock/skills` release?

**Verdict:** accepted through HITL review on 2026-07-14. The real-data path is
correctly blocked by five intentional local adaptations; preserve them through
a future Matty-owned seam, classify the hypothetical replacement as major, and
keep the current allowlist unchanged. Delete this directory after its answer is
captured durably in the wayfinder ticket.
