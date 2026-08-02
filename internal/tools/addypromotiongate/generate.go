package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/claudesmoke"
	"github.com/yersonargotev/packy/internal/governancedrift"
)

func generatePromotionEvidence(context addyacceptance.PromotionValidationContext, qualificationPath, evaluationPath, gatePath, acceptancePath, outputPath string) error {
	if !context.PromotionChange || context.FoundationChange || outputPath == "" {
		return errors.New("generation requires a promotion change and output path")
	}
	var qualification claudesmoke.AddyQualification
	_, err := readCanonicalRegular(qualificationPath, &qualification)
	if err != nil {
		return fmt.Errorf("read qualification: %w", err)
	}
	if err := claudesmoke.ValidateProductionAddyQualification(qualification); err != nil {
		return fmt.Errorf("validate production qualification: %w", err)
	}
	var acceptance addyacceptance.AcceptanceRunReport
	_, err = readCanonicalRegular(acceptancePath, &acceptance)
	if err != nil {
		return fmt.Errorf("read acceptance report: %w", err)
	}
	var evaluation governancedrift.Evaluation
	_, err = readCanonicalRegular(evaluationPath, &evaluation)
	if err != nil {
		return fmt.Errorf("read governance evaluation: %w", err)
	}
	var gate governancedrift.GateDecision
	_, err = readCanonicalRegular(gatePath, &gate)
	if err != nil {
		return fmt.Errorf("read governance gate: %w", err)
	}
	workflowSHABytes, err := gitOutputBytes("rev-parse", context.EvaluatedMergeSHA+":"+context.Workflow)
	if err != nil {
		return fmt.Errorf("resolve evaluated workflow blob: %w", err)
	}
	workflowSHA := string(bytes.TrimSpace(workflowSHABytes))
	collectedAt, err := time.Parse(time.RFC3339Nano, qualification.CollectedAt)
	if err != nil {
		return fmt.Errorf("parse qualification collection time: %w", err)
	}
	root, err := os.MkdirTemp("", "packy-addy-production-harness.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	evidence, err := addyacceptance.BuildProductionPromotionEvidence(context, addyacceptance.ProductionPromotionInputs{
		Acceptance: acceptance,
		Qualification: addyacceptance.ProductionQualification{
			SchemaVersion: qualification.Smoke.SchemaVersion,
			Synthetic:     qualification.Synthetic, Repository: qualification.Repository,
			Workflow: qualification.Workflow, WorkflowDigest: qualification.WorkflowDigest,
			RunID: qualification.RunID, Commit: qualification.Commit, CollectedAt: collectedAt,
			PackySHA: qualification.Smoke.PackySHA, PackyExecutableDigest: qualification.PackyExecutableDigest,
			RequestedClaudeVersion: qualification.Smoke.RequestedClaudeVersion,
			ResolvedClaudeVersion:  qualification.Smoke.ResolvedClaudeVersion,
			ClaudeIntegrity:        qualification.Smoke.ClaudeIntegrity, ClaudeDigest: qualification.Smoke.ClaudeDigest,
			Sandbox: qualification.Sandbox, Atomicity: projectProductionAtomicity(qualification),
		},
		GovernanceEvaluation: evaluation, GovernanceDecision: gate,
		WorkflowBlobSHA: workflowSHA, DisposableHarnessRoot: root,
	})
	if err != nil {
		return err
	}
	data, err := evidence.CanonicalJSON()
	if err != nil {
		return err
	}
	return writeExclusive(outputPath, data)
}

