# Register a private catalog

A private catalog is a private GitHub repository that publishes Catalog
Snapshots under the same release contract as every other catalog. Packy reads
it with `--trust access-controlled` and a GitHub token that you provide.

## Know what access-controlled trusts

An `attested` catalog proves its publisher with a GitHub build-provenance
attestation. GitHub offers attestations for private repositories only on
GitHub Enterprise Cloud, so a private catalog uses `access-controlled`.
It trusts GitHub access control to the repository in place of the attestation.
Packy still checks the release tag, the publisher, the source commit, the API
digest, and the checksum of every Catalog Snapshot.

`access-controlled` is a weaker trust level, and `packy catalog list` and
`packy list` say so. Packy accepts it only when the GitHub API reports the
repository as private. Register a public repository with `--trust attested`.
Packy sends no token for an `attested` catalog, so a private repository
cannot register as `attested`, even on GitHub Enterprise Cloud.

## Provide a GitHub token

Packy reads a token only when `packy catalog add` or `packy catalog refresh`
reaches an access-controlled catalog. It takes the first token from this list:

1. The `GH_TOKEN` environment variable.
2. The `GITHUB_TOKEN` environment variable.
3. The output of `gh auth token --hostname github.com`.

The token needs read access to the repository contents. Use a fine-grained
token with read-only Contents permission on the repository, or a classic token
with the `repo` scope. A `gh auth login` session has the `repo` scope.

Packy sends the token only to `https://api.github.com`, and it refuses an API
redirect that would carry the token anywhere else. It downloads release assets
through the release asset API, and it follows the redirect to GitHub's asset
host without the token. Packy never writes the token to disk.

## Register the catalog

To register the catalog with a token from the environment, run:

```sh
GH_TOKEN=<token> packy catalog add <owner/repo> --trust access-controlled
```

If you signed in with `gh auth login`, omit `GH_TOKEN`. Then run
`packy catalog list` to confirm the `access-controlled` trust policy and the
selected Catalog Snapshot.

## Refresh and use the catalog

`packy catalog refresh` reads the token again for each access-controlled
catalog. Without a token, the private catalog fails and keeps its previous
Catalog Snapshot. The other catalogs refresh, and the command exits 1.

Installation, `packy list`, `packy status`, `packy get skill`, and
`packy export` read the retained Catalog Snapshot. They need no token and no
network.

## Fix a refused registration

A refused registration leaves no registration and no Catalog Snapshot. The
error says which case applies:

- `needs a GitHub token` means that no source holds a token. Set `GH_TOKEN`
  or run `gh auth login`.
- `GitHub rejected the token` means that the token is invalid or expired.
  Renew it, or run `gh auth login` again.
- `needs read access to the repository and its contents` means that GitHub
  hides the repository or its releases from the token. Check the repository
  name and the token permissions.
- `the repository is public` means that GitHub reports the repository as
  public. Register it with `--trust attested`.
