# Projection lifecycle and rollback

Users install, update, and remove Pack projections while Packy checks their ownership and content. A rejected batch restores earlier committed targets when rollback operations succeed.

## How to get to it

Use the [local Catalog candidate workflow](../../../../docs/catalog-project.md#exercise-local-candidates) with reviewed baseline and updated snapshot assets and a new disposable workspace. It initializes the workspace project and isolates home and configuration state.

## Drive the journey

- Preview and install a Pack with multiple projected resources using `catalog candidate`. Give each Apply its normal approval in a private PTY. Record the transcript and exit status.
- Inspect projected bytes, paths, project intent, and receipts after installation. Run candidate `verify` and retain its result.
- Change a receipt-owned target and attempt the affected lifecycle operation. Capture the rejection and compare all projection bytes and state with the pre-attempt snapshot. Restore the fixture before continuing.
- Transition the workspace to updated assets using the baseline archive digest. Apply `update --project` in a PTY. Inspect changed and unchanged projection bytes and the updated receipt. Run `verify` again.
- Apply `uninstall` in a PTY. Check the removed owned paths, project intent, and receipts, and confirm unrelated files remain.
- For global projection changes, also apply `activate`, `update`, and `deactivate` in an isolated candidate workspace. Inspect home and configuration state after each operation.
- For staging, fingerprint, publication, or rollback changes, run [verify-projection-batches](../../verify-projection-batches/SKILL.md). Check its complete model and real-code replay, including the mutation self-test, and interpret every model result. Ordinary focused tests must cover fingerprint read and rollback failures outside the model.

A drift rejection before Apply does not exercise a later commit-time fingerprint rejection. The focused tests and formal replay supply that rejection coverage. A missing reviewed snapshot, consent transcript, or state comparison leaves the corresponding journey incomplete.
