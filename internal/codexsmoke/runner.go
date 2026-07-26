// Package codexsmoke proves that an exact, package-installed Codex release
// discovers Packy's canonical Vercel skill projections without contacting a model.
package codexsmoke

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

const ExactFloor = "0.145.0"

type Config struct{ Codex, SearchPath, Version, Integrity, PackyRef, PackySHA, RunID, EvidencePath string }
type SkillEvidence struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	SHA256              string `json:"complete_tree_sha256"`
	Invocation          string `json:"invocation"`
	Enabled             bool   `json:"enabled"`
	InvocationAvailable bool   `json:"invocation_available"`
}
type CommandEvidence struct {
	Name    string   `json:"name"`
	Args    []string `json:"args"`
	Outcome string   `json:"outcome"`
}
type RuntimeModeEvidence struct {
	ResourceID        string                              `json:"resource_id"`
	ModeID            string                              `json:"mode_id"`
	Invocation        string                              `json:"invocation"`
	State             capabilitypack.RuntimeModeState     `json:"state"`
	Requirements      []capabilitypack.RuntimeRequirement `json:"requirements"`
	Authorities       []capabilitypack.RuntimeAuthority   `json:"authorities"`
	Effects           []capabilitypack.RuntimeEffect      `json:"effects"`
	Fallback          capabilitypack.RuntimeFallback      `json:"fallback"`
	FallbackState     *capabilitypack.RuntimeModeState    `json:"fallback_state,omitempty"`
	Affected          []string                            `json:"affected"`
	SelectionObserved bool                                `json:"selection_observed"`
	FailBeforeEffects bool                                `json:"fail_before_effects"`
}
type Evidence struct {
	SchemaVersion          int                                    `json:"schema_version"`
	RunID                  string                                 `json:"run_id"`
	ObservedAt             time.Time                              `json:"observed_at"`
	PackyRef               string                                 `json:"packy_ref"`
	PackySHA               string                                 `json:"packy_sha"`
	VercelFixtureSHA256    string                                 `json:"vercel_fixture_sha256"`
	CodexVersion           string                                 `json:"codex_version"`
	CodexNPMIntegrity      string                                 `json:"codex_npm_integrity"`
	CodexExecutableSHA256  string                                 `json:"codex_executable_sha256"`
	SandboxRoots           []string                               `json:"sandbox_roots"`
	CommandAllowlist       []string                               `json:"command_allowlist"`
	Commands               []CommandEvidence                      `json:"commands"`
	Skills                 []SkillEvidence                        `json:"skills"`
	RuntimeModes           []RuntimeModeEvidence                  `json:"runtime_modes"`
	SemanticRerun          vercelacceptance.SemanticRerunEvidence `json:"semantic_rerun"`
	Mutation               vercelacceptance.MutationObservation   `json:"mutation_observation"`
	MissingOneNegativeTwin string                                 `json:"missing_one_negative_twin"`
	NoAuthentication       bool                                   `json:"no_authentication"`
	NoModelInvocation      bool                                   `json:"no_model_invocation"`
	NoDeploy               bool                                   `json:"no_deploy"`
	NoUpstreamExecution    bool                                   `json:"no_upstream_execution"`
}

func ResolveSelector(selector, output string) (string, string, error) {
	if selector != ExactFloor {
		return "", "", fmt.Errorf("Codex selector must be exactly %s", ExactFloor)
	}
	var m struct {
		Version   string `json:"version"`
		Integrity string `json:"dist.integrity"`
	}
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		return "", "", err
	}
	if m.Version != ExactFloor || m.Integrity == "" {
		return "", "", errors.New("npm metadata did not resolve the exact Codex release with integrity")
	}
	return m.Version, m.Integrity, nil
}

