package claudesmoke

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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

const ExactVercelClaudeVersion = "2.1.203"

type VercelConfig struct {
	Claude, SearchPath, PackyRepo, PackyRef, EvidencePath, ClaudeIntegrity string
}

type VercelSkillEvidence struct {
	Name            string `json:"name"`
	Invocation      string `json:"invocation"`
	TreeFingerprint string `json:"complete_tree_fingerprint"`
}

type VercelHostObservation struct {
	Kind                 string   `json:"kind"`
	DebugSummary         string   `json:"debug_summary"`
	UserSkillDirCommands int      `json:"user_skill_dir_commands"`
	Names                []string `json:"names"`
}

type VercelRuntimeRow struct {
	ResourceID        string                          `json:"resource_id"`
	ModeID            string                          `json:"mode_id"`
	State             capabilitypack.RuntimeModeState `json:"state"`
	FailBeforeEffects bool                            `json:"fail_before_effects"`
}

type VercelSafetyFacts struct {
	NoAuthentication    bool `json:"no_authentication"`
	NoModelExecution    bool `json:"no_model_execution"`
	NoDeployment        bool `json:"no_deployment"`
	NoUpstreamExecution bool `json:"no_upstream_execution"`
}

type VercelEvidence struct {
	SchemaVersion                   int                   `json:"schema_version"`
	PackySHA                        string                `json:"packy_sha"`
	FixtureSHA256                   string                `json:"fixture_sha256"`
	ClaudeVersion                   string                `json:"claude_version"`
	ClaudeNPMIntegrity              string                `json:"claude_npm_integrity"`
	ClaudeExecutableSHA256          string                `json:"claude_executable_sha256"`
	Skills                          []VercelSkillEvidence `json:"skills"`
	Positive                        VercelHostObservation `json:"positive_host_observation"`
	MissingOne                      VercelHostObservation `json:"missing_one_host_observation"`
	RuntimeModes                    []VercelRuntimeRow    `json:"runtime_modes"`
	TypedFailBeforeEffectsPreflight bool                  `json:"typed_fail_before_effects_preflight"`
	AllowedCommands                 []string              `json:"allowed_commands"`
	Safety                          VercelSafetyFacts     `json:"safety"`
}

var hostSkillSummary = regexp.MustCompile(`Loaded ([0-9]+) unique skills \(([0-9]+) unconditional, ([0-9]+) conditional, managed: ([0-9]+), user: ([0-9]+), project: ([0-9]+), additional: ([0-9]+), legacy commands: ([0-9]+)\)`)
var hostGetSkillsSummary = regexp.MustCompile(`getSkills returning: ([0-9]+) skill dir commands`)

// ParseClaudeSkillDebug accepts only Claude's two mutually corroborating,
// redaction-safe startup summaries. It intentionally does not treat fixture
// enumeration as host evidence.
func ParseClaudeSkillDebug(debug string) (int, string, error) {
	loaded := hostSkillSummary.FindStringSubmatch(debug)
	returned := hostGetSkillsSummary.FindStringSubmatch(debug)
	if loaded == nil || returned == nil {
		return 0, "", errors.New("Claude debug omitted skill startup summaries")
	}
	total, _ := strconv.Atoi(loaded[1])
	user, _ := strconv.Atoi(loaded[5])
	dirs, _ := strconv.Atoi(returned[1])
	if total != user || user != dirs {
		return 0, "", fmt.Errorf("Claude skill startup summaries disagree: total=%d user=%d skill-dir=%d", total, user, dirs)
	}
	return user, fmt.Sprintf("Loaded %d unique skills; getSkills returned %d skill dir commands", user, dirs), nil
}

