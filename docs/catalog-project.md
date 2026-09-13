# Catalog Project

[`yersonargotev/packy-catalog`](https://github.com/yersonargotev/packy-catalog)
is the canonical authoring repository for Addy, Argote, Engram, Issue Delivery,
Matty, Orchestrate, and pstack. Independent upstream products retain their own
repositories and release lifecycles.

The project preserves Packy's bundle-relative layout:

```text
bundle/
  packs/<pack-id>/pack.json
  agents/
  assets/
  commands/
  instructions/
  notices/
  skills/
```

The validator discovers manifests only at `bundle/packs/*/pack.json`; there is
no handwritten registry. Each manifest and the deterministic union of its
resource and typed capability source roots form its Declared Pack Closure.
Distinct Packs cannot own the same closure path.

## Validation

From a Packy checkout, validate a candidate Catalog Project with:

```sh
go run ./internal/tools/catalogvalidate --project /path/to/packy-catalog
```

To validate a content change and its independent Pack versions, supply the
target-branch checkout as a baseline:

```sh
go run ./internal/tools/catalogvalidate \
  --project /path/to/candidate \
  --baseline /path/to/baseline
```

The validator strictly checks every Pack's typed manifest vocabulary,
dependencies, conflicts, origins, exact-copy or adapted relationships,
required notices, safe resource paths, deterministic closure, and complete
runtime-fitness matrix. Exact copies are compared with their pinned public
upstream commits. Catalog files are read as inert data and never executed.

For an existing Pack, any manifest-contract or referenced-byte change requires
a strictly greater SemVer. Content with the same identity must retain its
version. A new Pack may introduce any valid SemVer without changing unrelated
Pack versions.

Pull-request CI in the Catalog Project runs this same validator with read-only
permissions. Automatic Catalog Snapshot publication is owned by separate
delivery work and is not part of authoring validation.
