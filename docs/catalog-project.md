# Catalog Project

[`yersonargotev/packy-catalog`](https://github.com/yersonargotev/packy-catalog)
is the canonical authoring repository for Addy, Argote, Engram, Issue Delivery,
Matty, Orchestrate, and pstack. Independent upstream products retain their own
repositories and release lifecycles.

An initial Packy engine release must provide Catalog Snapshot support before
users can consume this independent publication flow. After that prerequisite
is installed, the same installed Packy executable can acquire compatible
content-only publications with `packy catalog refresh`; no Packy release or
binary upgrade is required. Refresh changes only the selected catalog
availability. Existing activations keep their retained snapshot until the user
explicitly runs `packy update <pack> --surface <surface>`, and newly published
Packs remain inactive until explicitly activated.

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

## CLI authoring

Create a Pack from the supported empty template by naming its initial contract
explicitly:

```sh
packy catalog create example-pack \
  --project /path/to/packy-catalog \
  --template empty \
  --version 0.1.0 \
  --description "Example reviewed workflows" \
  --surface codex
```

Import one selected resource from an exact public upstream commit. Destinations,
relationships, and every Pack host are explicit; Packy does not classify or
convert the upstream repository:

```sh
packy catalog import example-pack \
  --project /path/to/packy-catalog \
  --version 0.1.1 \
  --repository example/upstream \
  --commit 0123456789abcdef0123456789abcdef01234567 \
  --origin-id upstream \
  --origin-path skills/example \
  --destination skills/example-pack/example \
  --relationship exact-copy \
  --kind skill \
  --resource-id example \
  --description "Runs the reviewed example workflow" \
  --host codex \
  --notice notice:upstream-mit
```

The supported imported kinds are `instruction`, `notice`, and `skill`. Import
a notice with explicit `--license` and `--attribution`; a notice records its
own attribution, while every other imported resource must name at least one
existing notice with `--notice`. Standard upstream notice filenames are
detected only to improve missing-information diagnostics. They are never used
to infer licensing, resource kinds, destinations, hosts, or relationships.

Each operation stages the complete `bundle/`, validates the resulting Catalog
Project, and only then atomically exchanges the complete prepared bundle into
the Git worktree. A rejected or interrupted operation never exposes a partial
Catalog Project. These commands never publish a Catalog Snapshot.

Refresh every exact-copy resource for one Pack origin by selecting the new
upstream commit and the Pack's new version:

```sh
packy catalog upstream-refresh example-pack \
  --project /path/to/packy-catalog \
  --origin-id upstream \
  --commit fedcba9876543210fedcba9876543210fedcba98 \
  --version 0.2.0
```

Before preparing the update, Packy verifies the current exact copies against
the previously pinned commit. Unexpected local changes stop the operation.
Packy then acquires the selected commit and presents the old-to-new upstream
differences for every adapted resource. In an interactive terminal, the
maintainer must explicitly confirm that each maintained adaptation reconciles
those changes; a declined or non-interactive request remains unapplied.

After reconciliation, Packy replaces all exact copies in the staged bundle
while preserving every maintained adaptation, its notices, and its provenance.
It updates the origin commit and Pack version, then validates the complete
Catalog Project before applying it atomically. Any unresolved adaptation,
acquisition, validation, or concurrent-write failure leaves the whole request
unapplied. Upstream Refresh leaves a reviewable Git diff and never publishes
it.

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
permissions. It cannot publish and receives no publication credentials.

## Catalog Snapshots

A successful validation of a reviewed merge on the Catalog Project's protected
`main` branch starts publication automatically. The read-only preparation job
uses Packy's pinned `catalogsnapshot` tool to build exactly two files:

- `catalog-snapshot.tar.gz`, containing `catalog-index.json` and the complete
  deterministic union of every declared Pack closure under `bundle/`; and
- `SHA256SUMS`, containing the archive's SHA-256 digest.

The index records the full Catalog Project commit, exact Packy builder commit,
catalog digest, and every included Pack's ID, version, manifest digest, closure
digest, and ordered path/mode/content-digest index. The immutable GitHub Release
tag is `catalog-<full-catalog-commit>`.

Only the final publication job receives `contents: write`, `id-token: write`,
and `attestations: write`. It receives the already validated artifact rather
than executing Catalog Project content, publishes a GitHub artifact attestation,
and creates a draft release before uploading and verifying its complete asset
set. Publishing the draft activates the Catalog Project repository's native
release-immutability control; the publisher requires GitHub to report the
result as immutable. A retry accepts an existing immutable, byte-identical
release or completes a matching draft. A published mutable release, a different
target, an unexpected asset, or changed bytes are rejected.

Consumers verify the archive checksum and the GitHub artifact attestation for
`yersonargotev/packy-catalog`. Together with the embedded source and builder
commits, this proves the official publisher, artifact integrity, and exact Pack
bytes. Catalog acquisition and local snapshot selection are implemented by the
separate consumer lifecycle work.
