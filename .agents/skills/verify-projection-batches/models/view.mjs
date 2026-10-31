export const title = "A rejected Packy projection batch";
export const state = (v) => {
  if (v.phase === "failed") {
    const restored = Object.keys(v.target).every((key) => v.target[key] === v.prior[key]);
    return restored ? "Rejected; original targets restored" : "Rejected; earlier target still changed";
  }
  if (v.phase === "success") return "Batch committed";
  if (v.phase.startsWith("undo") || v.phase === "restore-current") return "Restoring the batch";
  if (v.phase === "cleanup") return "Removing backups after success";
  const step = {
    check: "Check target fingerprint",
    backup: "Back up original target",
    publish: "Publish replacement or deletion",
    advance: "Target committed",
  }[v.phase];
  return `${step} ${v.i}`;
};
export const step = (action) => ({
  FingerprintMatches: "fingerprint matches",
  FingerprintChanged: "fingerprint differs; roll back earlier targets",
  BackupTarget: "rename original to backup",
  SkipAbsentBackup: "target was absent",
  BackupFailed: "backup rename fails",
  PublishTarget: "rename staged replacement into place",
  CommitDeletion: "commit deletion",
  PublishFailed: "publication rename fails",
  RestoreCurrent: "restore current target",
  Advance: "continue batch",
  UndoRemove: "remove an earlier published target",
  UndoRestore: "restore its original backup",
  ReturnFailure: "return error after rollback",
  CleanupBackup: "delete backup",
  Done: null,
})[action] ?? action;
