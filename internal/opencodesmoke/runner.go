// Package opencodesmoke proves that an exact OpenCode release discovers Packy's
// canonical Vercel skills without model, authentication, network, or skill execution.
package opencodesmoke

import (
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

const ExactVersion = "1.18.5"

type Config struct{ OpenCode, SearchPath, Version, Integrity, PackyRef, PackySHA, EvidencePath string }
type SkillEvidence struct {
	Name          string `json:"name"`
	Location      string `json:"location"`
	SHA256        string `json:"complete_tree_sha256"`
	ContentLoaded bool   `json:"content_loaded"`
}
type RuntimeModeEvidence struct {
	ResourceID            string                              `json:"resource_id"`
	ModeID                string                              `json:"mode_id"`
	Invocation            string                              `json:"invocation"`
	State                 capabilitypack.RuntimeModeState     `json:"state"`
	Requirements          []capabilitypack.RuntimeRequirement `json:"requirements"`
	Authorities           []capabilitypack.RuntimeAuthority   `json:"authorities"`
	Effects               []capabilitypack.RuntimeEffect      `json:"effects"`
	Fallback              capabilitypack.RuntimeFallback      `json:"fallback"`
	FallbackState         *capabilitypack.RuntimeModeState    `json:"fallback_state,omitempty"`
	Affected              []string                            `json:"affected"`
	SelectionObserved     bool                                `json:"selection_observed"`
	InvocationAvailable   bool                                `json:"invocation_available"`
	FailBeforeHostEffects bool                                `json:"fail_before_host_effects"`
}
type Evidence struct {
	SchemaVersion            int                   `json:"schema_version"`
	PackyRef                 string                `json:"packy_ref"`
	PackySHA                 string                `json:"packy_sha"`
	VercelFixtureSHA256      string                `json:"vercel_fixture_sha256"`
	OpenCodeVersion          string                `json:"opencode_version"`
	OpenCodeArchiveSHA256    string                `json:"opencode_archive_sha256"`
	OpenCodeExecutableSHA256 string                `json:"opencode_executable_sha256"`
	SandboxRoots             []string              `json:"sandbox_roots"`
	CommandAllowlist         []string              `json:"command_allowlist"`
	Skills                   []SkillEvidence       `json:"skills"`
	RuntimeModes             []RuntimeModeEvidence `json:"runtime_modes"`
	MissingOneNegativeTwin   string                `json:"missing_one_negative_twin"`
	NoAuthentication         bool                  `json:"no_authentication"`
	NoExternalModelNetwork   bool                  `json:"no_external_model_network"`
	NoDeploy                 bool                  `json:"no_deploy"`
	NativeSkillToolObserved  bool                  `json:"native_skill_tool_observed"`
	NoUpstreamEffects        bool                  `json:"no_upstream_effects"`
}

type expectedSkill struct{ name, source, target, content string }

func Run(ctx context.Context, cfg Config) (Evidence, error) {
	if cfg.OpenCode == "" || cfg.SearchPath == "" || cfg.Version != ExactVersion || cfg.Integrity == "" || cfg.PackyRef == "" || cfg.PackySHA == "" || cfg.EvidencePath == "" {
		return Evidence{}, errors.New("exact OpenCode acquisition, Packy identity, and evidence path are required")
	}
	evidenceAbs, err := filepath.Abs(cfg.EvidencePath)
	if err != nil {
		return Evidence{}, err
	}
	sandbox, err := os.MkdirTemp("", "packy-opencode-smoke-")
	if err != nil {
		return Evidence{}, err
	}
	defer os.RemoveAll(sandbox)
	if within(sandbox, evidenceAbs) {
		return Evidence{}, errors.New("evidence must be outside disposable sandbox")
	}
	home, xdg, xdgData, cache, state, bundle, work := filepath.Join(sandbox, "home"), filepath.Join(sandbox, "xdg"), filepath.Join(sandbox, "data"), filepath.Join(sandbox, "cache"), filepath.Join(sandbox, "state"), filepath.Join(sandbox, "bundle"), filepath.Join(sandbox, "work")
	for _, dir := range []string{home, xdg, xdgData, cache, state, bundle, work, filepath.Join(work, ".opencode", "skills")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return Evidence{}, err
		}
	}
	if err := materialize(bundle); err != nil {
		return Evidence{}, err
	}
	fixture := vercelacceptance.Canonical()
	expected := make([]expectedSkill, 0, 9)
	for _, resource := range fixture.Pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface != capabilitypack.SurfaceOpenCode {
				continue
			}
			source := filepath.Join(bundle, filepath.Clean(resource.Source))
			content, e := os.ReadFile(filepath.Join(source, "SKILL.md"))
			if e != nil {
				return Evidence{}, e
			}
			expected = append(expected, expectedSkill{binding.Name, source, filepath.Join(work, ".opencode", "skills", binding.Name), skillBody(string(content))})
		}
	}
	if len(expected) != 9 {
		return Evidence{}, fmt.Errorf("expected nine OpenCode skills, got %d", len(expected))
	}
	for _, skill := range expected {
		if err := os.Symlink(skill.source, skill.target); err != nil {
			return Evidence{}, err
		}
	}
	modes, err := preflightEveryMode(fixture.Pack)
	if err != nil {
		return Evidence{}, err
	} // Must precede every host process.
	versionOut, err := runHost(ctx, cfg, home, xdg, work, "--version")
	if err != nil {
		return Evidence{}, err
	}
	if !strings.Contains(string(versionOut), ExactVersion) {
		return Evidence{}, fmt.Errorf("unexpected opencode --version: %s", versionOut)
	}
	raw, err := runHost(ctx, cfg, home, xdg, work, "debug", "skill", "--pure")
	if err != nil {
		return Evidence{}, err
	}
	listed, err := parseSkills(raw)
	if err != nil {
		return Evidence{}, err
	}
	skills := make([]SkillEvidence, 0, 9)
	for _, want := range expected {
		got, ok := listed[want.name]
		if !ok {
			return Evidence{}, fmt.Errorf("OpenCode did not discover %s", want.name)
		}
		if !samePath(got.location, want.target) && !samePath(got.location, want.source) && !samePath(got.location, filepath.Join(want.target, "SKILL.md")) && !samePath(got.location, filepath.Join(want.source, "SKILL.md")) {
			return Evidence{}, fmt.Errorf("skill %s loaded from unexpected location %s", want.name, got.location)
		}
		if !strings.Contains(got.content, strings.TrimSpace(want.content)) {
			return Evidence{}, fmt.Errorf("OpenCode did not load content for %s", want.name)
		}
		fp, e := localprojection.FingerprintTree(want.source)
		if e != nil {
			return Evidence{}, e
		}
		skills = append(skills, SkillEvidence{want.name, sanitize(got.location, sandbox), fp, true})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	if err := observeACPSelections(ctx, cfg, home, xdg, xdgData, cache, state, work, expected, modes); err != nil {
		return Evidence{}, err
	}
	missing := expected[0].name
	if err := os.Remove(expected[0].target); err != nil {
		return Evidence{}, err
	}
	rawTwin, err := runHost(ctx, cfg, home, xdg, work, "debug", "skill", "--pure")
	if err != nil {
		return Evidence{}, err
	}
	listedTwin, err := parseSkills(rawTwin)
	if err != nil {
		return Evidence{}, err
	}
	if _, ok := listedTwin[missing]; ok {
		return Evidence{}, fmt.Errorf("missing-one twin still discovered %s", missing)
	}
	twinCount := 0
	for _, want := range expected[1:] {
		if _, ok := listedTwin[want.name]; ok {
			twinCount++
		}
	}
	if twinCount != 8 {
		return Evidence{}, fmt.Errorf("missing-one twin discovered %d Vercel skills, want 8", twinCount)
	}
	executableSHA, err := fileSHA(cfg.OpenCode)
	if err != nil {
		return Evidence{}, err
	}
	ev := Evidence{SchemaVersion: 2, PackyRef: cfg.PackyRef, PackySHA: cfg.PackySHA, VercelFixtureSHA256: vercelacceptance.ExactArchiveSHA256, OpenCodeVersion: strings.TrimSpace(string(versionOut)), OpenCodeArchiveSHA256: cfg.Integrity, OpenCodeExecutableSHA256: executableSHA, SandboxRoots: []string{"$SANDBOX/home", "$SANDBOX/xdg", "$SANDBOX/data", "$SANDBOX/cache", "$SANDBOX/state", "$SANDBOX/bundle", "$SANDBOX/work"}, CommandAllowlist: []string{"opencode --version", "opencode debug skill --pure", "opencode acp --cwd $SANDBOX/work --pure"}, Skills: skills, RuntimeModes: modes, MissingOneNegativeTwin: missing, NoAuthentication: true, NoExternalModelNetwork: true, NoDeploy: true, NativeSkillToolObserved: true, NoUpstreamEffects: true}
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return Evidence{}, err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(evidenceAbs), 0700); err != nil {
		return Evidence{}, err
	}
	if err = os.WriteFile(evidenceAbs, data, 0600); err != nil {
		return Evidence{}, err
	}
	return ev, nil
}

