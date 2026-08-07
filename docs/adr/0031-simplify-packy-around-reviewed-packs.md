---
status: accepted
---

# Simplify Packy around reviewed Packs and installed receipts

Packy `v0.2.0` is a clean architectural generation centered on installing and configuring four Git-reviewed Packs whose manifests begin at Pack version `1.0.0`, as accepted in [specification #513](https://github.com/yersonargotev/packy/issues/513).

Packy retains its CLI lifecycle, minimal installed Pack receipts, native GitHub CI and CodeQL controls, and a conventional immutable binary release. It removes upstream synchronization and classification, source provenance and Pack history, cross-Pack resolution, custom governance authorization and drift detection, the Vercel Pack, legacy manifest readers, and the custom release candidate and recovery framework.

This deliberately breaks compatibility with `v0.1.x` state because this Mac is the only installation and the prior machinery imposed disproportionate development and validation cost. Existing tags and published releases remain immutable history while obsolete architecture is removed from the current documentation set.
