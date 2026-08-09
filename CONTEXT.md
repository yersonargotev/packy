# Context

This glossary is the current domain language for Packy `v0.2.0`. The accepted
architecture is recorded in [ADR 0031](docs/adr/0031-simplify-packy-around-reviewed-packs.md)
and [ADR 0033](docs/adr/0033-make-the-tui-the-primary-interactive-interface.md).

## Glossary

### Packy

A terminal installer and configurator for reviewed capability Packs on Codex,
OpenCode, and Claude Code. Its interactive and command-line interfaces expose
the same Pack lifecycle.

### Pack

A Git-reviewed capability bundle with one manifest, reviewed resources,
declared supported surfaces, and a maintainer-selected SemVer.

### Reviewed Pack catalog

The current set of selectable bundled Pack manifests. Each manifest owns its
Pack version and supported surfaces, and each Pack is independently selectable.

### Orchestrate Pack

The accepted Codex-only Pack that contributes the exact upstream
`$orchestrate` coordination skill and its MIT notice. Its canonical source is a
stable release of `yersonargotev/orchestrate-skill`; Packy preserves Eric
Provencher's attribution and treats configured projection separately from
runtime usability.

### Pack manifest

The single `pack.json` contract for a Pack. It declares identity, version,
description, selectability, supported surfaces, resources, bindings,
intra-Pack dependencies, external requirements, conflicts, and exclusions.

### Pack Source

One upstream provenance declaration for selected Pack resources and legal
notices. Its configuration identifies a stable release, while its lock records
the exact selected content used to reproduce and review a bundle generation.

### Pack resource

One host-independent capability contributed by a Pack. A surface adapter turns
it into one or more host-native projections.

### CLI surface

A supported host Packy can configure. The supported surfaces are Codex,
OpenCode, and Claude Code. Antigravity and GitHub Copilot CLI remain outside
the current product.

### Pack activation

The user's explicit consent to a previewed global Pack operation on one CLI
surface. Project installation and personal project activation are separate.

### Pack lifecycle

The previewed state transitions for one Pack, CLI surface, and global or
project scope. It includes inspection, consent, application, and verification.

### Project installation

The reviewed project intent and materialized projections for one Pack and
surface in a Git worktree.

### Project Pack manifest

The human-authored `packy.json` at a project root. It records direct Pack,
surface, and resource intent.

### Installed Pack receipt

The minimal current-state record for one Pack and surface: Pack identity and
version, selected resource closure, projected paths, and content digests. It is
the authority for safe update or removal of unchanged Pack-owned content.

### Project Pack lock

The generated `packy.lock.json` containing one installed Pack receipt per Pack
and surface. It is committed with the project Pack manifest.

### Pack ownership

Packy's authority over an exact projected path established by an installed
Pack receipt.

### Owned projection drift

A receipt-owned path whose current content differs from its recorded digest.
Ordinary mutation stops before writing; force remains limited to paths in the
targeted receipt.

### Pack projection conflict

An attempted operation in which distinct Pack resources target the same path.
It fails before mutation, even when the proposed bytes match.

### Pack authoring workflow

Copy the standard Pack template when needed, add or edit reviewed content,
update the one manifest and maintainer-selected SemVer, then run the focused
validator for that Pack.

### Single-source Pack admission

The atomic initial transition that creates one previously absent Pack and its
one Pack Source as a complete bundle generation. It admits the source and
selected content, legal notice, Pack manifest, catalog entry, and initial
history together or publishes none of them.

### Composite Pack Source Bundle

The two-or-more-source unit used to admit a previously absent Pack whose
initial provenance spans multiple Pack Sources. It is distinct from
single-source Pack admission.

### External requirement

A host tool a Pack needs, such as Engram. Packy reports readiness without
turning external tools into Pack relationships.

### Issue delivery

The ordinary pull-request path protected by GitHub branch protection, required
CI, CodeQL, review, and human merge.

### Packy release

An immutable Packy distribution published from one version tag on `main`. A
faulty release is corrected by a newer version.

### Packy Home

The `~/.packy` root containing Packy's workstation receipts and state.
