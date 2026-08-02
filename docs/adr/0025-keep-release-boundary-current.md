# ADR 0025: Keep the release boundary current

## Status

Accepted.

## Context

Packy's release inputs should describe the product and capability packs that
can be selected today. Transitional migration, equivalence, and cleanup
contracts are not current product behavior and make every future release carry
responsibility for artifacts outside the current ownership model.

## Decision

The repository ships current Packy contracts and the history required by
selectable capability packs. It does not ship product-identity transition
fixtures, frozen transition evidence, retired command contracts, or release
steps that inspect and delete unrelated distribution paths.

The Homebrew publication boundary updates only `Formula/packy.rb`. Current
validation proves initialization, explicit capability-pack activation,
operator-file preservation, readiness, and release publication without replaying
retired product transitions.

## Consequences

- Release plans and evidence contain only current operations and assertions.
- Product transition records remain recoverable from Git history rather than
  being included in release source and runtime contracts.
- The selectable `matty` capability pack and its version history remain part of
  the catalog; they are capability-pack data, not product-identity fallback.
