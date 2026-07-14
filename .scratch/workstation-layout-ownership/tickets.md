# Tickets: Workstation layout ownership

A five-slice expand–migrate–contract sequence that hides derived workstation
layout behind owning modules without changing observable Matty behavior.
Source: [specification](spec.md).

Work the **frontier**: any ticket whose blockers are all done. Clear context
between tickets and use `/implement` for one frontier ticket at a time.

| # | Ticket | Blocked by | Status |
| --- | --- | --- | --- |
| 01 | [Contract the Workstation snapshot through initialization](tickets/01-contract-workstation-snapshot-through-initialization.md) | None | resolved |
| 02 | [Route Matty core lifecycle through owning layouts](tickets/02-route-core-lifecycle-through-owning-layouts.md) | 01 | resolved |
| 03 | [Route capability packs through owning layouts](tickets/03-route-capability-packs-through-owning-layouts.md) | 02 | resolved |
| 04 | [Route setup health through owner observations](tickets/04-route-setup-health-through-owner-observations.md) | 02 | resolved |
| 05 | [Contract the CLI layout surface and enforce ownership](tickets/05-contract-cli-layout-surface-and-enforce-ownership.md) | 03, 04 | resolved |
