# ADR 0021: Extract issue delivery from Packy

## Status

Accepted.

## Context

ADR 0001 defines Packy as an installer and configurator rather than a runtime
orchestrator. ADR 0020 nevertheless placed a second product in this repository:
a resumable issue-delivery engine with its own evidence schemas, lifecycle,
Git and GitHub effects, review policy, persistence, recovery, and maintenance
skill.

The public Packy binary does not import or expose that engine. Maintainers invoke
it through a private command and skill. Keeping it in this repository therefore
couples two products with different users, release cadence, domain vocabulary,
and reasons to change without providing a runtime integration benefit.

## Decision

Move issue-delivery orchestration to the independent
[`yersonargotev/packy-delivery`](https://github.com/yersonargotev/packy-delivery)
repository.

The extraction moves the evidence model, resumable `Advance` engine, executable
adapters, workflow contract, and `deliver-packy-issue` skill together. The new
project remains intentionally specific to Packy until another proven consumer
requires generalization.

Packy retains only the contracts the delivery tool observes:

- the repository validation authority;
- GitHub workflows and required checks;
- issue and governance conventions;
- product-owned architecture and domain documentation.

Packy does not import `packy-delivery` as a Go dependency and does not expose a
public delivery command. Maintainers install or invoke the independently
versioned `packy-deliver` executable through its maintenance skill.

## Compatibility

The extracted tool preserves the existing command semantics, JSON contracts,
evidence and run schemas, and Git-common-directory storage paths. Existing v2
runs can resume against the same Packy checkout. Schema v1 remains available
only through its explicit legacy workflow.

The extraction does not broaden delivery authority. Release publication, Pack
Source publication, package-manager effects, and real-user configuration remain
outside issue-delivery authority.

## Consequences

- Packy's repository, validation allowlist, domain vocabulary, and CI no longer
  own or test the delivery engine.
- Delivery-engine changes and releases no longer appear as Packy product
  changes.
- The maintenance skill and workflow are versioned with their executable.
- Compatibility between the two repositories is enforced at their observable
  command, schema, filesystem, validation, and GitHub boundaries rather than by
  shared internal packages.
- ADR 0020 remains as historical rationale for the extracted engine and is
  superseded for repository ownership.