func Run(ctx context.Context, cfg Config) (Evidence, error) {
	if cfg.Codex == "" || cfg.SearchPath == "" || cfg.Version != ExactFloor || cfg.Integrity == "" || cfg.PackyRef == "" || cfg.PackySHA == "" || strings.TrimSpace(cfg.RunID) == "" || cfg.EvidencePath == "" {
		return Evidence{}, errors.New("exact Codex acquisition, Packy identity, run ID, and evidence path are required")
	}
	evidenceAbs, err := filepath.Abs(cfg.EvidencePath)
	if err != nil {
		return Evidence{}, err
	}
	sandbox, err := os.MkdirTemp("", "packy-codex-smoke-")
	if err != nil {
		return Evidence{}, err
	}
	defer os.RemoveAll(sandbox)
	if within(sandbox, evidenceAbs) {
		return Evidence{}, errors.New("evidence must be outside disposable sandbox")
	}
	home, bundle, work := filepath.Join(sandbox, "home"), filepath.Join(sandbox, "bundle"), filepath.Join(sandbox, "work")
	for _, d := range []string{home, bundle, work, filepath.Join(home, ".codex")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return Evidence{}, err
		}
	}
	if err := materialize(bundle); err != nil {
		return Evidence{}, err
	}
	fixture := vercelacceptance.Canonical()
	projections := make([]skillProjection, 0, 9)
	invocations := map[string]string{}
	resourceInvocations := map[string]string{}
	for _, r := range fixture.Pack.Resources {
		if r.Kind == "skill" {
			for _, b := range r.Bindings {
				if b.Surface == "codex" {
					invocations[b.Name] = b.Invocation
					resourceInvocations[r.ID] = b.Invocation
					projections = append(projections, skillProjection{
						Name:   b.Name,
						Source: filepath.Join(bundle, filepath.Clean(r.Source)),
						Target: filepath.Join(home, ".agents", "skills", b.Name),
					})
				}
			}
		}
	}
	if len(projections) != 9 {
		return Evidence{}, fmt.Errorf("expected nine Codex projections, got %d", len(projections))
	}
	if err := materializeCodexSkillLinks(projections); err != nil {
		return Evidence{}, err
	}
	bundleBefore, err := localprojection.FingerprintTree(bundle)
	if err != nil {
		return Evidence{}, err
	}
	versionOut, cmd1, err := runVersion(ctx, cfg.Codex, cfg.SearchPath, home, work)
	if err != nil {
		return Evidence{}, err
	}
	if !strings.Contains(versionOut, ExactFloor) {
		return Evidence{}, fmt.Errorf("unexpected codex --version: %s", versionOut)
	}
	digest, err := fileSHA(cfg.Codex)
	if err != nil {
		return Evidence{}, err
	}
	listed, cmd2, err := listSkills(ctx, cfg.Codex, cfg.SearchPath, home, work)
	if err != nil {
		return Evidence{}, err
	}
	byName := map[string]listedSkill{}
	for _, s := range listed {
		byName[s.Name] = s
	}
	skills := make([]SkillEvidence, 0, 9)
	for _, projection := range projections {
		name := projection.Name
		s, ok := byName[name]
		if !ok || !s.Enabled {
			return Evidence{}, fmt.Errorf("Codex did not load enabled skill %s", name)
		}
		want := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		source := filepath.Join(projection.Source, "SKILL.md")
		if !samePath(s.Path, want) && !samePath(s.Path, source) {
			return Evidence{}, fmt.Errorf("skill %s loaded from %s, want projected target or source", name, s.Path)
		}
		fp, e := localprojection.FingerprintTree(projection.Source)
		if e != nil {
			return Evidence{}, e
		}
		skills = append(skills, SkillEvidence{Name: name, Path: s.Path, SHA256: fp, Invocation: invocations[name], Enabled: s.Enabled})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	firstSemantic, err := codexDiscoveryDigest(listed, invocations, sandbox)
	if err != nil {
		return Evidence{}, err
	}
	rerunListed, rerunCommand, err := listSkills(ctx, cfg.Codex, cfg.SearchPath, home, work)
	if err != nil {
		return Evidence{}, err
	}
	secondSemantic, err := codexDiscoveryDigest(rerunListed, invocations, sandbox)
	if err != nil {
		return Evidence{}, err
	}
	semanticRerun := vercelacceptance.SemanticRerunEvidence{FirstSHA256: firstSemantic, SecondSHA256: secondSemantic, ExactMatch: firstSemantic == secondSemantic}
	if !semanticRerun.Valid() {
		return Evidence{}, errors.New("Codex semantic discovery rerun differed")
	}
	var promptCommands []CommandEvidence
	for i := range skills {
		command, err := verifyPromptInvocation(ctx, cfg.Codex, cfg.SearchPath, home, work, skills[i].Name, skills[i].Invocation)
		if err != nil {
			return Evidence{}, err
		}
		skills[i].InvocationAvailable = true
		promptCommands = append(promptCommands, command)
	}
	runtimeModes, modeCommands, err := verifyRuntimeModes(
		ctx, cfg.Codex, cfg.SearchPath, home, work, fixture.Pack, resourceInvocations,
	)
	if err != nil {
		return Evidence{}, err
	}
	missing := skills[0].Name
	if err := os.Remove(filepath.Join(home, ".agents", "skills", missing)); err != nil {
		return Evidence{}, err
	}
	twin, cmd3, err := listSkills(ctx, cfg.Codex, cfg.SearchPath, home, work)
	if err != nil {
		return Evidence{}, err
	}
	count := 0
	for _, s := range twin {
		if _, ok := invocations[s.Name]; ok {
			if s.Name == missing {
				return Evidence{}, errors.New("missing-one twin still loaded removed skill")
			}
			count++
		}
	}
	if count != 8 {
		return Evidence{}, fmt.Errorf("missing-one twin loaded %d Vercel skills", count)
	}
	commands := []CommandEvidence{cmd1, cmd2, rerunCommand}
	commands = append(commands, promptCommands...)
	commands = append(commands, modeCommands...)
	commands = append(commands, cmd3)
	bundleAfter, err := localprojection.FingerprintTree(bundle)
	if err != nil {
		return Evidence{}, err
	}
	mutation := vercelacceptance.MutationObservation{Root: "$SANDBOX/bundle", BeforeSHA256: bundleBefore, AfterSHA256: bundleAfter, AllowedChanges: []string{}, ChangedPaths: []string{}, ZeroMutationExact: bundleBefore == bundleAfter}
	if !mutation.Valid() {
		return Evidence{}, errors.New("Codex host mutated the immutable fixture bundle")
	}
	e := Evidence{
		SchemaVersion: 1, RunID: cfg.RunID, ObservedAt: time.Now().UTC(), PackyRef: cfg.PackyRef, PackySHA: cfg.PackySHA,
		VercelFixtureSHA256: vercelacceptance.ExactArchiveSHA256,
		CodexVersion:        strings.TrimSpace(versionOut), CodexNPMIntegrity: cfg.Integrity,
		CodexExecutableSHA256: digest,
		SandboxRoots:          []string{"$SANDBOX/home", "$SANDBOX/bundle", "$SANDBOX/work"},
		CommandAllowlist:      []string{"codex --version", "codex app-server", "codex debug prompt-input"},
		Commands:              commands, Skills: sanitizeSkills(skills, sandbox), RuntimeModes: runtimeModes,
		SemanticRerun: semanticRerun, Mutation: mutation,
		MissingOneNegativeTwin: missing,
		NoAuthentication:       true, NoModelInvocation: true, NoDeploy: true, NoUpstreamExecution: true,
	}
	if err := validateArtifactIdentity(e.RunID, e.ObservedAt); err != nil {
		return Evidence{}, err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return Evidence{}, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(evidenceAbs), 0700); err != nil {
		return Evidence{}, err
	}
	if err := os.WriteFile(evidenceAbs, data, 0600); err != nil {
		return Evidence{}, err
	}
	return e, nil
}

