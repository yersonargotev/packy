---
name: verify-projection-batches
description: Verify failure atomicity when changing Packy's local projection staging, fingerprint checks, backup/publication, or rollback in internal/localprojection.
---

# Verify projection batches

**Property:** when `ApplyFilesystemChanges` returns an error, every target retains its pre-batch content, provided rollback filesystem operations succeed.

**Harm:** a rejected installation, update, or removal can leave earlier projections changed despite reporting failure.

The `formalverification` build tag keeps the bounded replay separate from ordinary package tests. Run both checks for changes to the covered code.

## Run and interpret

1. Read accepted [ADR 0031](../../../../docs/adr/0031-simplify-packy-around-reviewed-packs.md) for receipt ownership. Compare the covered code below with the sync commit, including uncommitted changes. Confirm the model's steps still match the implementation before treating its verdict as evidence about current code.
2. From the repository root, run:

   ```sh
   python3 .agents/skills/verify-projection-batches/scripts/verify.py
   ```

   Requires Python 3.9+, Go matching `go.mod`, and initial HTTPS access to GitHub release assets. The runner downloads checksum-pinned TLC 1.7.4 and Temurin JRE 21.0.13+9 into `~/.cache/packy-formal-verification`; it supports macOS/Linux on arm64/amd64. Later runs use the cache. Tool pins live in `scripts/tools.json`. Runtime execution uses sandboxed `HOME`, `XDG_CONFIG_HOME`, and `TMPDIR`, preserving Go caches. Evidence stays in a new scratch directory outside the repository, printed first; `--out` can select a new external directory.
3. Read `summary.json`, `model/tlc.out`, and `replay.out` in that evidence directory. Exit 0 means both checks pass, 1 means an invariant violation, and 2 means infrastructure did not complete. A parser/build/download error is incomplete verification. A Go replay failure is a concrete counterexample; a TLC-only violation needs replay or a model correction.
4. Read **every** model result with `$statechart` (the pstack `pstack:statechart` skill), following its currently installed instructions. Each model directory contains scratch copies of `ProjectionBatch.tla`, its `.cfg`, and `view.mjs`; `summary.json` supplies the Java and TLC paths. Use those scratch inputs, keep chart output outside the repository, and present the shortest failing path in Packy terms. A pass should also be interpreted with `$statechart` and its state chart.
5. For a violation, match the first changed target and later error to the actual `ApplyFilesystemChanges` branches. Classify it as an implementation finding only after the real replay confirms it. Report any failures and keep the invariant intact.

The [baseline record](references/baseline.md) records the historical finding and mutation evidence. Read it when comparing a new result with the repaired baseline.

## Coverage and correspondence

Last synchronized against product code at **`a1bdb3c6e4ac295ab0d146e6fcb8b3dec14329ab`**.

| Code in `internal/localprojection/localprojection.go` | Model/replay responsibility |
| --- | --- |
| `Executor.Apply` | Carries action preconditions into `FilesystemChange`; adapter before the modeled transaction |
| `ApplyFilesystemChanges`, staging loop | All payloads validated and `hadTarget` captured before the modeled initial state |
| `filesystemChangeTargetFingerprint`, commit-loop comparison | `FingerprintMatches` or `FingerprintChanged` followed by prefix rollback; stale mask in the Go replay |
| `renameTreePath(target, backup)` | `BackupTarget` or `BackupFailed`; one rename is one indivisible step |
| Deletion branch and `renameTreePath(stage, target)` | `CommitDeletion`, `PublishTarget`, or `PublishFailed` |
| Current-item backup restoration | `RestoreCurrent`; the current item is outside `items[:committed]` |
| `rollbackFilesystemChanges` | `UndoRemove`, `UndoRestore`, then `ReturnFailure`; reverse commit order |
| `succeeded` deferred cleanup | `CleanupBackup` on success; failed batches retain backups |
| `internal/localprojection/formal_batch_test.go` | Replays all 320 bounded input/fault selections against the actual implementation |

`git diff a1bdb3c6e4ac295ab0d146e6fcb8b3dec14329ab -- internal/localprojection/localprojection.go` shows changes since synchronization. Move the sync commit forward only after checking **every covered function** and updating both the model and replay to describe the new code. A code change alone does not synchronize a hand-written model.

## Bounds and assumptions

- Exactly two distinct targets in one batch; each starts absent or as a regular file and requests replacement or deletion.
- All four initial-presence masks, four operation masks, four stale-precondition masks, and five fault selections: none, backup failure at target 1/2, publication failure at target 1/2. A fault at an unreachable operation has no effect.
- Staging has succeeded. Fingerprint mismatch is a separate check from the following rename. Each modeled rename is atomic, and a failed rename leaves paths unchanged.
- At most one injected forward rename failure. Rollback and cleanup operations succeed. No other writer changes targets, stages, backups, or parent directories during the transaction.
- Values abstract whole-file content to absent/original/replacement; the replay checks actual bytes and existence with distinct contents for each target.

This is bounded model checking plus real-code replay, **not a proof of all Go executions**. It excludes process crashes, durability/power loss, errors during rollback, concurrent writers, symlink/tree payload internals, permission preservation, and batches larger than two. A passing two-target bound does not prove larger batches safe. Fingerprint read errors and rollback failures are covered by ordinary focused tests, not this model.

## Check that the guard can fail

After changing the verification, run:

```sh
python3 .agents/skills/verify-projection-batches/scripts/verify.py --self-test
```

The control first checks fresh preconditions only, isolating backup failure from fingerprint rejection. The unmodified control must pass in TLC and in the exact Go subtest `present-3_replace-3_stale-0_fault-backup-2`. The runner then uses a scratch Go overlay to replace `items[:committed]` with `items[:0]` **only in the backup-error rollback branch**, and a corresponding scratch model setting skips that rollback. Both must detect the newly broken property. It drops the overlay, reruns the unmodified control, and finally reruns the complete baseline check. Product source is never edited.

Treat a missed mutant or a failing control as a broken verification. Repair the model/replay or mutation harness before relying on the check. Intended product changes require rechecking correspondence and bounds; the optional `$maintain-formal-verification-skill` route can bring an out-of-sync verification back in line.
