package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/claudesmoke"
	"github.com/yersonargotev/packy/internal/governancedrift"
)

func TestProjectProductionAtomicityMapsCoreCutoverEvidenceExactly(t *testing.T) {
	wantAssertions := addyacceptance.ProductionAssertions{
		InstalledSourceInitialized: true, DoctorReportedCoreHealthy: true,
		RemovedInstallRejected: true, RemovedUpdateRejected: true, RemovedUninstallRejected: true,
		ClassicStatePreserved: true, ClaudeInstructionPreserved: true, ClaudeMCPPreserved: true,
		SharedSkillSentinelPreserved: true, InitializationCausedNoSurfaceChange: true,
		ActivationPreviewCausedNoChange: true, RepresentativePackActivated: true,
		ReadinessInspectedSeparately: true, NoActivationStateAfterInitialization: true,
		NoClaudeMutationOperations: true,
		EngramStubProtocolVerified: true, SensitiveFixtureRedacted: true,
	}
	q := claudesmoke.AddyQualification{Smoke: claudesmoke.Evidence{
		SchemaVersion: 3,
		Commands: []claudesmoke.CommandEvidence{
			{Name: "claude", Args: []string{"--version"}},
			{Name: "packy", Args: []string{"install"}, ExitCode: 1, Stderr: "unknown command"},
			{Name: "claude", Args: []string{"version"}},
		},
		Before: []claudesmoke.FileEvidence{{Path: "before", SHA256: "before-digest", Mode: 0o600, Size: 1}},
		After:  []claudesmoke.FileEvidence{{Path: "after", SHA256: "after-digest", Mode: 0o600, Size: 2}},
		Assertions: claudesmoke.AssertionEvidence{
			InstalledSourceInitialized: true, DoctorReportedCoreHealthy: true,
			RemovedInstallRejected: true, RemovedUpdateRejected: true, RemovedUninstallRejected: true,
			ClassicStatePreserved: true, ClaudeInstructionPreserved: true, ClaudeMCPPreserved: true,
			SharedSkillSentinelPreserved: true, InitializationCausedNoSurfaceChange: true,
			ActivationPreviewCausedNoChange: true, RepresentativePackActivated: true,
			ReadinessInspectedSeparately: true, NoActivationStateAfterInitialization: true,
			NoClaudeMutationOperations: true,
			EngramStubProtocolVerified: true, SensitiveFixtureRedacted: true,
		},
		Qualification: claudesmoke.AddyQualificationObservation{CollectedAt: "2026-08-01T12:00:00Z"},
	}}

	got := projectProductionAtomicity(q)
	if !reflect.DeepEqual(got.Assertions, wantAssertions) {
		t.Fatalf("assertion projection mismatch: %#v", got.Assertions)
	}
	if !reflect.DeepEqual(got.Commands[1], addyacceptance.ProductionCommand{Name: "packy", Args: []string{"install"}, ExitCode: 1, Stderr: "unknown command"}) {
		t.Fatalf("command projection mismatch: %#v", got.Commands[1])
	}
	if got.Before[0].Path != "before" || got.After[0].Path != "after" || !got.Observation.CollectedAt.Equal(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("manifest or observation projection mismatch: %#v", got)
	}
}

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
