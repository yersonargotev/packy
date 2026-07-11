Type: prototype
Status: resolved
Blocked by: 05

## Question

What CLI interaction makes pack discovery, per-surface activation, preview, conflict/dependency errors, status, disable, and recovery understandable while preserving Matty's existing dry-run and safety conventions?

## Answer

Use the confirmed [pack lifecycle UX prototype](../lifecycle-ux-prototype/README.md). Group discovery and lifecycle under `matty pack`; compose the deep facade's distinct Preview and Apply operations into one interactive command, while `--dry-run` performs Preview only and Apply always requires a TTY. Render exact immutable plans in typed reversible-local, executable/external, and destructive-cleanup phases with separate approvals.

Provide overview-first and targeted status, consolidated planning blockers, explicit combined dependency activation, catalog-current update, targeted and bulk reconcile, and successful no-ops. Stale plans execute zero actions and stop; partial failures report truthful outcomes, and recovery repeats the originating lifecycle verb to build a freshly inspected plan with new approvals. A verified Apply exits successfully even when host-owned trust, authentication, or reload remains pending; `status --require usable` is the separate readiness gate.
