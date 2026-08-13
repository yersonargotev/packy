# Packy release

Status: Active

## Goal

Publish one new immutable Packy version from `origin/main`, then verify the
GitHub Release and matching Homebrew installation.

## Establish

Fetch `origin`, tags, and GitHub Releases. Resolve the latest published stable
GitHub Release by SemVer. Use the version supplied by the user, or default to
the next patch version after that release. Require the selected `vX.Y.Z` to be
absent from local tags, remote tags, and GitHub Releases, then freeze the
current commit from `origin/main` in a clean workspace without changing
unrelated operator state.

Compare the latest published release tag with the frozen commit. Require
`docs/release-notes/next.md` on `origin/main` to account for every user-visible
change in that range, contain exactly one `{{TAG}}` placeholder, and keep every
factual version claim consistent with its repository authorities. Record a
candidate release-note summary and diff evidence. Incomplete or inaccurate
notes require the repository-change exception before publication.

**Complete when:** the prior stable release, selected unused version, frozen
commit, complete release delta, accurate release notes, and preserved operator
state are all known and checkable.

## Prove

Run focused checks and `./scripts/validate-packy.sh` with sandboxed `HOME` and
`XDG_CONFIG_HOME`. Build disposable artifacts with
`scripts/build-release-artifacts.sh`, validate them with
`scripts/validate-release-artifacts.sh`, and generate the formula from the same
`SHA256SUMS`. Confirm required CI is green for the commit.

**Complete when:** focused checks, canonical validation, disposable artifacts,
artifact validation, formula generation, and required CI all pass for the
frozen commit and selected version.

## Approve

Before the tag push, present the prior release, version, commit, validation
results, release-note summary and diff evidence, and the external effects:
pushing one new tag, creating one GitHub Release, and updating
`yersonargotev/homebrew-tap`. Publication requires the user's approval of that
exact version and commit.

That approval also authorizes the `release` and `homebrew` environment
approvals only when each pending deployment belongs to the same workflow run,
version, and commit. Any mismatch requires stopping publication.

**Complete when:** the user has approved the exact publication brief.

## Publish once

Re-fetch immediately before mutation. Require the selected commit still equals
`origin/main` and the version remains unused. Create the tag at that commit
and push only that tag. The tag-triggered Release workflow owns all publication.

When the workflow requests a protected environment approval, read back the
pending deployment before approving it and apply the publication approval only
to the matching environment, workflow run, version, and commit.

Never move, delete, or reuse a published tag. Never edit, replace, clobber, or
recover a published release or its assets. Any fault is corrected on `main` and
published as a newer version.

**Complete when:** the one new tag has been pushed once and the matching Release
workflow owns every remaining publication effect.

## Verify and close

Require the workflow to finish successfully. Read back the tag, GitHub Release,
four archives, `SHA256SUMS`, tap formula, and installed Homebrew binary. All
versions, URLs, and checksums must agree. Remove temporary state and confirm the
operator checkout retains its original branch, HEAD, and pre-existing changes.

**Complete when:** the workflow and every read-back agree and temporary state
is removed without changing pre-existing operator state.

## Repository-change exception

If publication exposes a required repository change, stop publication and
deliver that change through `deliver-issue`. Resume with a new version after the
change reaches `main`; never repair the failed version in place.

**Complete when:** `deliver-issue` reaches its Definition of done and a new
Establish run can begin from the resulting `origin/main` commit.
