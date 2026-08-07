# Publish a Packy release

Packy uses one conventional, immutable tag-driven release flow. A version tag
whose commit is on `main` builds and publishes the supported archives, one
`SHA256SUMS` file, one GitHub Release, and the matching Homebrew formula.

Published tags, releases, assets, and formulas are never repaired in place. If
a published release is faulty or incomplete, fix the cause on `main` and
publish a newer version.

## User install and upgrade

The [README quickstart](../README.md#quickstart) is the canonical Homebrew
installation path. Direct-download users choose the archive matching their OS
and architecture, verify it with `SHA256SUMS`, extract `packy` onto `PATH`, and
then follow the same `packy init` flow.

Homebrew users upgrade the binary and align the Installed Source before
updating an active Pack:

```bash
brew upgrade packy
packy init
packy pack update engram --surface codex --dry-run
packy pack update engram --surface codex
```

## Maintainer flow

1. Choose a new `vX.Y.Z` version. It must not reuse any existing tag or GitHub
   Release.
2. Finalize `docs/release-notes/next.md` on `main` and keep its single
   `{{TAG}}` placeholder.
3. Run focused checks for the changed code, then run the general local check:

   ```bash
   ./scripts/validate-packy.sh
   ```

4. Create the version tag on the current `main` commit and push that tag.
5. Wait for the Release workflow. It validates the tagged source, builds and
   verifies the distribution, creates the GitHub Release once, updates
   `yersonargotev/homebrew-tap`, and tests the installed Homebrew binary.
6. Confirm that the workflow, GitHub Release, and tap all name the same version.

There is no manual publication, candidate sealing, admission, recovery,
provenance, SBOM, or attestation path. Never rerun publication to mutate an
existing version. Correct failures after publication with a newer version.

## Artifact contract

`scripts/build-release-artifacts.sh --version <tag> --out-dir <dir>` builds:

```text
packy_<tag>_darwin_amd64.tar.gz
packy_<tag>_darwin_arm64.tar.gz
packy_<tag>_linux_amd64.tar.gz
packy_<tag>_linux_arm64.tar.gz
SHA256SUMS
```

Each archive contains one executable named `packy`. `SHA256SUMS` contains
exactly one digest for each archive. The workflow builds this directory once
and passes the same bytes to validation and publication.

`scripts/validate-release-artifacts.sh` is release-only validation. It rejects
missing or extra artifacts, malformed or mismatched checksums, and archives
whose contents differ from the contract. It extracts the current runner's
archive into a temporary installation root and requires the installed binary's
`--version` output to match the tag.

`scripts/generate-homebrew-formula.sh` generates `Formula/packy.rb` from the
same tag and checksum manifest. After the GitHub Release and formula are
published, the macOS package-smoke job runs `brew install --formula` and checks
the installed binary again.

## Publication boundaries

The workflow starts only on a version-tag push and verifies that the tagged
commit is the current `main` commit. Read-only preparation has `contents: read`; GitHub
publication alone has `contents: write`; the tap credential is available only
to the protected `homebrew` job. External actions are pinned to immutable
commits.

`gh release create` is invoked once without asset clobbering. The workflow has
no path to edit, delete, recreate, resume, or recover a release. The Homebrew
job publishes only `Formula/packy.rb`, and the final package smoke is downstream
of both publication steps. Publication reads the GitHub assets and tap formula
back from their providers and requires them to match the validated bytes.