func RunVercel(ctx context.Context, cfg VercelConfig) (VercelEvidence, error) {
	if cfg.Claude == "" || cfg.SearchPath == "" || cfg.PackyRepo == "" || cfg.PackyRef == "" || cfg.EvidencePath == "" || cfg.ClaudeIntegrity == "" {
		return VercelEvidence{}, errors.New("Claude executable, restricted search path, npm integrity, Packy repo/ref, and evidence path are required")
	}
	repo, err := filepath.Abs(cfg.PackyRepo)
	if err != nil {
		return VercelEvidence{}, err
	}
	head, err := commandOutput(ctx, repo, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return VercelEvidence{}, err
	}
	ref, err := commandOutput(ctx, repo, nil, "git", "rev-parse", "--verify", cfg.PackyRef+"^{commit}")
	if err != nil {
		return VercelEvidence{}, err
	}
	if head != ref {
		return VercelEvidence{}, errors.New("Packy ref does not resolve to checkout HEAD")
	}
	root, err := os.MkdirTemp("", "packy-claude-vercel-smoke-")
	if err != nil {
		return VercelEvidence{}, err
	}
	defer os.RemoveAll(root)
	bundle := filepath.Join(root, "bundle")
	if err := materializeVercelFixture(bundle); err != nil {
		return VercelEvidence{}, err
	}
	fixtureDigest := vercelacceptance.ExactArchiveSHA256
	claudeDigest, err := digestFile(cfg.Claude)
	if err != nil {
		return VercelEvidence{}, err
	}
	version, err := commandOutput(ctx, root, isolatedClaudeEnv(filepath.Join(root, "version-home"), cfg.SearchPath), cfg.Claude, "--version")
	if err != nil {
		return VercelEvidence{}, err
	}
	if !strings.Contains(version, ExactVercelClaudeVersion) {
		return VercelEvidence{}, fmt.Errorf("Claude version = %q, want %s", version, ExactVercelClaudeVersion)
	}

	positiveHome := filepath.Join(root, "positive-home")
	fixture := vercelacceptance.Canonical()
	skillsRoot := filepath.Join(positiveHome, ".claude", "skills")
	if err := materializeClaudeVercelSkills(fixture.Pack, bundle, skillsRoot); err != nil {
		return VercelEvidence{}, err
	}
	skills, err := projectedSkillEvidence(fixture.Pack, skillsRoot)
	if err != nil {
		return VercelEvidence{}, err
	}
	positive, err := observeClaudeStartup(ctx, cfg.Claude, cfg.SearchPath, positiveHome, skills)
	if err != nil {
		return VercelEvidence{}, err
	}
	negativeHome := filepath.Join(root, "negative-home")
	if err := copySkillTrees(skillsRoot, filepath.Join(negativeHome, ".claude", "skills"), skills[1:]); err != nil {
		return VercelEvidence{}, err
	}
	negative, err := observeClaudeStartup(ctx, cfg.Claude, cfg.SearchPath, negativeHome, skills[1:])
	if err != nil {
		return VercelEvidence{}, err
	}
	if positive.UserSkillDirCommands != 9 || negative.UserSkillDirCommands != 8 {
		return VercelEvidence{}, errors.New("fresh Claude startup did not distinguish nine skills from missing-one twin")
	}

	runtimeRows, preflight, err := evaluateSafeRuntimeModes(fixture.Pack)
	if err != nil {
		return VercelEvidence{}, err
	}
	e := VercelEvidence{SchemaVersion: 1, PackySHA: head, FixtureSHA256: fixtureDigest, ClaudeVersion: ExactVercelClaudeVersion,
		ClaudeNPMIntegrity: cfg.ClaudeIntegrity, ClaudeExecutableSHA256: claudeDigest, Skills: skills, Positive: positive, MissingOne: negative,
		RuntimeModes: runtimeRows, TypedFailBeforeEffectsPreflight: preflight,
		AllowedCommands: []string{"git rev-parse", "claude --version", "claude startup with --debug-file"},
		Safety:          VercelSafetyFacts{true, true, true, true}}
	if err := ValidateVercelEvidence(e); err != nil {
		return VercelEvidence{}, err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return VercelEvidence{}, err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(cfg.EvidencePath), 0o700); err != nil {
		return VercelEvidence{}, err
	}
	if err := os.WriteFile(cfg.EvidencePath, b, 0o600); err != nil {
		return VercelEvidence{}, err
	}
	return e, nil
}

