---
status: accepted
---

# Support multiple and private Pack catalogs

ADR 0039 made the reviewed Pack catalog an independent publication but fixed
consumption to the single official public Catalog Project. People also need to
consume Packs published by other maintainers, including Packs that must stay in
private repositories. This decision replaces ADR 0039. It keeps that ADR's
authoring and publication decisions and generalizes consumption to several
registered catalogs. The decisions below were confirmed during design review
on 2026-11-26.

## Retained authoring and publication decisions

The Catalog Project `yersonargotev/packy-catalog` is the canonical authoring
location for the official Packs. Independent upstream products, including
Engram, keep their own repositories, and consolidating Pack authorship does not
authorize archiving or deleting them.

Every Catalog Publication contains one complete immutable Catalog Snapshot.
Packs keep independent semantic versions: changed Pack content requires a
version bump for that Pack, and unchanged Packs keep their versions. Catalog
identity is generated from the source commit and digest, together with a
generated index. Published bytes cannot be replaced, and retrying the same
publication is idempotent. Catalog publication is separate from the Packy
executable release lifecycle.

Review and merge of a content pull request, with required validation passing,
authorize automatic publication of its resulting Catalog Snapshot. One content
pull request may add one Pack and update another.

Packy provides CLI authoring operations for creation, import, Upstream Refresh,
and validation. Maintainers explicitly select resources, destinations,
supported hosts, and adaptations, and templates and standard metadata detection
reduce repetitive input without heuristic conversion of arbitrary repositories.
Imports preserve provenance and notices. Upstream Refresh prepares and validates
its changes before writing, stops on unexpected local modifications to an exact
copy, and requires explicit reconciliation of adapted resources. Authoring
operations never publish automatically. A TUI Pack editor is out of scope.

These operations, and the publication command, work for any Catalog Project,
not only the official one. Packy documents a reference publication workflow for
third-party maintainers but does not generate Catalog repositories.

## Catalog registration

A catalog is a GitHub repository identified by its `owner/repo`, with no local
aliases. Every catalog publishes the same contract: an immutable stable release
tagged `catalog-<commit>`, published by `github-actions[bot]`, containing
exactly `catalog-snapshot.tar.gz` and `SHA256SUMS`, whose index names the same
repository and commit and was built by a Packy release. Arbitrary URLs, other
forges, and unpublished local directories are not catalogs.

Catalogs are registered per workstation. The official catalog is registered by
the Packy executable, cannot be removed, and has no precedence over other
catalogs. Registering a catalog downloads, fully validates, and selects its
initial Catalog Snapshot, and the registration exists only if that succeeds. A
project does not declare catalogs. Its lock records the catalog of each Pack,
and an operation that needs an unregistered catalog fails and names it.

Removing a catalog is refused while an installed Pack receipt on the
workstation references it. Removal does not delete retained snapshots.

## Catalog trust policy

Each registration fixes one explicit trust policy, which is never inferred:

- `attested` requires a verified GitHub build-provenance attestation of the
  archive, signed by the catalog's own publication workflow on `refs/heads/main`
  on a GitHub-hosted runner. The workflow path is fixed at registration and
  defaults to `.github/workflows/publish.yml`. The official catalog is
  `attested`.
- `access-controlled` is allowed only for private repositories. It relies on
  GitHub access control together with the release, publisher, tag, commit, API
  digest, and checksum verifications that every catalog receives. Packy shows it
  as a weaker trust level.

`access-controlled` exists because GitHub offers artifact attestations for
private repositories only on GitHub Enterprise Cloud. Two alternatives were
rejected: requiring attestation would make private catalogs unavailable on
other plans, and keyless signing through the public Sigstore instance would
publish private repository and workflow identities to a public transparency
log. There is no mode that skips verification, and a public repository cannot use
`access-controlled`.

A private repository can also be `attested` when GitHub attests its releases.
GitHub signs those attestations with its own Sigstore instance, which uses a
timestamp authority instead of a public transparency log. Packy verifies them
against that instance's trusted root, refreshed over TUF from
`https://tuf-repo.github.com` with an embedded TUF root, and requires the same
publisher identity as for the public instance. An `attested` catalog is read
without a token first, and Packy reads a token only when GitHub answers 404.

