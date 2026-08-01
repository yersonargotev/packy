package addyacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

const AcceptanceRunReportSchema = "addy-acceptance-run.v1"

type AcceptanceRunRow struct {
	ID             string `json:"id"`
	Package        string `json:"package"`
	OwningTest     string `json:"owning_test"`
	Result         string `json:"result"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type AcceptanceRunReport struct {
	Schema         string             `json:"schema"`
	Synthetic      bool               `json:"synthetic"`
	Repository     string             `json:"repository"`
	CommitSHA      string             `json:"commit_sha"`
	WorkflowDigest string             `json:"workflow_digest"`
	RunID          string             `json:"run_id"`
	Qualified      bool               `json:"qualified"`
	Rows           []AcceptanceRunRow `json:"rows"`
}

func (r AcceptanceRunReport) Validate(context PromotionValidationContext) error {
	if r.Schema != AcceptanceRunReportSchema || r.Synthetic || !r.Qualified {
		return errors.New("acceptance report must be a qualified non-synthetic production run")
	}
	if r.Repository != context.Repository || r.CommitSHA != contextCommit(context) ||
		r.WorkflowDigest != context.WorkflowDigest || r.RunID != context.RunID {
		return errors.New("acceptance report does not match trusted evaluated candidate")
	}
	rows := PromotionRows()
	if len(r.Rows) != len(rows) {
		return errors.New("acceptance report must contain every promotion row exactly once")
	}
	seen := map[string]bool{}
	for i, got := range r.Rows {
		want := rows[i]
		if seen[got.ID] || got.ID != want.ID || got.Package != "./internal/addyacceptance" ||
			got.OwningTest != want.OwningTest || got.Result != PromotionPassed ||
			!validAuthorityDigest(got.EvidenceSHA256) {
			return fmt.Errorf("acceptance report row %d has invalid identity, owner, result, or evidence", i+1)
		}
		seen[got.ID] = true
	}
	return nil
}

func (r AcceptanceRunReport) CanonicalJSON(context PromotionValidationContext) ([]byte, error) {
	if err := r.Validate(context); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type ProductionPromotionInputs struct {
	Acceptance            AcceptanceRunReport
	Qualification         ProductionQualification
	GovernanceEvaluation  governancedrift.Evaluation
	GovernanceDecision    governancedrift.GateDecision
	WorkflowBlobSHA       string
	DisposableHarnessRoot string
}

type ProductionQualification struct {
	SchemaVersion          int                 `json:"schema_version"`
	Synthetic              bool                `json:"synthetic"`
	Repository             string              `json:"repository"`
	Workflow               string              `json:"workflow"`
	WorkflowDigest         string              `json:"workflow_digest"`
	RunID                  string              `json:"run_id"`
	Commit                 string              `json:"commit"`
	CollectedAt            time.Time           `json:"collected_at"`
	PackySHA               string              `json:"packy_sha"`
	PackyExecutableDigest  string              `json:"packy_executable_sha256"`
	RequestedClaudeVersion string              `json:"requested_claude_version"`
	ResolvedClaudeVersion  string              `json:"resolved_claude_version"`
	ClaudeIntegrity        string              `json:"claude_npm_integrity"`
	ClaudeDigest           string              `json:"claude_executable_sha256"`
	Sandbox                string              `json:"sandbox"`
	Atomicity              ProductionAtomicity `json:"atomicity"`
}

type ProductionCommand struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}
type ProductionFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
}
type ProductionRunnerSafety struct {
	DisposableSandbox, AllowlistEnvironment, CredentialsScrubbed, CommandAllowlist bool
	CheckoutUnchanged, ConfiguredWritableRootsConfined, EvidencePathOutsideSandbox bool
	NoInteractiveClaude, WriteBoundaryEnforced                                     bool
}
type ProductionAssertions struct {
	InstalledSourceInitialized, DoctorReportedCoreHealthy                           bool
	RemovedInstallRejected, RemovedUpdateRejected, RemovedUninstallRejected         bool
	ClassicStatePreserved, ClaudeInstructionPreserved, ClaudeMCPPreserved           bool
	SharedSkillSentinelPreserved, NoPacksOwnershipState, NoClaudeMutationOperations bool
	EngramStubProtocolVerified, SensitiveFixtureRedacted                            bool
}
type ProductionWritableRoots struct {
	Home, XDGConfig, ClaudeConfig, State, Package, Repository, Acquisition string
}
type ProductionObservedSafety struct {
	NoGoRun, NoDevelopmentPath, NoDirectFixture, NoUntrackedInput           bool
	NoAuthentication, NoModelInvocation, NoPrint, NoREPL, NoUpstreamExecute bool
	NoCredentials, NoOutsideWrite                                           bool
}
type ProductionObservation struct {
	InstalledSource       string
	InstalledSourceCommit string
	InstalledSourceClean  bool
	WritableRoots         ProductionWritableRoots
	ProcessLogDigest      string
	CollectedAt           time.Time
	Safety                ProductionObservedSafety
}
type ProductionAtomicity struct {
	Commands    []ProductionCommand    `json:"commands"`
	Before      []ProductionFile       `json:"before"`
	After       []ProductionFile       `json:"after"`
	Safety      ProductionRunnerSafety `json:"safety"`
	Assertions  ProductionAssertions   `json:"assertions"`
	Observation ProductionObservation  `json:"qualification_observation"`
}

// BuildProductionPromotionEvidence owns admission policy for all independent
// production authorities and produces the exact aggregate evidence.
func BuildProductionPromotionEvidence(context PromotionValidationContext, in ProductionPromotionInputs) (PromotionEvidence, error) {
	if err := in.Acceptance.Validate(context); err != nil {
		return PromotionEvidence{}, err
	}
	q := in.Qualification
	if q.SchemaVersion != 2 || q.Synthetic || q.Repository != context.Repository || q.Workflow != context.Workflow ||
		q.WorkflowDigest != context.WorkflowDigest || q.RunID != context.RunID ||
		q.Commit != contextCommit(context) || q.PackySHA != contextCommit(context) ||
		q.RequestedClaudeVersion != "2.1.203" || q.ResolvedClaudeVersion != "2.1.203" ||
		!validAuthorityDigest(q.PackyExecutableDigest) || !validAuthorityDigest(q.ClaudeDigest) ||
		strings.TrimSpace(q.ClaudeIntegrity) == "" {
		return PromotionEvidence{}, errors.New("qualification does not match exact trusted run, commit, workflow, and Claude floor")
	}
	if q.CollectedAt.After(context.Now) || context.Now.Sub(q.CollectedAt) > 24*time.Hour {
		return PromotionEvidence{}, errors.New("qualification evidence is stale or future-dated")
	}
	if err := validateProductionAtomicity(q); err != nil {
		return PromotionEvidence{}, err
	}
	e, g := in.GovernanceEvaluation, in.GovernanceDecision
	if e.State != governancedrift.StateClean || len(e.Findings) != 0 || !g.Allowed || len(g.Reasons) != 0 {
		return PromotionEvidence{}, errors.New("governance evidence is dirty or gate decision is disallowed")
	}
	i := e.Identity
	if i.Repository != context.Repository || i.Ref != "refs/heads/main" ||
		i.CommitSHA != contextCommit(context) || i.WorkflowSHA != in.WorkflowBlobSHA {
		return PromotionEvidence{}, errors.New("governance identity does not match protected evaluated candidate")
	}
	if i.CollectedAt.After(context.Now) || context.Now.Sub(i.CollectedAt) > time.Hour {
		return PromotionEvidence{}, errors.New("governance evidence is stale or future-dated")
	}
	acceptanceSHA256, err := canonicalAuthorityDigest(in.Acceptance)
	if err != nil {
		return PromotionEvidence{}, err
	}
	qualificationSHA256, err := canonicalAuthorityDigest(in.Qualification)
	if err != nil {
		return PromotionEvidence{}, err
	}
	governanceSHA256, err := canonicalAuthorityDigest(struct {
		Evaluation governancedrift.Evaluation   `json:"evaluation"`
		Decision   governancedrift.GateDecision `json:"decision"`
	}{e, g})
	if err != nil {
		return PromotionEvidence{}, err
	}
	atomicitySHA256, err := canonicalAuthorityDigest(q.Atomicity)
	if err != nil {
		return PromotionEvidence{}, err
	}
	prepublication := digestBytes([]byte(acceptanceSHA256 + governanceSHA256))
	authority, err := NewProductionPromotionAuthority(context, in.Acceptance.Rows, acceptanceSHA256, qualificationSHA256, governanceSHA256, prepublication)
	if err != nil {
		return PromotionEvidence{}, err
	}
	report, err := (PromotionHarness{
		Root: in.DisposableHarnessRoot, Context: context, Mode: PromotionHarnessExactCandidate,
		Evaluate: ProductionPromotionRowEvaluator(authority),
	}).Run()
	if err != nil {
		return PromotionEvidence{}, err
	}
	claudeIdentities := []string{
		"version:" + q.ResolvedClaudeVersion,
		"npm-integrity:" + q.ClaudeIntegrity,
		"executable-sha256:" + q.ClaudeDigest,
	}
	sort.Strings(claudeIdentities)
	return report.BuildAggregate(context, PromotionAggregateCandidate{
		PackageCandidate: q.PackyExecutableDigest,
		ClaudeIdentities: claudeIdentities,
		AtomicitySHA256:  atomicitySHA256,
	})
}

func canonicalAuthorityDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func validateProductionAtomicity(q ProductionQualification) error {
	a := q.Atomicity
	want := []struct {
		name     string
		args     []string
		exitCode int
	}{
		{"claude", []string{"--version"}, 0},
		{"packy", []string{"version"}, 0},
		{"packy", []string{"init", "--home", filepath.Join(q.Sandbox, "home"), "--source-root", filepath.Join(q.Sandbox, "installed-source"), "--repository-url", filepath.Join(q.Sandbox, "source-repository"), "--repository-ref", "packy-smoke-proved-source"}, 0},
		{"packy", []string{"doctor"}, 0},
		{"packy", []string{"install"}, 1},
		{"packy", []string{"update"}, 1},
		{"packy", []string{"uninstall"}, 1},
		{"claude", []string{"version"}, 0},
	}
	if len(a.Commands) != len(want) {
		return errors.New("production atomicity commands are missing or incomplete")
	}
	for i, command := range a.Commands {
		if command.Name != want[i].name || !equalStrings(command.Args, want[i].args) || command.ExitCode != want[i].exitCode {
			return fmt.Errorf("production atomicity command %d is malformed or out of order", i)
		}
		if want[i].exitCode != 0 && !strings.Contains(command.Stdout+command.Stderr, "unknown command") {
			return fmt.Errorf("production atomicity command %d did not prove the root command is absent", i)
		}
	}
	if err := validateProductionManifest("before", a.Before); err != nil {
		return err
	}
	if err := validateProductionManifest("after", a.After); err != nil {
		return err
	}
	s := a.Safety
	if !s.DisposableSandbox || !s.AllowlistEnvironment || !s.CredentialsScrubbed || !s.CommandAllowlist ||
		!s.CheckoutUnchanged || !s.ConfiguredWritableRootsConfined || !s.EvidencePathOutsideSandbox ||
		!s.NoInteractiveClaude || !s.WriteBoundaryEnforced {
		return errors.New("production runner safety is incomplete")
	}
	x := a.Assertions
	if !x.InstalledSourceInitialized || !x.DoctorReportedCoreHealthy ||
		!x.RemovedInstallRejected || !x.RemovedUpdateRejected || !x.RemovedUninstallRejected ||
		!x.ClassicStatePreserved || !x.ClaudeInstructionPreserved || !x.ClaudeMCPPreserved ||
		!x.SharedSkillSentinelPreserved || !x.NoPacksOwnershipState || !x.NoClaudeMutationOperations ||
		!x.EngramStubProtocolVerified || !x.SensitiveFixtureRedacted {
		return errors.New("production runner assertions are incomplete")
	}
	o := a.Observation
	if !cleanProductionPath(q.Sandbox) || !cleanProductionPath(o.InstalledSource) ||
		!withinProductionPath(q.Sandbox, o.InstalledSource) || !o.InstalledSourceClean ||
		o.InstalledSourceCommit != q.Commit || !o.CollectedAt.Equal(q.CollectedAt) {
		return errors.New("production qualification observation is not bound to candidate and sandbox")
	}
	for _, root := range []string{o.WritableRoots.Home, o.WritableRoots.XDGConfig, o.WritableRoots.ClaudeConfig, o.WritableRoots.State, o.WritableRoots.Package, o.WritableRoots.Repository, o.WritableRoots.Acquisition} {
		if !cleanProductionPath(root) || !withinProductionPath(q.Sandbox, root) {
			return errors.New("production writable roots are incomplete or outside sandbox")
		}
	}
	os := o.Safety
	if !os.NoGoRun || !os.NoDevelopmentPath || !os.NoDirectFixture || !os.NoUntrackedInput ||
		!os.NoAuthentication || !os.NoModelInvocation || !os.NoPrint || !os.NoREPL ||
		!os.NoUpstreamExecute || !os.NoCredentials || !os.NoOutsideWrite {
		return errors.New("production observed safety is incomplete")
	}
	commands, err := json.Marshal(a.Commands)
	if err != nil {
		return err
	}
	if o.ProcessLogDigest != digestBytes(commands) {
		return errors.New("production process log digest does not match exact commands")
	}
	return nil
}

func validateProductionManifest(name string, files []ProductionFile) error {
	if len(files) == 0 {
		return fmt.Errorf("production %s manifest is empty", name)
	}
	last := ""
	for _, file := range files {
		if file.Path == "" || filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path ||
			file.Path == "." || file.Path == ".." || strings.HasPrefix(file.Path, ".."+string(filepath.Separator)) ||
			file.Path <= last || file.Size < 0 {
			return fmt.Errorf("production %s manifest is malformed or unordered", name)
		}
		mode := os.FileMode(file.Mode)
		if mode&^(os.ModePerm|os.ModeDir|os.ModeSymlink) != 0 ||
			(mode.Type() != 0 && mode.Type() != os.ModeDir && mode.Type() != os.ModeSymlink) ||
			(mode.IsRegular() && !validAuthorityDigest(file.SHA256)) ||
			(!mode.IsRegular() && file.SHA256 != "") {
			return fmt.Errorf("production %s manifest has invalid type or digest", name)
		}
		last = file.Path
	}
	return nil
}

func cleanProductionPath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func withinProductionPath(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NewAcceptanceRunReport(repository, commit, workflowDigest, runID string, rows []AcceptanceRunRow) AcceptanceRunReport {
	return AcceptanceRunReport{
		Schema: AcceptanceRunReportSchema, Synthetic: false, Repository: strings.TrimSpace(repository),
		CommitSHA: commit, WorkflowDigest: workflowDigest, RunID: strings.TrimSpace(runID),
		Qualified: true, Rows: append([]AcceptanceRunRow(nil), rows...),
	}
}
