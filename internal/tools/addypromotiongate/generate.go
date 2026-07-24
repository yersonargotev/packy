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
	atomicityMaterial := struct {
		Commands    any `json:"commands"`
		Before      any `json:"before"`
		After       any `json:"after"`
		Safety      any `json:"safety"`
		Observation any `json:"qualification_observation"`
	}{qualification.Smoke.Commands, qualification.Smoke.Before, qualification.Smoke.After, qualification.Smoke.Safety, qualification.Smoke.Qualification}

	root, err := os.MkdirTemp("", "packy-addy-production-harness.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	evidence, err := addyacceptance.BuildProductionPromotionEvidence(context, addyacceptance.ProductionPromotionInputs{
		Acceptance: acceptance,
		Qualification: addyacceptance.ProductionQualification{
			Synthetic: qualification.Synthetic, Repository: qualification.Repository,
			Workflow: qualification.Workflow, WorkflowDigest: qualification.WorkflowDigest,
			RunID: qualification.RunID, Commit: qualification.Commit, CollectedAt: collectedAt,
			PackySHA: qualification.Smoke.PackySHA, PackyExecutableDigest: qualification.PackyExecutableDigest,
			RequestedClaudeVersion: qualification.Smoke.RequestedClaudeVersion,
			ResolvedClaudeVersion:  qualification.Smoke.ResolvedClaudeVersion,
			ClaudeIntegrity:        qualification.Smoke.ClaudeIntegrity, ClaudeDigest: qualification.Smoke.ClaudeDigest,
			AtomicityMaterial: atomicityMaterial,
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