func codexDiscoveryDigest(listed []listedSkill, invocations map[string]string, sandbox string) (string, error) {
	type row struct {
		Name, Path string
		Enabled    bool
	}
	rows := make([]row, 0, len(invocations))
	for _, skill := range listed {
		if _, ok := invocations[skill.Name]; ok {
			rows = append(rows, row{skill.Name, sanitizePath(skill.Path, sandbox), skill.Enabled})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	if len(rows) != len(invocations) {
		return "", fmt.Errorf("Codex semantic discovery contains %d Vercel skills, want %d", len(rows), len(invocations))
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateArtifactIdentity(runID string, observedAt time.Time) error {
	if strings.TrimSpace(runID) == "" || observedAt.IsZero() || observedAt.Location() != time.UTC {
		return errors.New("evidence requires a nonempty run ID and nonzero UTC observation time")
	}
	return nil
}

func materialize(root string) error {
	files, err := vercelacceptance.InspectExactArchive()
	if err != nil {
		return err
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if !within(root, p) {
			return errors.New("unsafe fixture path")
		}
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Content, os.FileMode(f.Mode)&0777); err != nil {
			return err
		}
	}
	return nil
}

type skillProjection struct {
	Name, Source, Target string
}

// materializeCodexSkillLinks prepares only the disposable host fixture. The
// independent conformance suite proves that the Codex adapter derives the same
// nine complete-tree targets through the lifecycle gateway.
func materializeCodexSkillLinks(projections []skillProjection) error {
	for _, projection := range projections {
		if err := os.MkdirAll(filepath.Dir(projection.Target), 0o700); err != nil {
			return err
		}
		if err := os.Symlink(projection.Source, projection.Target); err != nil {
			return fmt.Errorf("materialize disposable Codex skill %s: %w", projection.Name, err)
		}
	}
	return nil
}

type listedSkill struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

func listSkills(ctx context.Context, binary, searchPath, home, cwd string) ([]listedSkill, CommandEvidence, error) {
	cmd := exec.CommandContext(ctx, binary, "app-server", "--stdio")
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "PATH=" + searchPath, "NO_COLOR=1"}
	in, e := cmd.StdinPipe()
	if e != nil {
		return nil, CommandEvidence{}, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return nil, CommandEvidence{}, e
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if e = cmd.Start(); e != nil {
		return nil, CommandEvidence{}, e
	}
	defer func() {
		_ = in.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	enc := json.NewEncoder(in)
	if err := enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "packy-codex-smoke", "version": "1"}}}); err != nil {
		return nil, CommandEvidence{}, err
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 1024), 4<<20)
	var found []listedSkill
	for scan.Scan() {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Data []struct {
					Skills []listedSkill `json:"skills"`
					Errors []any         `json:"errors"`
				} `json:"data"`
			} `json:"result"`
		}
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			return nil, CommandEvidence{}, fmt.Errorf("decode Codex app-server response: %w", err)
		}
		if msg.ID == 1 {
			if err := enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return nil, CommandEvidence{}, err
			}
			if err := enc.Encode(map[string]any{"id": 2, "method": "skills/list", "params": map[string]any{"cwds": []string{cwd}, "forceReload": true}}); err != nil {
				return nil, CommandEvidence{}, err
			}
		}
		if msg.ID == 2 {
			for _, d := range msg.Result.Data {
				if len(d.Errors) > 0 {
					return nil, CommandEvidence{}, errors.New("Codex reported skill errors")
				}
				found = append(found, d.Skills...)
			}
			break
		}
	}
	if err := scan.Err(); err != nil {
		return nil, CommandEvidence{}, err
	}
	if len(found) == 0 {
		return nil, CommandEvidence{}, fmt.Errorf("skills/list returned no skills: %s", stderr.String())
	}
	return found, CommandEvidence{"codex", []string{"app-server", "--stdio"}, "protocol_observed"}, nil
}
func runVersion(ctx context.Context, binary, searchPath, home, cwd string) (string, CommandEvidence, error) {
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "PATH=" + searchPath}
	b, e := cmd.CombinedOutput()
	ce := CommandEvidence{"codex", []string{"--version"}, "exited_0"}
	if e != nil {
		ce.Outcome = "failed"
	}
	return string(b), ce, e
}

