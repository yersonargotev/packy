# Publish a Packy release

Packy uses one immutable, version-tag publication flow. A version tag on the
current `main` commit builds Darwin and Linux binaries for amd64 and arm64,
`SHA256SUMS`, one GitHub Release, and a matching Homebrew formula.

If a published release is faulty, fix the cause on `main` and publish a newer version.
Published tags and assets never change.

## User install and upgrade

Homebrew users install or upgrade Packy, then prepare the packaged content:

```sh
brew install yersonargotev/tap/packy
packy init
```

Direct-download users choose the archive matching their OS and architecture,
verify it with `SHA256SUMS`, extract `packy` onto `PATH`, and run `packy init`.

Users moving from `v0.1.x` first follow the
[one-time v0.2 reset](reset-v0.2.md).

## Maintainer flow

1. Resolve the latest published stable GitHub Release. Use a supplied version,
   or default to its next patch version, and prove the candidate is unused.
2. Compare that release with the candidate commit. Finalize
   `docs/release-notes/next.md` on `main` so it accounts for every user-visible
   change, retains exactly one `{{TAG}}` placeholder, and keeps factual version
   claims consistent with their repository authorities.
3. Run focused checks, then the general local validation:

   ```sh
   ./scripts/validate-packy.sh
   ```

4. Present the exact version, commit, validation, release-note summary, diff
   evidence, and publication effects for approval.
5. Create the version tag on the current `main` commit and push that tag.
6. Wait for the Release workflow to build and validate the artifacts, create
   the GitHub Release, update `yersonargotev/homebrew-tap`, and test the
   installed Homebrew binary. The publication approval covers protected
   `release` and `homebrew` deployments only for the same workflow run,
   version, and commit.
7. Confirm that the tag, GitHub Release, checksums, formula, and installed
   binary all name the same version.

## Artifact contract

`scripts/build-release-artifacts.sh --version <tag> --out-dir <dir>` builds:

```text
packy_<tag>_darwin_amd64.tar.gz
packy_<tag>_darwin_arm64.tar.gz
packy_<tag>_linux_amd64.tar.gz
packy_<tag>_linux_arm64.tar.gz
SHA256SUMS
```

Each archive contains one executable named `packy`. `SHA256SUMS` contains one
digest for each archive.

`scripts/validate-release-artifacts.sh` verifies the artifact names, archive
contents, checksums, and the installed binary's `--version` output.
`scripts/generate-homebrew-formula.sh` generates `Formula/packy.rb` from the
same version and checksum manifest. The release workflow then tests the
published Homebrew installation.

## Publication boundaries

The workflow starts only on a version-tag push and requires that the tagged
commit is the current `main` commit. Read-only build steps have `contents:
read`; GitHub publication alone has `contents: write`; the tap credential is
available only to the protected Homebrew job. External actions are pinned to
immutable commits.

The workflow creates each version once. The final package smoke runs only after
the GitHub Release and Homebrew formula are published and verified.
