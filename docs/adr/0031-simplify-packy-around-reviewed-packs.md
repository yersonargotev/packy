---
status: accepted
---

# Simplify Packy around reviewed Packs and installed receipts

## Context

Packy `v0.2.0` is a clean architectural generation for its sole current user.
The earlier generation made routine Pack work depend on systems that were much
larger than the installer/configurator product.

## Decision

Packy installs and configures exactly four Git-reviewed Packs: `addy`, `argote`,
`engram`, and `matty`. Each starts this generation at Pack version `1.0.0`,
owns one current manifest, and may be selected independently for Codex,
OpenCode, or Claude Code.

Reviewed repository content is authoritative. Maintainers copy the standard
template when creating a Pack, edit reviewed content and one manifest, select
the Pack SemVer, and run the focused Pack validator.

A successful global activation or project installation writes an installed Pack receipt.
The receipt records only the Pack identity and version, target
surface and scope, selected resource closure, projected paths, and content
digests needed to check ownership, drift, and collisions. A project receipt
also seals the filesystem mode of every committed projection so portable
verification can detect permission drift without consulting the catalog or
personal state. Global and project state share the receipt model while keeping
project-only filesystem evidence out of global state, and each lifecycle
command operates on one Pack and receipt.

Repository integration uses native GitHub branch protection, ordinary CI,
CodeQL, review, and human merge. Issues organize work but grant no merge
authority.

The release boundary is conventional and immutable. One version tag on `main`
publishes Darwin and Linux binaries for amd64 and arm64, `SHA256SUMS`, one
GitHub Release, a matching Homebrew formula, and a package-install smoke. A
fault is corrected on `main` and published as a newer version.

## Consequences

`v0.2.0` does not read or migrate `v0.1.x` state. Adoption uses the documented
manual one-time reset, with warnings before the user removes prior workstation
state or project declarations. Packy ships no reset command or automatic
cleanup.

Historical Git remains available. Checked-out documentation dedicated
exclusively to retired systems remains excluded, preserving the cleanup
established by [issue 521](https://github.com/yersonargotev/packy/issues/521).

Durable, dated, non-normative evidence governed by the current
[research evidence contract](../research/README.md) may remain checked out.
Such evidence may support decisions but does not become current authority
merely by remaining in the repository: accepted ADRs define current
architecture, and [CONTEXT.md](../../CONTEXT.md) defines current domain
language.
