package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	qualificationBytes, err := readCanonicalRegular(qualificationPath, &qualification)
	if err != nil {
		return fmt.Errorf("read qualification: %w", err)
	}
	if err := claudesmoke.ValidateProductionAddyQualification(qualification); err != nil {
		return fmt.Errorf("validate production qualification: %w", err)
	}
	var evaluation governancedrift.Evaluation
	evaluationBytes, err := readCanonicalRegular(evaluationPath, &evaluation)
	if err != nil {
		return fmt.Errorf("read governance evaluation: %w", err)
	}
	var gate governancedrift.GateDecision
	gateBytes, err := readCanonicalRegular(gatePath, &gate)
	if err != nil {
		return fmt.Errorf("read governance gate: %w", err)
	}
	workflowSHABytes, err := gitOutputBytes("rev-parse", context.EvaluatedMergeSHA+":"+context.Workflow)
	if err != nil {
		return fmt.Errorf("resolve evaluated workflow blob: %w", err)
	}
	workflowSHA := string(bytes.TrimSpace(workflowSHABytes))
	if err := validateGenerationBindings(context, qualification, evaluation, gate, workflowSHA); err != nil {
		return err
	}

	acceptanceBytes, err := readNonemptyRegular(acceptancePath)
	if err != nil {
		return fmt.Errorf("read acceptance log: %w", err)
	}
	acceptanceSHA := sha256Hex(acceptanceBytes)
	qualificationSHA := sha256Hex(qualificationBytes)
	governanceSHA := sha256Hex(append(append([]byte(nil), evaluationBytes...), gateBytes...))
	prepublicationSHA := sha256Hex([]byte(acceptanceSHA + governanceSHA))
	authority, err := addyacceptance.NewProductionPromotionAuthority(context, acceptanceSHA, qualificationSHA, governanceSHA, prepublicationSHA)
	if err != nil {
		return err
	}

	root, err := os.MkdirTemp("", "packy-addy-production-harness.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	report, err := (addyacceptance.PromotionHarness{
		Root: root, Context: context, Mode: addyacceptance.PromotionHarnessExactCandidate,
		Evaluate: addyacceptance.ProductionPromotionRowEvaluator(authority),
	}).Run()
	if err != nil {
		return err
	}
	claudeIdentities := []string{
		"version:" + qualification.Smoke.ResolvedClaudeVersion,
		"npm-integrity:" + qualification.Smoke.ClaudeIntegrity,
		"executable-sha256:" + qualification.Smoke.ClaudeDigest,
	}
	sort.Strings(claudeIdentities)
	atomicityMaterial, err := json.Marshal(struct {
		Commands    any `json:"commands"`
		Before      any `json:"before"`
		After       any `json:"after"`
		Safety      any `json:"safety"`
		Observation any `json:"qualification_observation"`
	}{qualification.Smoke.Commands, qualification.Smoke.Before, qualification.Smoke.After, qualification.Smoke.Safety, qualification.Smoke.Qualification})
	if err != nil {
		return err
	}
	evidence, err := report.BuildAggregate(context, addyacceptance.PromotionAggregateCandidate{
		PackageCandidate: qualification.PackyExecutableDigest,
		ClaudeIdentities: claudeIdentities,
		AtomicitySHA256:  sha256Hex(atomicityMaterial),
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

func validateGenerationBindings(context addyacceptance.PromotionValidationContext, qualification claudesmoke.AddyQualification, evaluation governancedrift.Evaluation, gate governancedrift.GateDecision, workflowSHA string) error {
	if qualification.Repository != context.Repository || qualification.Workflow != context.Workflow ||
		qualification.WorkflowDigest != context.WorkflowDigest || qualification.RunID != context.RunID ||
		qualification.Commit != context.EvaluatedMergeSHA || qualification.Smoke.PackySHA != context.EvaluatedMergeSHA ||
		qualification.Smoke.RequestedClaudeVersion != claudesmoke.ExactFloor ||
		qualification.Smoke.ResolvedClaudeVersion != claudesmoke.ExactFloor {
		return errors.New("qualification does not match the exact trusted workflow run and evaluated merge")
	}
	if evaluation.State != governancedrift.StateClean || len(evaluation.Findings) != 0 || !gate.Allowed || len(gate.Reasons) != 0 {
		return errors.New("governance evidence is dirty or gate decision is not allowed")
	}
	i := evaluation.Identity
	if i.Repository != context.Repository || i.Ref != "refs/heads/main" ||
		i.CommitSHA != context.EvaluatedMergeSHA || i.WorkflowSHA != workflowSHA {
		return errors.New("governance evidence does not match repository, protected ref, evaluated merge, and workflow blob")
	}
	if i.CollectedAt.After(context.Now) || context.Now.Sub(i.CollectedAt) > time.Hour {
		return errors.New("governance evidence is stale or future-dated")
	}
	return nil
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
