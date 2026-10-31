------------------------- MODULE ProjectionBatch -------------------------
EXTENDS Naturals, FiniteSets

CONSTANTS N, AllowStale, RollbackOnBackupFailure
Slots == 1..N

VARIABLES prior, replacements, fresh, fault, faultSlot,
          target, backup, i, undo, phase, reason
vars == <<prior, replacements, fresh, fault, faultSlot,
          target, backup, i, undo, phase, reason>>
inputs == <<prior, replacements, fresh, fault, faultSlot>>

\* 0 = absent, 1 = pre-batch bytes, 2 = staged replacement bytes.
\* Start after successful staging; hadTarget is (prior[i] # 0).
Init ==
    /\ prior \in [Slots -> {0, 1}]
    /\ replacements \in SUBSET Slots
    /\ fresh \in SUBSET Slots
    /\ (~AllowStale => fresh = Slots)
    /\ fault \in {"none", "backup", "publish"}
    /\ faultSlot \in Slots
    /\ (fault = "none" => faultSlot = 1)
    /\ target = prior
    /\ backup = [s \in Slots |-> 0]
    /\ i = 1
    /\ undo = 0
    /\ phase = "check"
    /\ reason = ""

\* ApplyFilesystemChanges: filesystemChangeTargetFingerprint and comparison.
FingerprintMatches ==
    /\ phase = "check" /\ i \in fresh
    /\ phase' = "backup"
    /\ UNCHANGED <<inputs, target, backup, i, undo, reason>>

FingerprintChanged ==
    /\ phase = "check" /\ i \notin fresh
    /\ phase' = "undo-remove" /\ undo' = i - 1 /\ reason' = "fingerprint"
    /\ UNCHANGED <<inputs, target, backup, i>>

\* One successful os.Rename(target, backup) is one indivisible action.
BackupTarget ==
    /\ phase = "backup" /\ prior[i] # 0
    /\ ~(fault = "backup" /\ faultSlot = i)
    /\ backup' = [backup EXCEPT ![i] = target[i]]
    /\ target' = [target EXCEPT ![i] = 0]
    /\ phase' = "publish"
    /\ UNCHANGED <<inputs, i, undo, reason>>

SkipAbsentBackup ==
    /\ phase = "backup" /\ prior[i] = 0
    /\ phase' = "publish"
    /\ UNCHANGED <<inputs, target, backup, i, undo, reason>>

BackupFailed ==
    /\ phase = "backup" /\ prior[i] # 0
    /\ fault = "backup" /\ faultSlot = i
    /\ phase' = IF RollbackOnBackupFailure THEN "undo-remove" ELSE "failed"
    /\ undo' = i - 1 /\ reason' = "backup"
    /\ UNCHANGED <<inputs, target, backup, i>>

\* One os.Rename(stage, target), after the target was backed up if present.
PublishTarget ==
    /\ phase = "publish" /\ i \in replacements
    /\ ~(fault = "publish" /\ faultSlot = i)
    /\ target' = [target EXCEPT ![i] = 2]
    /\ phase' = "advance"
    /\ UNCHANGED <<inputs, backup, i, undo, reason>>

CommitDeletion ==
    /\ phase = "publish" /\ i \notin replacements
    /\ phase' = "advance"
    /\ UNCHANGED <<inputs, target, backup, i, undo, reason>>

PublishFailed ==
    /\ phase = "publish" /\ i \in replacements
    /\ fault = "publish" /\ faultSlot = i
    /\ phase' = "restore-current" /\ reason' = "publish"
    /\ UNCHANGED <<inputs, target, backup, i, undo>>

\* renameTreePath(backup, target) for the current unpublished item.
RestoreCurrent ==
    /\ phase = "restore-current"
    /\ target' = [target EXCEPT ![i] = backup[i]]
    /\ backup' = [backup EXCEPT ![i] = 0]
    /\ undo' = i - 1 /\ phase' = "undo-remove"
    /\ UNCHANGED <<inputs, i, reason>>

Advance ==
    /\ phase = "advance"
    /\ IF i = N
          THEN /\ phase' = "cleanup" /\ i' = 1
          ELSE /\ phase' = "check" /\ i' = i + 1
    /\ UNCHANGED <<inputs, target, backup, undo, reason>>

\* rollbackFilesystemChanges: remove the published target, then restore backup.
UndoRemove ==
    /\ phase = "undo-remove" /\ undo > 0
    /\ target' = [target EXCEPT ![undo] = 0]
    /\ phase' = "undo-restore"
    /\ UNCHANGED <<inputs, backup, i, undo, reason>>

UndoRestore ==
    /\ phase = "undo-restore"
    /\ target' = [target EXCEPT ![undo] = backup[undo]]
    /\ backup' = [backup EXCEPT ![undo] = 0]
    /\ undo' = undo - 1 /\ phase' = "undo-remove"
    /\ UNCHANGED <<inputs, i, reason>>

ReturnFailure ==
    /\ phase = "undo-remove" /\ undo = 0
    /\ phase' = "failed"
    /\ UNCHANGED <<inputs, target, backup, i, undo, reason>>

\* Deferred cleanup after succeeded=true removes each retained backup.
CleanupBackup ==
    /\ phase = "cleanup"
    /\ backup' = [backup EXCEPT ![i] = 0]
    /\ IF i = N
          THEN /\ phase' = "success" /\ i' = i
          ELSE /\ phase' = "cleanup" /\ i' = i + 1
    /\ UNCHANGED <<inputs, target, undo, reason>>

Done == phase \in {"failed", "success"} /\ UNCHANGED vars

Next == FingerprintMatches \/ FingerprintChanged \/ BackupTarget
        \/ SkipAbsentBackup \/ BackupFailed \/ PublishTarget \/ CommitDeletion
        \/ PublishFailed \/ RestoreCurrent \/ Advance \/ UndoRemove
        \/ UndoRestore \/ ReturnFailure \/ CleanupBackup \/ Done

Spec == Init /\ [][Next]_vars

TypeOK ==
    /\ prior \in [Slots -> {0, 1}]
    /\ target \in [Slots -> {0, 1, 2}]
    /\ backup \in [Slots -> {0, 1}]
    /\ i \in Slots /\ undo \in 0..N

RejectedBatchRestoresTargets == phase = "failed" => target = prior
=============================================================================
