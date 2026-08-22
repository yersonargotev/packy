---
status: accepted
---

# Promote releases from Managed Pack Projects

## Context

Packy's Pack Source machinery reconstructs Pack intent from arbitrary upstream
repositories. That spreads one authoring decision across inventory,
classification, exclusions, source locks, legal evidence, historical copies,
and Pack manifests. A separate Packmaint product would preserve those shallow
interfaces while adding another repository and release lifecycle.

## Decision

Every Pack is authored by exactly one public, maintainer-controlled Managed
Pack Project. The project places schema v1 `pack.json` at its root, uses final
bundle-relative resource paths, declares immutable public origins and
whole-resource `exact-copy` or `adapted` relationships, and publishes complete
stable immutable releases tagged `pack-v<pack-version>`. Positive resource
roots define membership; `exclusions` and `source_reference` do not exist in
the Managed Pack contract.

Packy owns one private Managed Pack Promotion module. Its later promotion
interface accepts one registered `<pack-id>@<version>` and returns no change,
a typed rejection, or one protected pull-request proposal. Acquisition,
offline validation, and GitHub mutation remain separate authority phases, and
project content is never executed.

Packy owns the reviewed one-to-one Managed Pack Registry and append-only Pack
Admission Records outside the end-user bundle. The initial registry is:

| Pack | Managed Pack Project |
| --- | --- |
| Addy | `yersonargotev/skills-addy` |
| Argote | `yersonargotev/argote` |
| Engram | `yersonargotev/engram` |
| Issue Delivery | `yersonargotev/issue-deliver-pack` |
| Matty | `yersonargotev/skills-mattpocock` |
| Orchestrate | `yersonargotev/orchestrate-skill` |
| pstack | `yersonargotev/pstack` |

The Declared Pack Closure is the exact root manifest plus the deterministic
union of referenced resource roots. It rejects traversal, absolute paths,
missing paths, overlapping roots, symlinks, submodules, and non-regular files.
Exact copies are compared byte for byte, adapted resources carry notices, and
every result seals the manifest digest, closure digest, and sorted
path/mode/SHA-256 index. A reusable Packy workflow gives projects the same
preventive validation contract before immutable release publication.

## Consequences

The existing catalog stays operational while all seven Packs migrate through
higher immutable versions. After that migration, Packy removes Pack Sources,
locks, exclusions, whole-repository classification, separate legal and
compatibility evidence, historical bundle copies, and obsolete workflows; it
does not retain a compatibility loader.

This decision supersedes ADR 0032's single-source admission architecture and
ADR 0037's independent-source delivery architecture. It narrows ADR 0031's
Pack authoring model while preserving its offline bundle, installed receipts,
ordinary CI, CodeQL, review, branch protection, and human merge. ADR 0035's
capability-driven readiness remains unchanged: exact projection does not prove
runtime usability.
