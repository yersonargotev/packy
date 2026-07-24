package addyacceptance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAddyPromotionIndependentInputs(t *testing.T) {
	before, err := CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(before); got != "4f5e42be89e9c121c48abdbb77b1312d881f1207887119d50c252c316a1ff02f" || len(before) != 54507 {
		t.Fatalf("original Addy 1.0.0 oracle changed: sha256=%s bytes=%d", got, len(before))
	}
	first, second := CanonicalPromotionHistory(), CanonicalPromotionHistory()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("promotion history fixture changed between reconstructions")
	}
	if first.CatalogAdvertised || first.CurrentVersion != PackVersion {
		t.Fatalf("promotion fixture advertised Addy 1.1.0: %#v", first)
	}
	if len(first.Versions) != 2 || first.Versions[0].Version != "1.0.0" || first.Versions[1].Version != "1.1.0" {
		t.Fatalf("immutable versions = %#v", first.Versions)
	}
	if first.Versions[0].SnapshotSHA256 != first.Versions[1].SnapshotSHA256 {
		t.Fatal("synthetic 1.1.0 history changed selected Addy source bytes")
	}
	after, _ := CanonicalJSON()
	if !bytes.Equal(before, after) {
		t.Fatal("promotion reconstruction changed the original Addy 1.0.0 oracle")
	}

	context := applicablePromotionContext()
	evidence := validApplicablePromotionEvidence(context)
	evidence.Proof.BaseSHA256 = evidence.PackageCandidate
	evidence.Proof.HeadSHA256 = evidence.PackageCandidate
	evidence.Proof.HistorySHA256 = evidence.PackageCandidate
	evidence.Proof.DiffSHA256 = evidence.PackageCandidate
	if err := ValidatePromotionEvidence(evidence, context); err == nil || !strings.Contains(err.Error(), "independent reconstruction") {
		t.Fatalf("candidate self-authorization was accepted: %v", err)
	}
}

func TestAddyPromotionAuthorityFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{4, 5, 6} {
		row := rows[number-1]
		if row.Gate != 2 || row.OwningTest != "TestAddyPromotionAuthorityFoundations" {
			t.Fatalf("authority row %d = %#v", number, row)
		}
	}
}

func TestAddyPromotionLifecycleFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{7, 8, 9, 10} {
		row := rows[number-1]
		if row.Gate < 3 || row.Gate > 4 || row.OwningTest != "TestAddyPromotionLifecycleFoundations" {
			t.Fatalf("lifecycle row %d = %#v", number, row)
		}
	}
}

func TestAddyPromotionRealHostFoundations(t *testing.T) {
	rows := PromotionRows()
	for _, number := range []int{11, 12} {
		row := rows[number-1]
		if row.Gate != 5 || row.OwningTest != "TestAddyPromotionRealHostFoundations" {
			t.Fatalf("real-host row %d = %#v", number, row)
		}
	}
}