Packy never stores credentials. Only explicit network operations (registration
and Catalog Refresh) read `GH_TOKEN`, `GITHUB_TOKEN`, or `gh auth token`, and a
missing or insufficient credential fails with an explanation.

## Pack identity across catalogs

Pack IDs stay global. Packy rejects a whole Catalog Snapshot, at registration
or refresh, when it introduces a Pack ID that is present in the selected
snapshot of another registered catalog. Qualified skills keep the
`<pack>:<skill>` form and project Pack manifests do not name catalogs. Catalog
qualification was rejected because it would change every Pack and skill
reference to support coexisting forks, which is not a current need.

A Pack's dependency closure is resolved within its own Catalog Snapshot. Pack
or skill dependencies never cross catalogs.

An installed Pack receipt and the project Pack lock bind each Pack to its
catalog and snapshot. A lifecycle update looks for newer versions only in that
catalog. Moving a Pack ID to another catalog requires an explicit uninstall
followed by a new installation.

## Consumption

Catalog Refresh is explicit. It refreshes every registered catalog, or one named
catalog. Each catalog's refresh is atomic and independent: a failure leaves that
catalog's previous selection in place, does not undo the others, and makes the
command exit non-zero after it reports every result. Listing, status inspection,
on-demand skill access, plugin export, and activation from downloaded content
never discover newer publications over the network.

A new snapshot that needs vocabulary or capabilities the installed Packy does
not support is rejected as a whole, even when the incompatible content belongs
to an unselected Pack. The previously selected snapshot remains usable, and the
failure explains the required engine update. Partial catalog loading is not
supported.

Downloaded Catalog Snapshots are retained without automatic cleanup.
Activations keep referencing their immutable source snapshot until their own
explicit lifecycle update, and refreshing a catalog never rewrites the content
behind active references. A Pack removed from a catalog's current snapshot can
no longer be newly activated from that catalog, but existing installations stay
inspectable and deactivatable. A defective Pack is corrected by a newer Pack
version in a new Catalog Publication. Downgrades and automatic uninstall are out
of scope.

The TUI shows registered catalogs with their trust policy and selected snapshot,
shows each Pack's catalog, and refreshes catalogs. Registration and removal are
CLI-only.

## Adoption

Adoption is a clean cut. Installed Pack receipts, project Pack locks, and the
catalog store change schema to record catalog identity, and earlier state is
rejected with verifiable adoption instructions. No converter exists, and old
receipts are never read as official-catalog state. As with ADR 0039, users
deactivate or uninstall affected installations with the previous Packy version
and reinstall them. Personal files, credentials, and Memory are preserved, and
old source directories are not deleted automatically.

## Acceptance

The main acceptance seam is the real installed CLI against a controlled GitHub
transport. That transport serves the official catalog, a second public
`attested` catalog, a private `access-controlled` catalog that requires a
token, and a private `attested` catalog that requires a token. The CLI must cover registration, refresh, and removal. It must also show
that a Pack ID collision, a missing credential, and an incompatible snapshot are
rejected without partial changes, that offline consumption works, that a project
lock naming an unregistered catalog fails, and that the TUI behaves the same. A
manual smoke test consumes a real private catalog. Lower-level tests target
faults that are hard to reproduce through the observable seam.

## Consequences

This decision supersedes ADR 0039. Like ADR 0039, it supersedes ADR 0031's fixed
four-Pack catalog and Pack authoring coupled to Packy releases, and ADR 0038's
one-repository-per-Pack authoring, registry, and promotion model. It preserves
ADR 0031's installed-receipt, ownership, and protected-integration contracts,
the reviewed resource contract, provenance and notices, Pack ownership, and the
capability and readiness principles of ADR 0035. Every existing user must
reinstall their Packs once. Private catalogs that use `access-controlled` accept
GitHub repository access, not build provenance, as their publisher evidence.
