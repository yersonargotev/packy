package vercelacceptance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAcceptanceRegistryOrderAndCopySafety(t *testing.T) {
	rows := Rows()
	if len(rows) != 24 {
		t.Fatalf("rows = %d, want 24", len(rows))
	}
	for i, row := range rows {
		if row.ID != "VERCEL-ACCEPTANCE-"+twoDigits(i+1) || row.Gate != Gate(i/4+1) {
			t.Fatalf("row %d = %#v", i, row)
		}
		if row.Name == "" || row.EvidenceSeam == "" || row.NegativeSeam == "" || row.OracleSeam == "" ||
			row.NegativeFact == "" || row.Oracle == "" {
			t.Fatalf("row %d lacks auditable semantics: %#v", i, row)
		}
		wantSource := EvidenceFoundation
		if i >= 16 && i <= 18 {
			wantSource = EvidenceHost
		}
		if row.Source != wantSource {
			t.Fatalf("row %d source = %d, want %d", i, row.Source, wantSource)
		}
	}
	if len(FoundationRows()) != 21 || len(HostRows()) != 3 {
		t.Fatalf("source partition = %d foundation, %d host", len(FoundationRows()), len(HostRows()))
	}
	if rows[16].Name != "codex-exact-host-readiness" || rows[16].Oracle != "nine skills and 28 modes" {
		t.Fatalf("representative readiness row = %#v", rows[16])
	}
	rows[0].ID = "changed"
	if Rows()[0].ID == "changed" {
		t.Fatal("Rows returned shared storage")
	}
}

func TestLifecycleRowsUseUniqueSurfaceOwnedWriteBoundaryProofs(t *testing.T) {
	want := map[string]string{
		"VERCEL-ACCEPTANCE-09": "./internal/codex/TestVercelLifecycleExercisesEveryCodexWriteBoundaryAndExactDiff",
		"VERCEL-ACCEPTANCE-10": "./internal/opencode/TestVercelLifecycleExercisesEveryOpenCodeWriteBoundaryAndExactDiff",
		"VERCEL-ACCEPTANCE-11": "./internal/claudecode/TestVercelLifecycleExercisesEveryClaudeWriteBoundaryAndExactDiff",
	}
	seen := map[string]bool{}
	for _, row := range Rows() {
		seam, ok := want[row.ID]
		if !ok {
			continue
		}
		if row.NegativeSeam != seam || row.OracleSeam != seam {
			t.Fatalf("%s lifecycle seams = %q / %q, want %q", row.ID, row.NegativeSeam, row.OracleSeam, seam)
		}
		if seen[seam] {
			t.Fatalf("lifecycle seam reused: %s", seam)
		}
		seen[seam] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("surface lifecycle seams = %v, want %v", seen, want)
	}
}

func TestCanonicalCohortReportAndDeterministicRerun(t *testing.T) {
	ctx, evidence := canonicalCohort(t)
	first, err := Evaluate(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(ctx, evidence)
	if err != nil || first.Fingerprint != second.Fingerprint {
		t.Fatalf("rerun = %q, %v; want %q", second.Fingerprint, err, first.Fingerprint)
	}
	a, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(a, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Rows) != 24 || decoded.Rows[23].Status != "passed" {
		t.Fatalf("report = %#v", decoded)
	}
	for i, row := range decoded.Rows {
		if row.Evidence == nil || row.Evidence.EvidenceSHA256 != evidence[i].EvidenceSHA256 || row.Evidence.EvidenceFingerprint != evidence[i].EvidenceFingerprint {
			t.Fatalf("row %d omitted accepted evidence: %#v", i, row)
		}
	}
	tampered := first
	tampered.Rows = append([]RowResult(nil), first.Rows...)
	copyEvidence := *tampered.Rows[0].Evidence
	copyEvidence.EvidenceSHA256 = strings.Repeat("c", 64)
	tampered.Rows[0].Evidence = &copyEvidence
	if _, err := tampered.CanonicalJSON(); err == nil {
		t.Fatal("canonical report accepted tampered owning evidence")
	}
}

func TestCohortRejectsMissingDuplicateMixedStaleTamperedAndFailedEvidence(t *testing.T) {
	ctx, good := canonicalCohort(t)
	tests := []struct {
		name string
		edit func([]RowEvidence) []RowEvidence
		want string
	}{
		{"missing", func(e []RowEvidence) []RowEvidence { return e[1:] }, "missing evidence"},
		{"duplicate", func(e []RowEvidence) []RowEvidence { return append(e, e[0]) }, "duplicate row"},
		{"mixed", func(e []RowEvidence) []RowEvidence { e[0].CandidateSHA = "other"; sealRows(e); return e }, "mixed candidate"},
		{"stale", func(e []RowEvidence) []RowEvidence {
			e[0].ObservedAt = ctx.Now.Add(-2 * ctx.MaxAge)
			sealRows(e)
			return e
		}, "stale evidence"},
		{"tampered", func(e []RowEvidence) []RowEvidence { e[0].EvidenceFingerprint = "tampered"; return e }, "tampered evidence fingerprint"},
		{"failed", func(e []RowEvidence) []RowEvidence { e[0].Passed = false; sealRows(e); return e }, "failed or incomplete"},
		{"unbound", func(e []RowEvidence) []RowEvidence { e[0].EvidenceSHA256 = ""; sealRows(e); return e }, "invalid owning evidence digest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := append([]RowEvidence(nil), good...)
			_, err := Evaluate(ctx, tt.edit(e))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCohortSuppressesEveryRowAfterFirstFailure(t *testing.T) {
	ctx, evidence := canonicalCohort(t)
	evidence[5].Passed = false
	evidence[5].EvidenceFingerprint = FingerprintRowEvidence(evidence[5])
	report, err := Evaluate(ctx, evidence)
	if err == nil {
		t.Fatal("expected failure")
	}
	for i, row := range report.Rows {
		want := "passed"
		if i == 5 {
			want = "failed"
		} else if i > 5 {
			want = "suppressed"
		}
		if row.Status != want {
			t.Fatalf("row %d status = %s, want %s", i, row.Status, want)
		}
	}
}

func canonicalCohort(t *testing.T) (CohortContext, []RowEvidence) {
	t.Helper()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	ctx := CohortContext{CandidateSHA: strings.Repeat("a", 40), FixtureSHA256: ExactArchiveSHA256, RunID: "run-262", Now: now, MaxAge: time.Hour}
	var evidence []RowEvidence
	for _, row := range Rows() {
		evidence = append(evidence, RowEvidence{RowID: row.ID, CandidateSHA: ctx.CandidateSHA, FixtureSHA256: ctx.FixtureSHA256, RunID: ctx.RunID, ObservedAt: now.Add(-time.Minute), Passed: true, NegativeTwin: true, Deterministic: true, ZeroMutation: true, EvidenceSHA256: strings.Repeat("b", 64)})
	}
	sealRows(evidence)
	return ctx, evidence
}

func sealRows(evidence []RowEvidence) {
	for i := range evidence {
		evidence[i].EvidenceFingerprint = FingerprintRowEvidence(evidence[i])
	}
}

func twoDigits(n int) string {
	return string([]byte{'0' + byte(n/10), '0' + byte(n%10)})
}
