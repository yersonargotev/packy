---
status: accepted
---

# Make Packy's root namespace Pack-oriented

## Context

Packy's original command tree placed the Pack lifecycle below a non-runnable
`pack` grouping command. Users invoked `packy pack <verb>` even though Pack is
the product's central domain object and the intermediate command owned no
routing policy, shared flags, safety boundary, or behavior.

Research against Packy's Cobra v1.9.1 dependency established that promoting
the existing lifecycle command objects changes their accepted path and command
path without changing their handlers, flags, argument validation, or error
policy. Cobra derives help and shell completion from that same command tree.
The current root has no competing application command names for the lifecycle
verbs.

## Decision

Packy's root namespace is Pack-oriented. `list`, `show`, `status`, `install`,
`uninstall`, `activate`, `update`, and `deactivate` are direct root commands.
The obsolete `pack` grouping command is absent and is not retained through an
alias, forwarding command, fallback, or deprecation path.

This is a routing decision at the `internal/cli` adapter boundary.
Pack lifecycle behavior remains in `internal/capabilitypack`,
and the direct verbs retain their flags, positional arguments, previews,
consent gates, receipts,
ownership rules, failure behavior, and structured-output report identifiers.

Root help owns shared lifecycle guidance and identifies the namespace as Pack
operations. Verb help owns only verb-specific behavior. Generated completion
and current operator documentation follow the root command tree; agent
instructions do not duplicate a command inventory.

## Consequences

Generic names such as `install`, `status`, and `update` are reserved for Pack
lifecycle behavior. A future unrelated domain that wants one of those names
requires a new namespace decision.

The former `packy pack <verb>` spelling fails through
Cobra's ordinary unknown command behavior. Historical material may retain that
spelling as evidence, but current guidance teaches only the direct commands.

Command-tree, help, completion, generated-documentation, and documentation-link
tests protect this decision. The implementation and its complete behavioral
acceptance record remain available in [issue 589](https://github.com/yersonargotev/packy/issues/589).
