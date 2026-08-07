# Packy release

Status: Active

## Goal

Publish one new immutable Packy version from `origin/main`, then verify the
GitHub Release and matching Homebrew installation.

## Establish

Fetch `origin` and tags. Select a new `vX.Y.Z` version that is absent from local
tags, remote tags, and GitHub Releases. Freeze the intended commit from
`origin/main`, prepare the release notes, and use a clean workspace without
changing unrelated operator state.

## Prove

Run focused checks and `./scripts/validate-packy.sh` with sandboxed `HOME` and
`XDG_CONFIG_HOME`. Build disposable artifacts with
`scripts/build-release-artifacts.sh`, validate them with
`scripts/validate-release-artifacts.sh`, and generate the formula from the same
`SHA256SUMS`. Confirm required CI is green for the commit.

## Approve

Before the tag push, present the version, commit, validation results, and the
external effects: pushing one new tag, creating one GitHub Release, and updating
`yersonargotev/homebrew-tap`. Publication requires the user's approval of that
exact version and commit.

## Publish once

Re-fetch immediately before mutation. Require the selected commit still belongs
to `origin/main` and the version remains unused. Create the tag at that commit
and push only that tag. The tag-triggered Release workflow owns all publication.

Never move, delete, or reuse a published tag. Never edit, replace, clobber, or
recover a published release or its assets. Any fault is corrected on `main` and
published as a newer version.

## Verify and close

Require the workflow to finish successfully. Read back the tag, GitHub Release,
four archives, `SHA256SUMS`, tap formula, and installed Homebrew binary. All
versions, URLs, and checksums must agree. Remove temporary state and confirm the
operator checkout retains its original branch, HEAD, and pre-existing changes.

## Repository-change exception

If publication exposes a required repository change, stop publication and
deliver that change through `deliver-issue`. Resume with a new version after the
change reaches `main`; never repair the failed version in place.
