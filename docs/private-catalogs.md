# Register a private catalog

A private catalog is a private GitHub repository that publishes Catalog
Snapshots under the same release contract as every other catalog. Packy reads
it with a GitHub token that you provide. You register it with one of two trust
policies.

## Choose a trust policy

- `attested`, the default, requires a verified GitHub build-provenance
  attestation of each Catalog Snapshot. It is the check that every public
  catalog receives. GitHub offers artifact attestations for private
  repositories only on GitHub Enterprise Cloud.
- `access-controlled` trusts GitHub access control to the repository in place
  of the attestation. Use it when the repository cannot publish attestations.

Under both policies, Packy checks the release tag, the publisher, the source
commit, the API digest, and the checksum of every Catalog Snapshot.

## Know what attested trusts

An `attested` catalog proves its publisher with an attestation that the
registered publication workflow signed on `refs/heads/main` on a
GitHub-hosted runner. Publish it as
[Publish a third-party catalog](catalog-project.md#publish-a-third-party-catalog)
describes.

GitHub signs the attestations of a private repository with its own Sigstore
instance, so the repository and workflow names never reach a public
transparency log. Packy verifies them against the trusted root of that
instance, which it refreshes from `https://tuf-repo.github.com` during
registration, refresh, and restore. A public catalog's attestations come from
the public Sigstore instance. The publisher identity that Packy requires is the
same for both instances.

Packy reads the release of an `attested` catalog without a token first. That
is the latest release, or for `packy catalog restore` the release of one exact
commit. GitHub answers 404 for a private repository, and only then does Packy
read a token and repeat the request with it. The release assets and the
attestations are then read with the same token. A public `attested` catalog
with a published release never reads a token. A missing repository or a
repository without a stable release also answers 404, so Packy reads a token
for it too, and the error then says that the token needs read access.

## Know what access-controlled trusts

`access-controlled` relies on GitHub access control to the repository instead
of build provenance. It is a weaker trust level, and `packy catalog list` and
`packy list` say so. Packy accepts it only when the GitHub API reports the
repository as private. Register a public repository with `--trust attested`.

## Provide a GitHub token

Packy reads a token only when `packy catalog add`, `packy catalog refresh`, or
`packy catalog restore` reaches a private catalog. It takes the first token
from this list:

1. The `GH_TOKEN` environment variable.
2. The `GITHUB_TOKEN` environment variable.
3. The output of `gh auth token --hostname github.com`.

Packy uses the first token it finds and never falls back to the next source.
An invalid `GH_TOKEN` fails the command even when `GITHUB_TOKEN` or `gh`
holds a valid token, so unset it or replace it.

The token needs read access to the repository contents. An `attested` catalog
also needs read access to its attestations. Use a fine-grained token with
read-only Contents permission on the repository, and read-only Attestations
permission for an `attested` catalog. A classic token with the `repo` scope
also works. A `gh auth login` session has the `repo` scope.

Packy sends the token only to `https://api.github.com`, and it refuses an API
redirect that would carry the token anywhere else. It downloads release assets
through the release asset API, and it follows the redirect to GitHub's asset
host without the token. Packy never writes the token to disk.

## Register the catalog

To register an `attested` catalog with a token from the environment, run:

```sh
GH_TOKEN=<token> packy catalog add <owner/repo>
```

Add `--workflow <path>` when the publication workflow is not
`.github/workflows/publish.yml`. To register an `access-controlled` catalog,
run:

```sh
GH_TOKEN=<token> packy catalog add <owner/repo> --trust access-controlled
```

If you signed in with `gh auth login`, omit `GH_TOKEN`. Then run
`packy catalog list` to confirm the trust policy and the selected Catalog
Snapshot.

## Refresh and use the catalog

`packy catalog refresh` reads the token again for each private catalog.
Without a token, the private catalog fails and keeps its previous Catalog
Snapshot. The other catalogs refresh, and the command exits 1.

Installation, `packy list`, `packy status`, `packy get skill`, and
`packy export` read the retained Catalog Snapshot. They need no token and no
network.

## Fix a refused registration

A refused registration leaves no registration and no Catalog Snapshot of the
refused catalog. When no official Catalog Snapshot is selected yet,
`packy catalog add` selects one first, and that selection stays after the
refusal. The error says which case applies:

- `needs a GitHub token` means that no source holds a token. Set `GH_TOKEN`
  or run `gh auth login`.
- `GitHub rejected the token` means that the token is invalid or expired.
  Renew it, or run `gh auth login` again.
- `needs read access to the repository and its contents` means that GitHub
  hides the repository or its releases from the token. Check the repository
  name and the token permissions. After `fetch catalog artifact attestations`,
  it also means that GitHub holds no attestation of the Catalog Snapshot that
  the token can read. Check the Attestations permission. A repository that
  cannot publish attestations uses `--trust access-controlled`.
- `no catalog attestation verified` means that no attestation of the Catalog
  Snapshot was signed by the registered workflow on `refs/heads/main` on a
  GitHub-hosted runner. If the catalog publishes from another workflow,
  register it with `--workflow <path>`.
- `the repository is public` means that GitHub reports the repository as
  public. Register it with `--trust attested`.