type discoveredSkill struct{ location, content string }

func parseSkills(data []byte) (map[string]discoveredSkill, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode opencode debug skill: %w", err)
	}
	out := map[string]discoveredSkill{}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			name := firstString(x, "name", "id", "slug")
			location := firstString(x, "path", "location", "file", "filename")
			content := firstString(x, "content", "body", "text", "prompt")
			if name != "" && location != "" && content != "" {
				out[name] = discoveredSkill{location, content}
			}
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(root)
	if len(out) == 0 {
		return nil, errors.New("OpenCode debug skill returned no independently verifiable skill records")
	}
	return out, nil
}
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			return s
		}
	}
	return ""
}
func skillBody(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return strings.TrimSpace(content)
	}
	if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
		return strings.TrimSpace(content[end+9:])
	}
	return strings.TrimSpace(content)
}
func preflightEveryMode(pack capabilitypack.Pack) ([]RuntimeModeEvidence, error) {
	now := time.Unix(0, 0).UTC()
	records := make([]capabilitypack.RuntimeModeEvidence, 0, 28)
	ob := capabilitypack.RuntimeObservation{State: capabilitypack.ObservationUnverified, Reason: capabilitypack.ObservationReasonObserverError, ObservedAt: now.Format(time.RFC3339), ObserverRevision: "opencode-smoke-no-runtime-observer-v1"}
	for _, r := range pack.Resources {
		for _, m := range r.RuntimeModes {
			e := capabilitypack.RuntimeEvidence{
				Requirements: make([]capabilitypack.RuntimeRequirementObservation, 0, len(m.Requirements)),
				Authorities:  make([]capabilitypack.RuntimeAuthorityObservation, 0, len(m.Authorities)),
			}
			for _, req := range m.Requirements {
				e.Requirements = append(e.Requirements, capabilitypack.RuntimeRequirementObservation{Kind: req.Kind, ID: req.ID, RuntimeObservation: ob})
			}
			for _, a := range m.Authorities {
				e.Authorities = append(e.Authorities, capabilitypack.RuntimeAuthorityObservation{Kind: a.Kind, Scope: a.Scope, RuntimeObservation: ob})
			}
			records = append(records, capabilitypack.RuntimeModeEvidence{ResourceID: r.ID, ModeID: m.ID, Evidence: e})
		}
	}
	if len(records) != 28 {
		return nil, fmt.Errorf("runtime mode count = %d, want 28", len(records))
	}
	out := make([]RuntimeModeEvidence, 0, 28)
	for _, record := range records {
		result, err := capabilitypack.PreflightRuntimeMode(pack, record.ResourceID, record.ModeID, records, now, time.Minute)
		if err == nil {
			return nil, fmt.Errorf("unverified runtime mode %s:%s unexpectedly passed preflight", record.ResourceID, record.ModeID)
		}
		var failure capabilitypack.RuntimePreflightError
		if !errors.As(err, &failure) {
			return nil, err
		}
		invocation := openCodeInvocation(pack, record.ResourceID)
		if invocation == "" {
			return nil, fmt.Errorf("runtime mode %s:%s has no OpenCode invocation", record.ResourceID, record.ModeID)
		}
		out = append(out, RuntimeModeEvidence{
			ResourceID: record.ResourceID, ModeID: record.ModeID, Invocation: invocation + " " + record.ModeID,
			State: result.State, Requirements: result.Requirements, Authorities: result.Authorities,
			Effects: result.Effects, Fallback: result.Fallback, FallbackState: result.FallbackState,
			Affected: result.Affected, FailBeforeHostEffects: true,
		})
	}
	return out, nil
}
func openCodeInvocation(pack capabilitypack.Pack, resourceID string) string {
	for _, resource := range pack.Resources {
		if resource.ID != resourceID {
			continue
		}
		for _, binding := range resource.Bindings {
			if binding.Surface == capabilitypack.SurfaceOpenCode {
				return binding.Invocation
			}
		}
	}
	return ""
}
func runHost(ctx context.Context, cfg Config, home, xdg, work string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, cfg.OpenCode, args...)
	cmd.Dir = work
	cmd.Env = hostEnv(cfg.SearchPath, home, xdg, filepath.Join(home, ".local", "share"), filepath.Join(home, ".cache"), filepath.Join(home, ".local", "state"))
	outFile, err := os.CreateTemp(work, "opencode-output-")
	if err != nil {
		return nil, err
	}
	outPath := outFile.Name()
	defer os.Remove(outPath)
	var stderr bytes.Buffer
	cmd.Stdout = outFile
	cmd.Stderr = &stderr
	err = cmd.Run()
	closeErr := outFile.Close()
	if err != nil {
		return nil, fmt.Errorf("opencode %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return os.ReadFile(outPath)
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
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(p, f.Content, os.FileMode(f.Mode)&0777); err != nil {
			return err
		}
	}
	return nil
}
func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func samePath(a, b string) bool {
	aa, ea := filepath.EvalSymlinks(a)
	bb, eb := filepath.EvalSymlinks(b)
	return ea == nil && eb == nil && aa == bb
}
func sanitize(path, sandbox string) string {
	if rel, err := filepath.Rel(sandbox, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Join("$SANDBOX", rel))
	}
	canonicalPath, pathErr := filepath.EvalSymlinks(path)
	canonicalSandbox, sandboxErr := filepath.EvalSymlinks(sandbox)
	if pathErr == nil && sandboxErr == nil {
		if rel, err := filepath.Rel(canonicalSandbox, canonicalPath); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(filepath.Join("$SANDBOX", rel))
		}
	}
	return filepath.Base(path)
}

func hostEnv(searchPath, home, xdg, data, cache, state string) []string {
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + xdg, "XDG_DATA_HOME=" + data, "XDG_CACHE_HOME=" + cache, "XDG_STATE_HOME=" + state, "PATH=" + searchPath, "NO_COLOR=1", "OPENCODE_DISABLE_DEFAULT_SKILLS=true", "OPENCODE_DISABLE_EXTERNAL_SKILLS=true"}
}