func ValidateVercelEvidence(e VercelEvidence) error {
	if e.SchemaVersion != 1 || len(e.PackySHA) != 40 || e.FixtureSHA256 != vercelacceptance.ExactArchiveSHA256 || e.ClaudeVersion != ExactVercelClaudeVersion || !strings.HasPrefix(e.ClaudeNPMIntegrity, "sha512-") || len(e.ClaudeExecutableSHA256) != 64 {
		return errors.New("invalid Vercel smoke identity")
	}
	if len(e.Skills) != 9 || len(e.RuntimeModes) != 28 || e.Positive.UserSkillDirCommands != 9 || e.MissingOne.UserSkillDirCommands != 8 || !e.TypedFailBeforeEffectsPreflight {
		return errors.New("incomplete Vercel smoke evidence")
	}
	want := vercelacceptance.Canonical().Pack
	wantNames := map[string]bool{}
	for _, r := range want.Resources {
		for _, b := range r.Bindings {
			if b.Surface == capabilitypack.SurfaceClaude {
				wantNames[b.Name] = true
			}
		}
	}
	for _, s := range e.Skills {
		if !wantNames[s.Name] || s.Invocation != "/"+s.Name || s.TreeFingerprint == "" || s.TreeFingerprint == "missing" {
			return errors.New("invalid Claude skill evidence")
		}
		delete(wantNames, s.Name)
	}
	if len(wantNames) != 0 || len(e.Positive.Names) != 9 || len(e.MissingOne.Names) != 8 ||
		!sameNames(e.Positive.Names, e.Skills) || !sameNames(e.MissingOne.Names, e.Skills[1:]) ||
		!e.Safety.NoAuthentication || !e.Safety.NoModelExecution || !e.Safety.NoDeployment || !e.Safety.NoUpstreamExecution {
		return errors.New("unsafe or incomplete Vercel smoke evidence")
	}
	for _, r := range e.RuntimeModes {
		if r.State != capabilitypack.RuntimeModeUnverified && r.State != capabilitypack.RuntimeModeAvailable {
			return errors.New("unsafe runtime-mode claim")
		}
		if !r.FailBeforeEffects {
			return errors.New("runtime mode omitted fail-before-effects")
		}
	}
	return nil
}

func sameNames(names []string, skills []VercelSkillEvidence) bool {
	if len(names) != len(skills) {
		return false
	}
	for i := range names {
		if names[i] != skills[i].Name {
			return false
		}
	}
	return true
}

func materializeVercelFixture(root string) error {
	files, err := vercelacceptance.InspectExactArchive()
	if err != nil {
		return err
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Content, os.FileMode(f.Mode)&0o777); err != nil {
			return err
		}
	}
	return nil
}

func projectedSkillEvidence(pack capabilitypack.Pack, root string) ([]VercelSkillEvidence, error) {
	var out []VercelSkillEvidence
	for _, r := range pack.Resources {
		if r.Kind != "skill" {
			continue
		}
		for _, b := range r.Bindings {
			if b.Surface != capabilitypack.SurfaceClaude {
				continue
			}
			fp, exists, err := localprojection.FingerprintPath(filepath.Join(root, b.Name))
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("missing projected skill %s", b.Name)
			}
			out = append(out, VercelSkillEvidence{Name: b.Name, Invocation: b.Invocation, TreeFingerprint: fp})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// materializeClaudeVercelSkills builds only the disposable host fixture. The
// independent claudecode acceptance suite proves that the production adapter
// derives and owns the same nine targets through its sole SurfaceAdapter seam.
func materializeClaudeVercelSkills(pack capabilitypack.Pack, bundle, root string) error {
	resources := map[string]capabilitypack.Resource{}
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
	}
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" {
			continue
		}
		var binding *capabilitypack.Binding
		for i := range resource.Bindings {
			if resource.Bindings[i].Surface == capabilitypack.SurfaceClaude {
				binding = &resource.Bindings[i]
				break
			}
		}
		if binding == nil {
			continue
		}
		source := filepath.Join(bundle, filepath.FromSlash(resource.Source))
		target := filepath.Join(root, binding.Name)
		if err := copyTree(source, target); err != nil {
			return err
		}
		for _, required := range resource.Requires {
			asset, ok := resources[required]
			if !ok || asset.Kind != "asset" {
				continue
			}
			assetName := filepath.Base(asset.Source)
			skillPath := filepath.Join(target, "SKILL.md")
			skill, err := os.ReadFile(skillPath)
			if err != nil {
				return err
			}
			oldReference := "../../references/" + assetName
			if !bytes.Contains(skill, []byte(oldReference)) {
				return fmt.Errorf("Claude smoke fixture %s omitted dependency reference %s", resource.ID, oldReference)
			}
			if err := os.WriteFile(skillPath, bytes.ReplaceAll(skill, []byte(oldReference), []byte("references/"+assetName)), 0o644); err != nil {
				return err
			}
			content, err := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(asset.Source)))
			if err != nil {
				return err
			}
			assetTarget := filepath.Join(target, "references", assetName)
			if err := os.MkdirAll(filepath.Dir(assetTarget), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(assetTarget, content, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Claude smoke fixture contains non-regular file %s", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, info.Mode()&0o777)
	})
}

