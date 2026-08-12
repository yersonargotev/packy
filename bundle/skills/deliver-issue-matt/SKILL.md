---
name: deliver-issue-matt
description: Deliver one ready issue through Matt Pocock's implementation workflow, manual verification, CI, protected integration, and cleanup.
disable-model-invocation: true
---

# Deliver Issue with Matt's Workflow

Deliver exactly one ready issue identified by the user. Require its number or
the tracker's unambiguous equivalent and explicit delivery intent.

## Bootstrap

1. Resolve the repository root and read every applicable repository
   instruction. Read `docs/agents/issue-tracker.md` and
   `docs/agents/triage-labels.md` from that root. These two consumer-owned files
   are the complete setup contract.

   **Complete when:** both files and every applicable instruction are loaded,
   or an exception brief identifies the missing or contradictory material.
2. Verify that the model-invoked `/tdd` and `/code-review` skills can load.

   **Complete when:** both runtime dependencies are available, or an exception
   brief identifies the missing dependency.
3. Load [`references/invariants.md`](references/invariants.md) before observing
   or mutating delivery state.

   **Complete when:** the invariants govern the run.

## Gates

The active gate is the earliest incomplete gate. Load only its reference and
satisfy its completion criterion before loading the next. A candidate change
returns the delivery to the earliest gate whose proof it invalidates.

1. [`references/01-qualify-and-isolate.md`](references/01-qualify-and-isolate.md)
2. [`references/02-implement-and-verify.md`](references/02-implement-and-verify.md)
3. [`references/03-change-request-and-ci.md`](references/03-change-request-and-ci.md)
4. [`references/04-integrate-and-clean-up.md`](references/04-integrate-and-clean-up.md)
