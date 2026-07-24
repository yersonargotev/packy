package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/claudesmoke"
	"github.com/yersonargotev/packy/internal/governancedrift"
)

func TestReadCanonicalRegularRejectsMalformedNoncanonicalAndNonregularInputs(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"malformed":    "{",
		"noncanonical": `{"allowed":true,"reasons":[]}`,
		"trailing":     "{\n  \"allowed\": true,\n  \"reasons\": []\n}\n{}",
		"empty":        "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var decision governancedrift.GateDecision
			if _, err := readCanonicalRegular(path, &decision); err == nil {
				t.Fatal("untrusted input accepted")
			}
		})
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	var decision governancedrift.GateDecision
	if _, err := readCanonicalRegular(directory, &decision); err == nil {
		t.Fatal("nonregular input accepted")
	}
}

func TestValidateGenerationBindingsRejectsCrossRunCrossCommitAndGovernanceFailures(t *testing.T) {
	commit, workflowSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	context := addyacceptance.PromotionValidationContext{
		Repository: "owner/repo", Workflow: ".github/workflows/ci.yml",
		WorkflowDigest: strings.Repeat("c", 64), RunID: "7", EvaluatedMergeSHA: commit,
		Now: time.Date(2026, 7, 24, 14, 0, 0, 0, time.UTC),
	}
	qualification := claudesmoke.AddyQualification{
		Repository: context.Repository, Workflow: context.Workflow, WorkflowDigest: context.WorkflowDigest,
		RunID: context.RunID, Commit: commit, Smoke: claudesmoke.Evidence{
			PackySHA: commit, RequestedClaudeVersion: claudesmoke.ExactFloor, ResolvedClaudeVersion: claudesmoke.ExactFloor,
		},
	}
	evaluation := governancedrift.Evaluation{
		Identity: governancedrift.EvidenceIdentity{
			Repository: context.Repository, Ref: "refs/heads/main", CommitSHA: commit, WorkflowSHA: workflowSHA,
			CollectedAt: context.Now.Add(-time.Minute),
		},
		State: governancedrift.StateClean, Findings: []governancedrift.Finding{},
	}
	gate := governancedrift.GateDecision{Allowed: true, Reasons: []string{}}
	if err := validateGenerationBindings(context, qualification, evaluation, gate, workflowSHA); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*claudesmoke.AddyQualification, *governancedrift.Evaluation, *governancedrift.GateDecision)
	}{
		{"cross-run", func(q *claudesmoke.AddyQualification, _ *governancedrift.Evaluation, _ *governancedrift.GateDecision) {
			q.RunID = "8"
		}},
		{"cross-commit", func(q *claudesmoke.AddyQualification, _ *governancedrift.Evaluation, _ *governancedrift.GateDecision) {
			q.Commit = strings.Repeat("d", 40)
		}},
		{"dirty-governance", func(_ *claudesmoke.AddyQualification, e *governancedrift.Evaluation, _ *governancedrift.GateDecision) {
			e.State = governancedrift.StateConfirmedDrift
		}},
		{"stale-governance", func(_ *claudesmoke.AddyQualification, e *governancedrift.Evaluation, _ *governancedrift.GateDecision) {
			e.Identity.CollectedAt = context.Now.Add(-2 * time.Hour)
		}},
		{"disallowed-gate", func(_ *claudesmoke.AddyQualification, _ *governancedrift.Evaluation, g *governancedrift.GateDecision) {
			g.Allowed = false
			g.Reasons = []string{"blocked"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q, e, g := qualification, evaluation, gate
			test.mutate(&q, &e, &g)
			if err := validateGenerationBindings(context, q, e, g, workflowSHA); err == nil {
				t.Fatal("mismatched production authority accepted")
			}
		})
	}
}

func TestWriteExclusiveRequiresNewOutOfTreePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(filepath.Join(cwd, "candidate.json"), []byte("{}\n")); err == nil {
		t.Fatal("in-tree output accepted")
	}
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := writeExclusive(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(path, []byte("{}\n")); err == nil {
		t.Fatal("existing output replaced")
	}
}
