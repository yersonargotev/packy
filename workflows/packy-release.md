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
candidate release-note summary or diff evidence. Incomplete or inaccurate notes
require the repository-change exception before publication.

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

The tag-triggered workflow additionally executes the same complete Catalog CLI
contract against all four staged release archives in native compatible
Darwin/Linux amd64/arm64 runtimes before publication. Every target must pass;
ordinary CI exercises this same staged-artifact gate with unpublished fixtures.
Retain the release-run contract evidence, identities, and four archive checksums
for Catalog adoption. No live Catalog Publication or model/host service is part
of contract proof.

## Approve

Before the tag push, present the prior release, version, commit, validation
results, release-note evidence as a summary or diff, and the external effects:
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

Use `gh run watch <run-id> --compact --interval 45 --exit-status` for progress.
Keep the watcher resumable while checking protected-environment waits. Filter
run and pending-deployment reads with `--jq` to the run ID, version, commit,
job status, environment name/ID, and approval eligibility needed by the current
gate; inspect additional deployment fields only to establish that same identity.
Report stage changes, failures, and decisions. During an unchanged wait, use
a brief heartbeat when the host requires updates.

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

Before a tag is pushed, invoking `release-packy` authorizes a correction confined
to `docs/release-notes/next.md` when Establish finds incomplete or inaccurate
notes. Create or adopt exactly one issue for that correction, record the release
request and this inherited authority in its body, and apply `status:approved`.
Its objective and acceptance criteria must cover the observed release delta and
note contracts. Pass that issue number to `deliver-issue` and complete its full
protected integration, review, validation, closure, and cleanup. This bounded
handoff requires no additional confirmation. If a competing delivery exists,
resolve ownership through the issue workflow before proceeding.

Changes beyond that file, or required after a tag has been pushed, need separate
approval of their numbered issue before delivery. Stop publication and present
the observed change and scope for that decision.

After delivery reaches `main`, restart Establish and discard proof and publication
approval tied to the earlier commit. A candidate version that remains unused may
be selected again before any tag push. Once a tag has been pushed, correct the
cause on `main` and select a newer unused version; never repair that version
in place. The exact-version/commit publication brief remains mandatory.

**Complete when:** `deliver-issue` reaches its Definition of done and a new
Establish run can begin from the resulting `origin/main` commit.