func projectProductionAtomicity(q claudesmoke.AddyQualification) addyacceptance.ProductionAtomicity {
	commands := make([]addyacceptance.ProductionCommand, len(q.Smoke.Commands))
	for i, command := range q.Smoke.Commands {
		commands[i] = addyacceptance.ProductionCommand{Name: command.Name, Args: append([]string(nil), command.Args...), ExitCode: command.ExitCode, Stdout: command.Stdout, Stderr: command.Stderr}
	}
	projectFiles := func(files []claudesmoke.FileEvidence) []addyacceptance.ProductionFile {
		out := make([]addyacceptance.ProductionFile, len(files))
		for i, file := range files {
			out[i] = addyacceptance.ProductionFile{Path: file.Path, SHA256: file.SHA256, Mode: file.Mode, Size: file.Size}
		}
		return out
	}
	s, x, o := q.Smoke.Safety, q.Smoke.Assertions, q.Smoke.Qualification
	return addyacceptance.ProductionAtomicity{
		Commands: commands, Before: projectFiles(q.Smoke.Before), After: projectFiles(q.Smoke.After),
		Safety: addyacceptance.ProductionRunnerSafety{
			DisposableSandbox: s.DisposableSandbox, AllowlistEnvironment: s.AllowlistEnvironment,
			CredentialsScrubbed: s.CredentialsScrubbed, CommandAllowlist: s.CommandAllowlist,
			CheckoutUnchanged: s.CheckoutUnchanged, ConfiguredWritableRootsConfined: s.ConfiguredWritableRootsConfined,
			EvidencePathOutsideSandbox: s.EvidencePathOutsideSandbox, NoInteractiveClaude: s.NoInteractiveClaude,
			WriteBoundaryEnforced: s.WriteBoundaryEnforced,
		},
		Assertions: addyacceptance.ProductionAssertions{
			InstalledSourceInitialized: x.InstalledSourceInitialized, DoctorReportedCoreHealthy: x.DoctorReportedCoreHealthy,
			RemovedInstallRejected: x.RemovedInstallRejected, RemovedUpdateRejected: x.RemovedUpdateRejected,
			RemovedUninstallRejected: x.RemovedUninstallRejected, ClassicStatePreserved: x.ClassicStatePreserved,
			ClaudeInstructionPreserved: x.ClaudeInstructionPreserved, ClaudeMCPPreserved: x.ClaudeMCPPreserved,
			SharedSkillSentinelPreserved:         x.SharedSkillSentinelPreserved,
			InitializationCausedNoSurfaceChange:  x.InitializationCausedNoSurfaceChange,
			ActivationPreviewCausedNoChange:      x.ActivationPreviewCausedNoChange,
			RepresentativePackActivated:          x.RepresentativePackActivated,
			ReadinessInspectedSeparately:         x.ReadinessInspectedSeparately,
			NoActivationStateAfterInitialization: x.NoActivationStateAfterInitialization,
			NoClaudeMutationOperations:           x.NoClaudeMutationOperations, EngramStubProtocolVerified: x.EngramStubProtocolVerified,
			SensitiveFixtureRedacted: x.SensitiveFixtureRedacted,
		},
		Observation: addyacceptance.ProductionObservation{
			InstalledSource: o.InstalledSource, InstalledSourceCommit: o.InstalledSourceCommit,
			InstalledSourceClean: o.InstalledSourceClean,
			WritableRoots: addyacceptance.ProductionWritableRoots{
				Home: o.WritableRoots.Home, XDGConfig: o.WritableRoots.XDGConfig, ClaudeConfig: o.WritableRoots.ClaudeConfig,
				State: o.WritableRoots.State, Package: o.WritableRoots.Package, Repository: o.WritableRoots.Repository, Acquisition: o.WritableRoots.Acquisition,
			},
			ProcessLogDigest: o.ProcessLogDigest, CollectedAt: mustParseProductionTime(o.CollectedAt),
			Safety: addyacceptance.ProductionObservedSafety{
				NoGoRun: o.Safety.NoGoRun, NoDevelopmentPath: o.Safety.NoDevelopmentPath, NoDirectFixture: o.Safety.NoDirectFixture,
				NoUntrackedInput: o.Safety.NoUntrackedInput, NoAuthentication: o.Safety.NoAuthentication,
				NoModelInvocation: o.Safety.NoModelInvocation, NoPrint: o.Safety.NoPrint, NoREPL: o.Safety.NoREPL,
				NoUpstreamExecute: o.Safety.NoUpstreamExecute, NoCredentials: o.Safety.NoCredentials, NoOutsideWrite: o.Safety.NoOutsideWrite,
			},
		},
	}
}

func mustParseProductionTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func readCanonicalRegular(path string, target any) ([]byte, error) {
	data, err := readNonemptyRegular(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("JSON contains trailing values")
	}
	canonical, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return nil, err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(data, canonical) {
		return nil, errors.New("JSON is not canonical")
	}
	return data, nil
}

func readNonemptyRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, errors.New("input must be a nonempty regular file")
	}
	return os.ReadFile(path)
}

func writeExclusive(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return errors.New("output must be an absolute out-of-tree path")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("output must be outside the repository checkout")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