func TestAddyPromotionEvidenceFoundations(t *testing.T) {
	rows := PromotionRows()
	if len(rows) != 14 {
		t.Fatalf("promotion rows = %d, want 14", len(rows))
	}
	for i, row := range rows {
		wantID := "ADDY-CLAUDE-PROMOTION-ROW-" + twoDigits(i+1)
		if row.ID != wantID || row.Number != i+1 || row.BlockedDiagnostic != wantID+"-BLOCKED" {
			t.Fatalf("row %d = %#v", i+1, row)
		}
	}
	rows[0].ID = "mutated"
	if PromotionRows()[0].ID == "mutated" {
		t.Fatal("PromotionRows returned shared storage")
	}

	context := applicablePromotionContext()
	if err := ValidatePromotionEvidence(validApplicablePromotionEvidence(context), context); err != nil {
		t.Fatalf("valid promotion evidence rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*PromotionEvidence)
		want string
	}{
		{name: "missing row", edit: func(e *PromotionEvidence) { e.Rows = e.Rows[:13] }, want: "13 rows"},
		{name: "duplicate row", edit: func(e *PromotionEvidence) { e.Rows[13].ID = e.Rows[12].ID }, want: "duplicate row"},
		{name: "unknown row", edit: func(e *PromotionEvidence) { e.Rows[13].ID = "ADDY-CLAUDE-PROMOTION-ROW-99" }, want: "unknown row"},
		{name: "stale aggregate", edit: func(e *PromotionEvidence) { e.CollectedAt = e.CollectedAt.Add(-2 * time.Hour) }, want: "stale"},
		{name: "stale row", edit: func(e *PromotionEvidence) { e.Rows[0].CollectedAt = e.Rows[0].CollectedAt.Add(-2 * time.Hour) }, want: "stale"},
		{name: "cross commit", edit: func(e *PromotionEvidence) { e.Rows[0].CommitSHA = strings.Repeat("f", 40) }, want: "cross-commit"},
		{name: "cross workflow", edit: func(e *PromotionEvidence) { e.Rows[0].WorkflowDigest = strings.Repeat("e", 64) }, want: "cross-commit"},
		{name: "cross run", edit: func(e *PromotionEvidence) { e.Rows[0].RunID = "other-run" }, want: "cross-commit"},
		{name: "ambiguous result", edit: func(e *PromotionEvidence) { e.Rows[0].Result = "blocked" }, want: "ambiguous result"},
		{name: "ambiguous identity", edit: func(e *PromotionEvidence) { e.Tag = "v1.1.0" }, want: "identity does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := validApplicablePromotionEvidence(context)
			test.edit(&evidence)
			if err := ValidatePromotionEvidence(evidence, context); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAddyPromotionNotApplicableIsCanonicalAndFailsClosed(t *testing.T) {
	context := notApplicablePromotionContext()
	evidence := NewNotApplicablePromotionEvidence(context)
	first, err := evidence.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := evidence.CanonicalJSON()
	if !bytes.Equal(first, second) {
		t.Fatal("not_applicable encoding changed between reruns")
	}
	decoded, err := DecodePromotionEvidence(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePromotionEvidence(decoded, context); err != nil {
		t.Fatalf("canonical not_applicable rejected: %v", err)
	}
	if _, err := ValidateCanonicalPromotionEvidence(first, context); err != nil {
		t.Fatalf("canonical not_applicable bytes rejected: %v", err)
	}
	compact, _ := json.Marshal(decoded)
	if _, err := ValidateCanonicalPromotionEvidence(compact, context); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical aggregate bytes were accepted: %v", err)
	}
	if !bytes.Contains(first, []byte(`"disposition": "not_applicable"`)) {
		t.Fatalf("canonical output = %s", first)
	}

	var raw map[string]any
	if err := json.Unmarshal(first, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	unknown, _ := json.Marshal(raw)
	if _, err := DecodePromotionEvidence(unknown); err == nil {
		t.Fatal("unknown aggregate field was accepted")
	}

	context.PromotionChange = true
	if err := ValidatePromotionEvidence(decoded, context); err == nil || !strings.Contains(err.Error(), "cannot be not_applicable") {
		t.Fatalf("promotion change used not_applicable: %v", err)
	}
}

func applicablePromotionContext() PromotionValidationContext {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	return PromotionValidationContext{
		PromotionChange:   true,
		Repository:        "yersonargotev/packy",
		PullRequest:       201,
		BaseSHA:           strings.Repeat("a", 40),
		HeadSHA:           strings.Repeat("b", 40),
		EvaluatedMergeSHA: strings.Repeat("c", 40),
		Workflow:          PromotionCheckName,
		WorkflowDigest:    strings.Repeat("d", 64),
		MatrixVersion:     PromotionMatrixVersion,
		RunID:             "12345",
		Now:               now,
		MaxAge:            time.Hour,
		Inputs:            CanonicalIndependentPromotionInputs(),
	}
}

func notApplicablePromotionContext() PromotionValidationContext {
	context := applicablePromotionContext()
	context.PromotionChange = false
	context.EvaluatedMergeSHA = ""
	return context
}

func validApplicablePromotionEvidence(context PromotionValidationContext) PromotionEvidence {
	rows := make([]PromotionRowEvidence, 0, len(promotionRows))
	for i, definition := range promotionRows {
		rows = append(rows, PromotionRowEvidence{
			ID:             definition.ID,
			Result:         PromotionPassed,
			EvidenceSHA256: strings.Repeat(string("0123456789abcdef"[i%16]), 64),
			CommitSHA:      contextCommit(context),
			WorkflowDigest: context.WorkflowDigest,
			RunID:          context.RunID,
			CollectedAt:    context.Now.Add(-time.Minute),
		})
	}
	return PromotionEvidence{
		Schema:            PromotionEvidenceSchema,
		Disposition:       PromotionApplicable,
		Repository:        context.Repository,
		PullRequest:       context.PullRequest,
		BaseSHA:           context.BaseSHA,
		HeadSHA:           context.HeadSHA,
		EvaluatedMergeSHA: context.EvaluatedMergeSHA,
		Workflow:          context.Workflow,
		WorkflowDigest:    context.WorkflowDigest,
		MatrixVersion:     context.MatrixVersion,
		RunID:             context.RunID,
		CollectedAt:       context.Now.Add(-time.Minute),
		Rows:              rows,
		Proof:             PromotionProof{IndependentPromotionInputs: context.Inputs},
		PackageCandidate:  strings.Repeat("1", 64),
		ClaudeIdentities:  []string{"claude-code@2.1.203"},
		AtomicitySHA256:   strings.Repeat("2", 64),
	}
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}
