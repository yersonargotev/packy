# Evidence retention acceptance

This synthetic acceptance proves the partial-copy mechanism in issue #911. It
does not recover the five missing historical originals from #907. Later release
acceptance remains separate proof.

## Candidate and dependency

Packy changes only its active development and release cleanup policy and adds
an opt-in routing probe. The reusable checker and procedure are authored in
[Catalog dependency #132](https://github.com/yersonargotev/packy-catalog/issues/132).
The Packy PR does not publish that dependency or change installed resources.

The exercised helper SHA-256 is
`10e439dfecd02d11ac8217c1aedf6ed4dc20ae19e2010f3eff4f7301262b6386`.
The supporting reference SHA-256 is
`663fac035eea563d13b533aa1d0f22d3e165b5296c2d7df49fd60d994b62923e`.
The reports bind every copied Packy instruction by SHA-256. The final PR records
the exact reviewed candidate commits and validation results.

## Regression

The [baseline](baseline-reproduction.json) has six live original pointers but
only one retained copy. The [red probe](catalog-red.log) fails before the helper
exists. The [green probe](catalog-green.log) executes both adapted helpers and
covers complete retention, missing originals and destinations, wrong bytes,
destinations in other cleanup targets, absent targets, unsupported citations,
symlinks, and unchanged unrelated state.

The lead's initial [partial reproduction](checked-reproduction-partial.json) observes
exit 1 and names all five unretained originals. The [completed reproduction](checked-reproduction-complete.json)
observes exit 0 after identical copies and relocation rows are added. Both
checks leave files unchanged and preserve the original trail prefix. They record
the initial helper identity. The final four routing runs below bind the repaired
helper bytes and preserve its per-artifact digest output.

Independent review found missing durable identity output and false success for
filesystem aliases. The [identity regression](identity-red.log) fails before
per-copy hashes are logged and [passes afterward](identity-green.log). The
[alias regression](alias-red.log) reproduces the false success and
[passes after repair](alias-green.log). Both adaptations now verify containment
by filesystem identity and print original, retained path, and matched SHA-256.
The retained digest remains checkable after the fixture original is removed.

A further review exposed invalid raw paths that passed after normalization. The
[raw-path regression](path-resolution-red.log) fails before component validation
and [passes after repair](path-resolution-green.log). The lead independently
checked a [ten-case path matrix](raw-resolution-matrix.json), including valid
parent traversal, missing and non-directory components, trailing file slashes,
symlinks, and unsupported percent paths. Inputs remain unchanged.

## Actual controlled tool traces

Four fresh Codex turns read candidate instructions and execute the real checker.
Each fixture has a private Git repository, an unrelated worktree, a separate
operator sentinel, six originals, and one or six retained copies. Prepared
integration and release read-back statements are fixture inputs. No real
integration or publication occurred.

| Entry point | Partial copy | Complete retention |
| --- | --- | --- |
| Development | [Refusal](development-partial/report.json), [tool trace](development-partial/tool-trace.json) | [Success](development-complete/report.json), [tool trace](development-complete/tool-trace.json) |
| Release | [Refusal](release-partial/report.json), [tool trace](release-partial/tool-trace.json) | [Success](release-complete/report.json), [tool trace](release-complete/tool-trace.json) |

The partial traces have a nonzero checker result and no deletion attempt. The
complete traces show exit zero before a separate exact-path deletion.
The `after.json` beside each report independently records the observed final
files. Retained proof and reference bytes, operator files, unrelated worktrees,
Git refs, and worktree registrations remain unchanged. Only the authorized
complete fixtures remove their disposable target.

The checked-in traces contain the checker, deletion, and applicable state-read
tool events extracted from the raw command records. Prompts, before and after
state, replies, and source hashes remain beside each report. Full raw JSONL and
all commands are retained outside disposable state under
`/private/tmp/packy-911-evidence/routing-v4-*`.

The first routing attempt produced observer failures for nested or redirected
checker commands, shell quoting, and newly written continuation records. Its
raw traces remain retained. The final probe requires standalone checker commands
and keeps continuation records outside the fixed handoff. Those final runs are
the four passing reports linked above.

## Limits

The checker supports a documented citation subset. Unsupported local forms
block. It requires live originals for copy comparisons and does not consume
historical identity attestations. It checks the supplied inventory at one point
in time. Concurrent changes require another preflight. Ownership validation,
pending-work preservation, deletion authority, and postconditions remain
separate requirements.

The external deslop and control-cli integrations were unavailable. The lead
reviewed the diff directly and drove the actual helper and model tool traces.
An independent comment audit found one redundant probe docstring and one stale
candidate-state description. Both were corrected. Standards and Spec review
the final candidates separately. No compatible different-family trail reviewer
is configured on this host; a diverse-family review is not claimed.
