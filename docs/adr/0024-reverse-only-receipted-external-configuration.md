# ADR 0024: Reverse only receipted external configuration

## Status

Accepted.

## Context

Explicit pack activation may separately authorize a tool-owned host setup command such as `engram setup codex` or `engram setup opencode`. The existing durable command fingerprint prevents accidental re-execution but records neither the exact configuration contribution observed after setup nor authority to reverse it. Treating that fingerprint, a familiar path, or recognizable content as ownership would let deactivation delete foreign configuration.

## Decision

`internal/capabilitypack` owns versioned external-effect receipts, last-contributor policy, typed destructive consent, freshness rejection, recovery evidence, and receipt retirement. A receipt seals the initiating effect identity, target CLI surface, exact freshly verified contribution observations, and the adapter provenance that defines their reversal contract. A fingerprint without that complete receipt grants no reversal authority.

The owning Codex or OpenCode surface adapter freshly observes each receipted contribution and constructs only its exact host-native reversal. Deactivation may schedule that reversal under `destructive-cleanup` only when the final activation contributor is removed and every sealed observation still matches. Missing, modified, ambiguous, shared, foreign, or receipt-less setup is preserved without inferred ownership. Apply observes again before mutation, verifies the exact contribution absent afterward, retains the receipt and journal after partial failure, and retires the active receipt only after verified success without deleting attempt history.

Receipts authorize configuration reversal only. They never authorize removal or modification of the Engram executable, service, memory, data, sessions, credentials, or unrelated host configuration. Human and structured lifecycle reports disclose these limits while redacting sealed values and environment arguments.

This decision refines the separately authorized tool-owned host setup in [ADR 0022](0022-require-explicit-surface-activation.md) and the lifecycle/adapter ownership boundary in [ADR 0005](0005-capability-pack-surface-adapter.md).
