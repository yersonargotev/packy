---
name: deliver-packy-issue
description: Deliver a named Packy GitHub issue end to end through a local implementation-review loop, pull request, green CI, merge, and cleanup. Use when the user explicitly asks for complete issue delivery.
---

# Deliver Packy Issue

Read the complete [workflow contract](../../../workflows/packy-issue-delivery.md)
and [repository instructions](../../../AGENTS.md) before mutating project or
tracker state. The contract owns delivery behavior; keep this skill as its thin
orchestrator.

Create or resume the issue's `Delivery Run`, then invoke the contract's
private `deliveryevidence advance` adapter repeatedly. Supply genuine decisions,
review results, and adjudications only when the returned state requires them.
Otherwise let `Advance` reacquire facts, perform deterministic work, persist
evidence, and recover idempotently.

Stop only when the run is completed, blocked, waiting for an external result,
needs review, or needs one decision. For a schema v1 run, follow only the
contract's explicit legacy-v1 behavior.
