# Preserve evidence before cleanup

Inventory every exact workflow-owned cleanup target and every decision trail
and retained handoff for the run. Include all targets in one preflight so a copy
inside another target cannot count as retained evidence. Keep the documents and
proof outside every target. Retain only the evidence needed for its claim,
without copying credentials.

## Resolve the reviewed checker

The reusable checker belongs to the Catalog Project's maintained
`pstack:show-me-your-work` adaptations. Resolve `scripts/check-retention.py`
and `references/evidence-retention.md` from the same reviewed Agents or Claude
resource root. An explicit reviewed Catalog source checkout is also usable.
Record its exact commit and helper SHA-256 with the run's evidence.

[Catalog issue #132](https://github.com/yersonargotev/packy-catalog/issues/132)
tracks this delivery prerequisite. The installed pstack 6.1.0 does not contain
the checker. This Packy change does not publish Catalog content, refresh a
snapshot, or update an installation. Until the dependency is integrated and
acquired, use an explicitly reviewed candidate source checkout or stop cleanup.
Never patch an installed or prepared resource to obtain the helper.

## Run the preflight

Read the helper's supporting reference for its supported citation forms,
resolution bases, and relocation records. Resolve TSV paths against the
explicit `--base`. Resolve handoff links against their containing document.
Use the existing six-column TSV and Markdown documents. Preserve historical
rows and append explicit relocation records after copying affected originals.
Copies must match available originals byte for byte and survive every target.

Run the helper with explicit absolute document and target paths. Repeat each
flag for every corresponding input.

```sh
python3 -B "$PACKY_EVIDENCE_CHECKER" \
  --base "$PACKY_RUN_BASE" \
  --trail "$PACKY_RETAINED_TRAIL" \
  --handoff "$PACKY_RETAINED_HANDOFF" \
  --target "$PACKY_OWNED_SCRATCH" \
  --target "$PACKY_OWNED_WORKTREE"
```

If a document kind does not exist for the run, declare `--no-trail` or
`--no-handoff` instead of omitting its inventory. Keep stdout, stderr, exit
status, exact inputs, source identity, and final-state observations outside the
cleanup targets. The checker only reads. It performs no copy, reference update,
or deletion.

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
