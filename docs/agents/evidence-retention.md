# Preserve evidence before cleanup

Use this lifecycle for development and release evidence. Collect only what the
run needs to prove its claims and resume work. Set `PYTHONDONTWRITEBYTECODE=1`
before launching Python. Do not copy a whole sandbox, project, or home directory
into evidence. Keep generated caches and disposable fixture state out of durable
evidence. Record a tested binary or archive's identity, commands, results,
checksums, and publication URLs where applicable. Keep a payload only when a
named unresolved claim needs it for investigation or reproduction.

## Keep active proof

Keep original citations, decision trails, and handoffs for active, pending, or
unresolved work. Keep their proof outside every cleanup target. A summary does
not replace required proof, and age does not authorize deleting pending evidence.
Inventory every exact workflow-owned cleanup target and every decision trail
and retained handoff for the run. Include all targets in one preflight so a copy
inside another target cannot count as retained evidence. Retain only the
evidence needed for its claim, without copying credentials.

Cleaning disposable scratch while keeping the proof is not evidence retirement.
It needs no completed-run closeout. Use the normal preflight with all retained
trails and handoffs, verified copies, and relocation records. After deletion,
check the retained files against the recorded hashes, not against absent originals.

## Retire completed detail

A development run is complete after observing the candidate merged and its issue
closed. A release run is complete after successful publication and all required
readbacks. In either case, no correction, investigation, or owned process may
remain pending. Stop retirement if a raw detail is still needed to resolve a
claim. Record any deliberately retained raw artifact, its named claim, and the
reason it remains necessary.

Before retirement, preserve a compact closeout reachable from the PR description
or existing publication record. Include the approved scope, candidate identity,
commands and results, reviews, limitations, and persistent issue, PR, release,
and check references as applicable. State which raw detail is retired and that
hashes record earlier observations; hashes cannot reconstruct deleted originals.
Do not send an unsolicited message to make the closeout reachable.

Record the exact retirement inventory and its owner outside the targets. Treat
historical target paths as inventory data, not live citations. If the inventory
contains retired paths, keep it in a separate record linked from the closeout.
The inventory is a record of past targets, not a retained handoff with live
citations. Explicitly retire the historical working trail and handoffs together
with their raw details. Do not edit trail rows to pretend proof remains, or omit
active documents from preflight. Inventory the retired documents as targets.
Cite each deliberately retained raw artifact in the final summary or a retained
trail or handoff. Pass those documents to the checker. All their local references
must survive and pass the unchanged checker. The retirement decision does not
bypass the separate ownership and deletion authority checks.

Local raw evidence is temporary through handoff and is removed at observed
closure once the closeout exists and cleanup is authorized. A diagnostic grace
period needs an explicit reason and expiry. On the next related run, offer or
apply authorized cleanup of owned completed-run evidence; do not promise a
background cleanup.

## Use the repository checker

Packy's cleanup policy owns `scripts/check-evidence-retention.py` in this
repository. Use the checker from the same candidate checkout as the development
or release instructions. Record the candidate commit, working-tree changes, and
helper SHA-256 with the run's evidence. Set `PACKY_SOURCE_ROOT` to that checkout
and `PACKY_RUN_BASE` to the directory used to resolve trail paths. Python 3 with
its standard library is required.

## Run the preflight

Use the supported citation forms below. Resolve TSV paths against the explicit
`--base`. Resolve handoff links against their containing document.
Use the existing six-column TSV and Markdown documents. For each retained trail,
preserve historical rows and append explicit relocation records after copying
affected originals.
Copies must match available originals byte for byte and survive every target.
Append a six-column TSV row with phase `relocation` for each copy. Its evidence
cell must be one inline Markdown link: the label is the original absolute path,
and the destination is the retained absolute path. Keep the original rows and files through preflight. Conflicting or chained
mappings block the check.

Run the helper with explicit absolute document and target paths. Repeat each
flag for every corresponding input.

```sh
python3 -B "$PACKY_SOURCE_ROOT/scripts/check-evidence-retention.py" \
  --base "$PACKY_RUN_BASE" \
  --trail "$PACKY_RETAINED_TRAIL" \
  --handoff "$PACKY_RETAINED_HANDOFF" \
  --target "$PACKY_OWNED_SCRATCH" \
  --target "$PACKY_OWNED_WORKTREE"
```

If the run has no retained documents of a kind, declare `--no-trail` or
`--no-handoff`. Explicitly retired originals can still exist inside the targets
until deletion and remain in the retirement inventory. Do not omit active or
deliberately retained documents. Keep stdout, stderr, exit
status, exact inputs, source identity, and final-state observations outside the
cleanup targets. Preserve the checker's per-copy source, destination, and matched
SHA-256 output. After cleanup, compare those recorded hashes with the retained
files. This checks the earlier observation without claiming a new comparison
against the deleted original. The checker only reads. It performs no copy,
reference update, or deletion.

A nonzero result stops cleanup before any destructive action. Missing originals,
invalid copies, dangling citations, and unsupported or ambiguous local references
remain blocking. Correct retention while the originals exist and rerun. An
already-absent target does not excuse a dangling citation. Never assert a new
byte comparison against an absent original. An unrelated URL or later overlapping
check does not replace the original proof.

Require exit zero immediately before the separate cleanup decision. Repeat the
check after any input, evidence, target, or path topology changes. Prevent
concurrent mutation between the check and deletion. A successful result is a
snapshot of the explicit inventory, not a lock or proof of omitted documents.

## Apply the separate cleanup policy

Continue only under the active development or release policy. Preflight grants
no ownership, merge, publication, or deletion authority. Preserve pending work,
validate each exact owned path, and keep unrelated operator state and worktrees.
Use the permitted deletion primitive as a separate action. Check that intended
targets are absent and retained evidence still resolves afterward.

## Supported citations

A trail uses the exact `ts`, `phase`, `decision`, `why`, `evidence`, `result` TSV
header. Normal evidence cells contain pointers separated by commas or semicolons.
Supported pointers are absolute paths, relative paths with a slash, simple filenames
with an extension, an optional numeric `:line` suffix, ordinary inline Markdown
links, HTTPS URLs, `commit <hex>`, a 7-to-64-digit hexadecimal commit, `PR #N`,
and `#N`. Link destinations follow the same local path rules. Relocation links
require absolute local paths on both sides. Whitespace, escapes, percent signs
(including percent-encoded link destinations), shell variables, globs, URL
relocations and paths with Markdown delimiters are unsupported.
Directories and special files are not evidence files; cite their individual files.
Path components must resolve under filesystem traversal rules before normalization.
Symlink paths and ancestors are rejected, including symlinks encountered before
`..`. Missing cleanup targets are allowed, but `..` cannot follow a missing
component. Existing targets are also compared by filesystem identity with paths
and their ancestors, so aliases into targets are blocked. A hard-linked file may
therefore be treated conservatively as inside a file target. An absent cleanup
target can pass only when all citations still resolve.

In handoffs, express local citations as ordinary inline links or inline code paths.
Links resolve relative to the handoff. Plain HTTPS URLs are independent citations.
Reference-style links, nested or escaped Markdown, malformed constructs and
path-like prose block the check. Inline code must be a supported path or external
citation. The helper is not a general Markdown parser and cannot discover
implicit references in free-form prose or documents omitted from
the inventory. Rewrite mutable handoff citations into the supported form, and
record any append-only trail correction without erasing history. Unknown trail
cells block visibly; do not omit them to obtain a pass.