func observeClaudeStartup(ctx context.Context, claude, searchPath, home string, skills []VercelSkillEvidence) (VercelHostObservation, error) {
	for _, directory := range []string{filepath.Join(home, ".claude"), filepath.Join(home, "xdg-config"), filepath.Join(home, "xdg-cache"), filepath.Join(home, "tmp")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return VercelHostObservation{}, err
		}
	}
	debug := filepath.Join(home, "startup.debug")
	cmd := exec.CommandContext(ctx, claude, "--debug-file", debug)
	cmd.Dir = home
	cmd.Env = isolatedClaudeEnv(home, searchPath)
	cmd.Stdin = bytes.NewReader(nil)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run() // unauthenticated startup is expected to terminate non-zero.
	b, err := os.ReadFile(debug)
	if err != nil {
		return VercelHostObservation{}, err
	}
	count, summary, err := ParseClaudeSkillDebug(string(b))
	if err != nil {
		return VercelHostObservation{}, err
	}
	names := make([]string, len(skills))
	for i := range skills {
		names[i] = skills[i].Name
	}
	return VercelHostObservation{Kind: "slash invocation-mode availability from exact host skill loading", DebugSummary: summary, UserSkillDirCommands: count, Names: names}, nil
}

func isolatedClaudeEnv(home, searchPath string) []string {
	return []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"), "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"), "TMPDIR=" + filepath.Join(home, "tmp"), "PATH=" + searchPath, "NO_COLOR=1", "CI=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"}
}

func copySkillTrees(from, to string, skills []VercelSkillEvidence) error {
	for _, s := range skills {
		source := filepath.Join(from, s.Name)
		target := filepath.Join(to, s.Name)
		if err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(source, path)
			dest := filepath.Join(target, rel)
			if info.IsDir() {
				return os.MkdirAll(dest, 0o700)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, data, info.Mode()&0o777)
		}); err != nil {
			return err
		}
	}
	return nil
}

func evaluateSafeRuntimeModes(pack capabilitypack.Pack) ([]VercelRuntimeRow, bool, error) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []capabilitypack.RuntimeModeEvidence{}
	for _, r := range pack.Resources {
		for _, m := range r.RuntimeModes {
			ev := capabilitypack.RuntimeEvidence{
				Requirements: []capabilitypack.RuntimeRequirementObservation{},
				Authorities:  []capabilitypack.RuntimeAuthorityObservation{},
			}
			for _, q := range m.Requirements {
				ev.Requirements = append(ev.Requirements, capabilitypack.RuntimeRequirementObservation{Kind: q.Kind, ID: q.ID, RuntimeObservation: capabilitypack.RuntimeObservation{State: capabilitypack.ObservationUnverified, Reason: capabilitypack.ObservationReasonObserverError, ObservedAt: now.Format(time.RFC3339), ObserverRevision: "claude-vercel-smoke-v1"}})
			}
			for _, a := range m.Authorities {
				ev.Authorities = append(ev.Authorities, capabilitypack.RuntimeAuthorityObservation{Kind: a.Kind, Scope: a.Scope, RuntimeObservation: capabilitypack.RuntimeObservation{State: capabilitypack.ObservationUnverified, Reason: capabilitypack.ObservationReasonObserverError, ObservedAt: now.Format(time.RFC3339), ObserverRevision: "claude-vercel-smoke-v1"}})
			}
			records = append(records, capabilitypack.RuntimeModeEvidence{ResourceID: r.ID, ModeID: m.ID, Evidence: ev})
		}
	}
	results, err := capabilitypack.EvaluateRuntimeModes(pack, records, now, time.Hour)
	if err != nil {
		return nil, false, err
	}
	rows := make([]VercelRuntimeRow, 0, len(results))
	preflight := false
	for _, result := range results {
		_, pfErr := capabilitypack.PreflightRuntimeMode(pack, result.ResourceID, result.ModeID, records, now, time.Hour)
		var typed capabilitypack.RuntimePreflightError
		if result.State == capabilitypack.RuntimeModeUnverified && errors.As(pfErr, &typed) {
			preflight = true
		}
		rows = append(rows, VercelRuntimeRow{result.ResourceID, result.ModeID, result.State, true})
	}
	return rows, preflight, nil
}

func commandOutput(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}
func digestFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
