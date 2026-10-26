package cli

import "testing"

func TestIssue877RetiredSurfaceRejectsEveryLifecycleBeforeMutation(t *testing.T) {
	assertRetiredSurfaceRejectsEveryLifecycleBeforeMutation(t, "codex")
}

func TestIssue877OfflineContractsAndAuditRejectRetiredState(t *testing.T) {
	assertOfflineContractsAndAuditRejectRetiredState(t, "codex")
}

func TestIssue877TUIRejectsRetiredSurfaceBeforeObservation(t *testing.T) {
	assertTUIRejectsRetiredSurfaceBeforeObservation(t, "codex")
}
