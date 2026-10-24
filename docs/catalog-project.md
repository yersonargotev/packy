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

Each Pack owns one self-contained directory:

```text
packs/
  <pack-id>/
    pack.json
    agents/
    assets/
    commands/
    instructions/
    notices/
    skills/
```

The validator discovers manifests only at `packs/*/pack.json`; there is
no handwritten registry. Each manifest and the deterministic union of its
common resource, variant, and typed capability source roots form its
Declared Pack Closure.
Every source is relative to its Pack root, so distinct Packs may use the same
relative path without sharing physical files. Undeclared files are rejected.

## Reviewed surface variants

Reviewed standalone skills can also declare the typed
[on-demand compatibility contract](on-demand-skills.md#reviewed-catalog-contract).
This declaration is independent of native installation support.

The current contract is [Pack manifest v3](../schemas/pack/v3/pack.schema.json)
and Catalog Snapshot index v3. Older generations are rejected as a complete
snapshot; refresh failures preserve the selected snapshot. Adopt this clean cut
engine-first, then publish the complete catalog with the immutable engine revision.

A resource can declare a `variants` array sorted uniquely by surface. Each
entry supplies a `surface` and applicable typed fields, for example:

```json
"variants": [
  {
    "surface": "codex",
    "source": "skills/guide/codex",
    "origin": {"id": "upstream", "path": "skills/guide", "relationship": "adapted"}
  }
]
```

The selected surface automatically chooses its reviewed effective resource;
logical kind and ID, bindings, and surface exclusions remain unchanged. Omitted
fields inherit the common definition. Present arrays and objects replace whole
values, empty arrays clear allowed fields, and null is invalid. Invalid declared
variants never fall back to common content. Source-only assets and notices
resolve under their consumer's surface without acquiring a projection binding.

Retain all original and adapted trees in the declared closure. Changing an
imported source requires explicit variant provenance and notice coverage.
Validation checks every body and every surface's effective dependency graph,
even when a particular operation does not select that body. Skill projections
and receipts cover the entire selected tree, including references, scripts, and
metadata. Distinct variants still obey physical ownership and host discovery
constraints; variants alone do not promise simultaneous host coexistence.

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

The supported new-resource import kinds are `instruction`, `notice`, and `skill`. Import
a notice with explicit `--license` and `--attribution`; a notice records its
own attribution, while every other imported resource must name at least one
existing notice with `--notice`. Standard upstream notice filenames are
detected only to improve missing-information diagnostics. They are never used
to infer licensing, resource kinds, destinations, hosts, or relationships.

To import a surface body for an existing file-backed logical resource, use
`--variant-surface codex` instead of `--host`. Keep its existing `--kind` and
`--resource-id`, and choose a separate Pack-relative `--destination`. This adds
one reviewed variant without changing its bindings or the original tree; an
existing variant is never overwritten. The same operation accepts existing
agents, commands, and assets as well as skills, instructions, and notices.
MCP and lifecycle resources have no file body to import. Review and adapt the
imported bytes in the Catalog Project; Packy does not execute adaptation code.

Variant imports explicitly record provenance. Omitted dependency/conflict
options inherit the common fields; provided arrays replace them. Notice
variants must retain the complete original notice bytes, license, and original
attribution, and may append additional adaptation attribution and text.

Each operation stages only the affected Pack, composes it with the unchanged
worktree Packs for whole-catalog validation, and then atomically exchanges only
`packs/<pack-id>`. Packy rechecks the complete `packs/` identity before the
exchange, so a concurrent catalog change aborts without exposing a partial
result. These commands never publish a Catalog Snapshot.

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
differences for every adapted common or surface body, labeling surface variants
explicitly. Inherited source/provenance is refreshed once. Exact source roots
that overlap a distinct maintained tree are rejected; place original and
adapted bodies in separate roots. In an interactive terminal, the
maintainer must explicitly confirm that each maintained adaptation reconciles
those changes; a declined or non-interactive request remains unapplied.

After reconciliation, Packy replaces all exact copies in the staged Pack
while preserving every maintained adaptation, its notices, and its provenance.
It updates the origin commit and Pack version, then validates the complete
Catalog Project before applying it atomically. Any unresolved adaptation,
acquisition, validation, or concurrent-write failure leaves the whole request
unapplied. Upstream Refresh leaves a reviewable Git diff and never publishes
it.

## Validation

Using a Packy executable, validate a candidate Catalog Project with:

```sh
packy catalog validate --project /path/to/packy-catalog
```

To validate a content change and its independent Pack versions, supply the
target-branch checkout as a baseline:

```sh
packy catalog validate \
  --project /path/to/candidate \
  --baseline /path/to/baseline
```

Neither operation requires Go or a Packy source checkout at runtime. `--project`
defaults to the current directory. Validation never installs or activates Packs
and does not initialize or refresh the workstation catalog.

Add `--json` for the versioned `catalog-validation` report. Success returns
`result: "valid"`, all Packs in identity order with their manifest/closure
SHA-256 digests and file counts, and the runtime-fitness row count. Failure
returns `result: "invalid"` and an `error` with a stable `code` and actionable
`message`; no partial validated Pack list is reported. Exit status is zero on
success and nonzero on invalid input, origin acquisition failure, or output
failure. JSON goes to stdout; diagnostic errors go to stderr. CLI syntax errors
before `--json` is parsed and output failures may have only stderr diagnostics.
The offline schema is
[`catalog-validation.schema.json`](../schemas/cli/v1/catalog-validation.schema.json).

`packy version --json` reports `schema_version: 1`, `report: "packy-version"`,
`version`, and `source_commit`. Official release binaries embed the full source
commit; unstamped development builds report `dev` and `unknown`. Use this
identity to verify the executable independently of the current directory.

The Catalog Project remains pinned to its previous immutable Packy release
until the complete supporting release is adopted. Removing the source-only
validator from current Packy does not alter that old release. The supported CLI provides snapshot construction and publication;
the Catalog Project adopts them with the complete supporting immutable release.

The validator strictly checks every Pack's typed manifest vocabulary,
dependencies, conflicts, origins, exact-copy or adapted relationships,
required notices, safe resource paths, deterministic closure, and complete
runtime-fitness matrix. Exact copies are compared with their pinned public
upstream commits. Origin acquisition honors `HTTPS_PROXY` and an explicit
`SSL_CERT_FILE` PEM bundle on every supported host, adding that bundle to system
trust while retaining certificate and hostname verification. This permits
controlled origin transports without changing the declared upstream identity.
Catalog files are read as inert data and never executed.

For an existing Pack, any manifest-contract or referenced-byte change requires
a strictly greater SemVer. Content with the same identity must retain its
version. A new Pack may introduce any valid SemVer without changing unrelated
Pack versions.

Pull-request CI in the Catalog Project uses the same validator through its
pinned engine release, with read-only permissions. It cannot publish and receives no publication credentials.

## Catalog Snapshots

A successful validation of a reviewed merge on the Catalog Project's protected
`main` branch starts publication automatically. The read-only preparation job
uses `packy catalog build` from the pinned Packy release to build exactly two files:

- `catalog-snapshot.tar.gz`, containing `catalog-index.json` and the complete
  deterministic union of every declared Pack closure under `packs/<pack-id>/`; and
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
`yersonargotev/packy-catalog`. These checks authenticate the official publisher
and artifact integrity. The publication workflow must also bind the prepared
assets to the reviewed Catalog Project commit; source and builder labels alone
do not prove that published bytes match that commit.

### Build a local candidate

```sh
packy catalog build --project <catalog-project-root> \
  --source-repository yersonargotev/packy-catalog \
  --source-commit <full-catalog-commit> --out-dir <new-output-directory>
```

The command revalidates the complete candidate, including provenance, closures,
and runtime fitness, before construction. Add `--baseline <baseline-project>`
to enforce independent Pack version changes during the same build. Catalog
resources remain inert. The existing constructor seals validated inputs,
rejects input drift and unsafe paths, refuses an existing output directory,
and stages the complete asset set before making it available.

`--source-repository` and `--source-commit` are caller-supplied labels for local
candidate bytes. They do not inspect Git, require a clean checkout, or certify
that the input matches a reviewed commit. Official publication must establish
that correspondence separately through its reviewed preparation workflow and
publisher verification. A local build is not a Catalog Publication.

The index's `builder` is always `yersonargotev/packy@<embedded-source-commit>`
from the executing release binary. There is no `--builder` override. Unstamped
development binaries whose embedded commit is `unknown` cannot build snapshots;
use a release-stamped binary. `packy version --json` exposes the same commit.
Repeated builds with identical validated input and labels retain the existing
same-platform deterministic bytes; cross-platform byte identity is not promised.

Default output reports the source, builder, digests, and included Packs. With
`--json`, the version 1 `catalog-build` report returns `result: "built"`, the
complete `index`, `archive_path`, `checksum_path`, and `archive_sha256`. A failure
returns a nonzero exit status and `result: "failed"` with `error.code` and
`error.message`; codes are `invalid_arguments`, `origin_setup_failed`,
`validation_failed`, and `build_failed`. Invalid labels, unavailable embedded
builder identity, input drift, and filesystem output failures are `build_failed`.
If output itself cannot be written, the command exits nonzero without a complete
report. Treat only a complete success report **and** zero exit status as success;
assets may already exist when writing the final report fails. The JSON contract
is [catalog-build v1](../schemas/cli/v1/catalog-build.schema.json).

The obsolete source-only snapshot command has been removed. The Catalog Project
remains pinned to its previous immutable release until the complete supporting
Packy release is available; this engine change does not update that pin.


### Exercise local candidates

Use the ordinary Packy executable to exercise complete local snapshot assets in
an explicit disposable authoring workspace. Neither Go nor a Packy source
checkout is required. Both paths must be absolute; the workspace's parent must
already exist and the workspace itself must not exist on first use:

```sh
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/workspace -- list --json
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/workspace -- show example-pack --surface claude --json
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/workspace -- get skill example-pack:guide --surface codex --json
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/workspace -- install example-pack \
  --surface codex --resource skill:guide --dry-run --json
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/workspace -- install example-pack \
  --surface codex --resource skill:guide
```

Packy validates the complete candidate before creating the workspace. Its
`snapshots/`, `home/`, `config/`, and initialized Git `project/` are disposable
owned state. The version-2 `local-candidate.json` ownership marker records the
current selection and retained snapshot identities. Older disposable workspace
formats are rejected; create a new workspace rather than editing the marker.
Ordinary candidate calls require the selected snapshot's identical assets.
Every operation revalidates the captured ownership and retained sources.
`list`, `show`, and `get` remain available when the owned project is outside Git
or its Git metadata is damaged. Native lifecycle operations and explicit
transitions additionally require the intact owned Git boundary, including after
interactive approval; an enclosing repository never supplies that authority.
Removing or editing the ownership marker during consent rejects Apply without
lifecycle mutation.

To change snapshot availability while preserving all installed baseline state,
use the explicit transition operation. Obtain the baseline archive digest from
its `catalog build --json` report or `SHA256SUMS`; the outer `--snapshot` selects
the proposed candidate assets:

```sh
packy catalog candidate --snapshot /absolute/path/to/candidate-dist \
  --workspace /absolute/path/to/workspace -- transition \
  --expect-archive-sha256 <current-baseline-archive-sha256> --json
packy catalog candidate --snapshot /absolute/path/to/candidate-dist \
  --workspace /absolute/path/to/workspace -- update example-pack \
  --surface codex --project
packy catalog candidate --snapshot /absolute/path/to/candidate-dist \
  --workspace /absolute/path/to/workspace -- update example-pack \
  --surface claude --project
```

Transition requires an existing owned workspace, the exact expected current
archive digest, and a different snapshot identity. It validates all retained
sources and the complete proposed snapshot, then changes selection without
changing projections, receipts, project intent, or unrelated files. Retained
snapshots remain immutable and resolve the installed receipts of surfaces that
have not yet been updated or removed. An identity reused with different bytes,
a stale expected digest, invalid/incompatible input, damaged isolation, or
retained-source drift is rejected without replacing prior usable state. Failed
preparation/selection preserves installed state. Transition changes availability
only; each subsequent lifecycle Apply requires its own normal consent.

Available operations after the required `--` separator are `list`, `show`,
`install`, `verify`, `uninstall`, `update`, `activate`, `deactivate`, and
`transition`. Project `install`, `verify`, `uninstall`, and `update --project`
always target `workspace/project`. Global `activate`, `deactivate`, and `update`
use only `workspace/home` and `workspace/config`:

```sh
packy catalog candidate --snapshot /absolute/path/to/baseline-dist \
  --workspace /absolute/path/to/global-workspace -- activate example-pack \
  --surface codex --resource skill:guide
# Transition this workspace with the candidate assets and its observed digest.
packy catalog candidate --snapshot /absolute/path/to/candidate-dist \
  --workspace /absolute/path/to/global-workspace -- update example-pack \
  --surface codex
packy catalog candidate --snapshot /absolute/path/to/candidate-dist \
  --workspace /absolute/path/to/global-workspace -- deactivate example-pack \
  --surface codex
```

The invoking directory, enclosing projects, ambient HOME, and XDG roots are not
lifecycle destinations. Native global skill links may point only into retained
workspace snapshots; redirected roots, project symlinks, external links,
missing isolation directories, damaged Git boundaries, and unowned workspaces
are rejected. Surface adapters retain normal selection, effective dependencies,
coexistence, drift, ownership, and exact-preview freshness checks. `activate`
and `deactivate` with `--project` remain unavailable. Post-install personal
project activation offers remain suppressed.

Candidate source/builder labels are explicitly unauthenticated. Preparation
checks the existing archive/checksum format, official Catalog Project source
label, whole-catalog compatibility, safe extraction, and exact retained file
identities. It never fabricates attestation or selects an official catalog.
Ordinary `init` and `catalog refresh` retain official publisher verification;
this authoring route is not a custom-catalog source for ordinary use. Resources
remain inert. Host/model execution, authentication, and external executable
acquisition are unavailable; the interface proves configured state rather than
actual host runtime usability.

Arguments after `--` retain the ordinary operation's versioned JSON contract.
Preview is read-only after workspace preparation. Every applicable lifecycle
Apply requires normal interactive consent; JSON and piped `yes` are not
consent. JSON reports go to stdout and candidate diagnostics go to stderr.
Failures exit nonzero; workspace preparation failures before lifecycle dispatch
have stderr diagnostics without a lifecycle report.

`transition --json` emits schema version 1, report
`catalog-candidate-transition`, provenance `local-candidate`, and result
`transitioned` with `snapshot_id`, `archive_sha256`, and
`previous_archive_sha256`. Rejections emit result `failed` with stable error
code `invalid_arguments` or `transition_failed` and an actionable message.
The offline contract is
[`catalog-candidate-transition.schema.json`](../schemas/cli/v1/catalog-candidate-transition.schema.json).
Outer CLI parsing/lock failures and output failures may have only stderr
diagnostics. A failed transition report write restores the previous selection and removes
newly retained candidate bytes.

Callers must run bounded child processes with explicit disposable HOME and XDG
environments. For Apply, attach a controlled PTY to stdin and approve the
observed preview; keep stdout/stderr separate when reading structured reports.
Use a timeout, terminate and reap the owned child on failure, close PTY handles,
and clean up only the explicitly owned workspace after collecting evidence.
Do not concurrently edit or operate on a workspace. Candidate CLI operations
serialize on the workspace parent directory; external edits still invalidate
source/preview identity rather than granting consent. Packy retains snapshots
until the caller removes the disposable workspace and does not automatically
remove authoring evidence.

Packy's inert synthetic contracts own lifecycle mechanisms and run against the
same release-stamped executable on Darwin and Linux, amd64 and arm64. The
Catalog owns actual Pack content assertions. The shared built/release executable
contract also covers on-demand discovery and retrieval for Codex and Claude Code,
complete resource/legal closure, retained nested access after an explicit
transition, inert offline use, and rejection of invalid complete catalogs.
After transition, pass the new selected assets to the outer `--snapshot`; pass a
previously returned identity to the nested `get skill --snapshot <identity>` to
retain the original procedure. Missing retained content fails even when prepared
copies remain. These operations use the supplied executable directly; downstream
Catalog acceptance needs no neighboring engine checkout or development build. Integration of candidate-interface
changes alone does not unblock Catalog #57: a separately authorized immutable
supporting Packy release and reviewed adoption of its single release declaration
must precede downstream migration and deletion of legacy probes.

### Publish prepared assets

```sh
packy catalog publish --repository yersonargotev/packy-catalog \
  --commit <full-reviewed-catalog-commit> --dist <prepared-assets-directory>
```

Run the installed Packy executable from any directory. No Packy source checkout,
Go runtime, `jq`, or checksum utility is needed. Install the GitHub CLI (`gh`) on
`PATH` and authenticate it through `GH_TOKEN` (as in the official workflow) or
`gh auth login`. The credential must have release read/write access to the
intended repository (`contents: write` in GitHub Actions). The repository must
have native release immutability enabled. Temporary space for sealed copies of
the prepared assets and downloaded verification bytes must be writable.

The command consumes exactly `catalog-snapshot.tar.gz` and `SHA256SUMS`, verifies
the archive checksum, and never builds or executes Catalog Project resources.
It checks the exact `catalog-<commit>` tag, full target commit, complete remote
asset set, and matching bytes. It creates a draft, uploads only missing matching
assets, verifies the complete download, and publishes through GitHub's native
immutability control. Unchanged immutable releases succeed without mutation;
matching interrupted drafts can resume. Different targets, unexpected or
conflicting assets, missing published assets, and mutable publications fail.
Transport errors stop the operation; repeat the same command to recover a
matching draft after an interrupted upload or lost response.

Official authority remains with the reviewed preparation and publication
workflow described above. The workflow binds prepared bytes to the protected
Catalog commit and issues the artifact attestation with `id-token: write` and
`attestations: write` before publication. This command neither creates an
attestation nor certifies caller-supplied commit labels. Publishing local
candidate fixtures does not make them attested official Catalog Snapshots;
normal consumer verification remains required.

Default output describes publication, completed interruption, or an unchanged
release. With `--json`, the version 1 `catalog-publication` report has `result`
`published`, `resumed`, or `unchanged`, plus `repository`, `commit`, `tag`, and
`archive_sha256`. Failure reports `result: "failed"` and an actionable `error`
with code `invalid_arguments`, `assets_invalid`, `transport_failed`, or
`publication_conflict` (`publication_failed` is reserved for unexpected errors).
Success exits zero; failures exit nonzero. JSON is on stdout and diagnostics
on stderr; an output-write failure may only have stderr diagnostics. The offline
schema is [catalog-publication v1](../schemas/cli/v1/catalog-publication.schema.json).
The former source-only publisher script is removed after migrating its Packy
callers; the Catalog Project continues using its previous immutable release
until adoption.

## Origin download reuse

Authoring commands retain verified upstream checkouts under the operating
system's user cache directory, in `packy/catalog-origins-v1` (on Linux,
`$XDG_CACHE_HOME` or `~/.cache`; on macOS, `~/Library/Caches`). Each exact
repository/full-commit pair has its own entry. Warm validation and builds avoid
another upstream clone, but still verify source bytes and run all candidate,
provenance, runtime-fitness, and applicable baseline checks.

Each invocation uses a private temporary checkout. Retained source entries are
sealed, copied under a cache-wide lock, and verified before use. Acquisition and
lock waiting share a five-minute deadline. Failures never publish an incomplete
entry; a later resolution cleans staging left by an interrupted process.

Resolution evicts entries unused for seven days and retains at most eight
entries, using least-recently-used eviction. This is an entry/age bound rather
than a byte quota. To reclaim all space, delete the exact
`packy/catalog-origins-v1` directory while all authoring commands are idle.
A corruption error identifies the individual entry to delete before retrying;
Packy does not silently retry corruption or switch origins. Never remove an
active cache directory, since its directory lock coordinates authoring processes.
Installed Catalog Snapshots and host configuration are separate and unaffected.
See [ADR 0042](adr/0042-reuse-verified-catalog-origin-downloads.md).