func verifyPromptInvocation(ctx context.Context, binary, searchPath, home, cwd, name, invocation string) (CommandEvidence, error) {
	args := []string{"debug", "prompt-input", invocation}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "PATH=" + searchPath, "NO_COLOR=1"}
	out, err := cmd.Output()
	evidence := CommandEvidence{Name: "codex", Args: args, Outcome: "exited_0"}
	if err != nil {
		evidence.Outcome = "failed"
		return evidence, fmt.Errorf("Codex prompt-input for %s: %w", invocation, err)
	}
	if err := validatePromptInput(out, name, invocation); err != nil {
		return evidence, err
	}
	return evidence, nil
}

func verifyRuntimeModes(
	ctx context.Context,
	binary, searchPath, home, cwd string,
	pack capabilitypack.Pack,
	invocations map[string]string,
) ([]RuntimeModeEvidence, []CommandEvidence, error) {
	now := time.Unix(0, 0).UTC()
	records := make([]capabilitypack.RuntimeModeEvidence, 0, 28)
	observation := capabilitypack.RuntimeObservation{
		State: capabilitypack.ObservationUnverified, Reason: capabilitypack.ObservationReasonObserverError,
		ObservedAt: now.Format(time.RFC3339), ObserverRevision: "codex-smoke-no-runtime-observer-v1",
	}
	for _, resource := range pack.Resources {
		for _, mode := range resource.RuntimeModes {
			evidence := capabilitypack.RuntimeEvidence{
				Requirements: make([]capabilitypack.RuntimeRequirementObservation, 0, len(mode.Requirements)),
				Authorities:  make([]capabilitypack.RuntimeAuthorityObservation, 0, len(mode.Authorities)),
			}
			for _, requirement := range mode.Requirements {
				evidence.Requirements = append(evidence.Requirements, capabilitypack.RuntimeRequirementObservation{
					Kind: requirement.Kind, ID: requirement.ID, RuntimeObservation: observation,
				})
			}
			for _, authority := range mode.Authorities {
				evidence.Authorities = append(evidence.Authorities, capabilitypack.RuntimeAuthorityObservation{
					Kind: authority.Kind, Scope: authority.Scope, RuntimeObservation: observation,
				})
			}
			records = append(records, capabilitypack.RuntimeModeEvidence{
				ResourceID: resource.ID, ModeID: mode.ID, Evidence: evidence,
			})
		}
	}
	results, err := capabilitypack.EvaluateRuntimeModes(pack, records, now, time.Minute)
	if err != nil {
		return nil, nil, err
	}
	modes := make([]RuntimeModeEvidence, 0, len(results))
	commands := make([]CommandEvidence, 0, len(results))
	for _, result := range results {
		invocation := invocations[result.ResourceID]
		if invocation == "" {
			return nil, nil, fmt.Errorf("runtime mode %s:%s has no Codex invocation", result.ResourceID, result.ModeID)
		}
		_, preflightErr := capabilitypack.PreflightRuntimeMode(
			pack, result.ResourceID, result.ModeID, records, now, time.Minute,
		)
		failBeforeEffects := false
		if result.State == capabilitypack.RuntimeModeAvailable {
			if preflightErr != nil {
				return nil, nil, preflightErr
			}
		} else {
			var failure capabilitypack.RuntimePreflightError
			if !errors.As(preflightErr, &failure) {
				return nil, nil, fmt.Errorf("runtime mode %s:%s did not fail through typed preflight: %w", result.ResourceID, result.ModeID, preflightErr)
			}
			failBeforeEffects = true
		}
		selection := invocation + " " + result.ModeID
		command, err := verifyPromptInvocation(
			ctx, binary, searchPath, home, cwd, strings.TrimPrefix(invocation, "$"), selection,
		)
		if err != nil {
			return nil, nil, err
		}
		commands = append(commands, command)
		modes = append(modes, RuntimeModeEvidence{
			ResourceID: result.ResourceID, ModeID: result.ModeID, Invocation: selection,
			State: result.State, Requirements: result.Requirements, Authorities: result.Authorities,
			Effects: result.Effects, Fallback: result.Fallback, FallbackState: result.FallbackState,
			Affected: result.Affected, SelectionObserved: true, FailBeforeEffects: failBeforeEffects,
		})
	}
	return modes, commands, nil
}

func validatePromptInput(data []byte, name, invocation string) error {
	var wire any
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode Codex prompt-input: %w", err)
	}
	var texts []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "text" {
					if text, ok := child.(string); ok {
						texts = append(texts, text)
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(wire)
	available, invoked := false, false
	for _, text := range texts {
		available = available || strings.Contains(text, "\n- "+name+":")
		invoked = invoked || text == invocation
	}
	if !available || !invoked {
		return fmt.Errorf("Codex prompt-input did not expose and preserve invocation %s", invocation)
	}
	return nil
}
func fileSHA(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func within(root, p string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && aa == bb
}

func sanitizeSkills(skills []SkillEvidence, sandbox string) []SkillEvidence {
	out := append([]SkillEvidence(nil), skills...)
	for i := range out {
		if rel, err := filepath.Rel(sandbox, out[i].Path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			out[i].Path = filepath.ToSlash(filepath.Join("$SANDBOX", rel))
		}
	}
	return out
}

func sanitizePath(path, sandbox string) string {
	if rel, err := filepath.Rel(sandbox, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Join("$SANDBOX", rel))
	}
	return path
}
