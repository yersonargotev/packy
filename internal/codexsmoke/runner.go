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

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/localprojection"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

const ExactFloor = "0.145.0"

type Config struct{ Codex, SearchPath, Version, Integrity, PackyRef, PackySHA, EvidencePath string }
type SkillEvidence struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	SHA256              string `json:"complete_tree_sha256"`
	Invocation          string `json:"invocation"`
	Enabled             bool   `json:"enabled"`
	InvocationAvailable bool   `json:"invocation_available"`
}
type CommandEvidence struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
}
type Evidence struct {
	SchemaVersion          int               `json:"schema_version"`
	PackyRef               string            `json:"packy_ref"`
	PackySHA               string            `json:"packy_sha"`
	VercelFixtureSHA256    string            `json:"vercel_fixture_sha256"`
	CodexVersion           string            `json:"codex_version"`
	CodexNPMIntegrity      string            `json:"codex_npm_integrity"`
	CodexExecutableSHA256  string            `json:"codex_executable_sha256"`
	SandboxRoots           []string          `json:"sandbox_roots"`
	CommandAllowlist       []string          `json:"command_allowlist"`
	Commands               []CommandEvidence `json:"commands"`
	Skills                 []SkillEvidence   `json:"skills"`
	MissingOneNegativeTwin string            `json:"missing_one_negative_twin"`
	NoAuthentication       bool              `json:"no_authentication"`
	NoModelInvocation      bool              `json:"no_model_invocation"`
	NoDeploy               bool              `json:"no_deploy"`
	NoUpstreamExecution    bool              `json:"no_upstream_execution"`
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
	if cfg.Codex == "" || cfg.SearchPath == "" || cfg.Version != ExactFloor || cfg.Integrity == "" || cfg.PackyRef == "" || cfg.PackySHA == "" || cfg.EvidencePath == "" {
		return Evidence{}, errors.New("exact Codex acquisition, Packy identity, and evidence path are required")
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
	adapter := codex.NewSurfaceAdapter(bundle, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "AGENTS.md"))
	actions := make([]capabilitypack.ProjectionAction, 0, 9)
	invocations := map[string]string{}
	for _, r := range fixture.Pack.Resources {
		if r.Kind == "skill" {
			for _, b := range r.Bindings {
				if b.Surface == "codex" {
					invocations[b.Name] = b.Invocation
					actions = append(actions, capabilitypack.ProjectionAction{
						ID:     "skill:" + b.Name,
						Kind:   capabilitypack.ActionSkillLink,
						Source: filepath.Join(bundle, filepath.Clean(r.Source)),
						Target: filepath.Join(home, ".agents", "skills", b.Name),
					})
				}
			}
		}
	}
	if len(actions) != 9 {
		return Evidence{}, fmt.Errorf("expected nine Codex projections, got %d", len(actions))
	}
	if e := adapter.ApplyProjections(ctx, actions); e != nil {
		return Evidence{}, e
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
	for _, a := range actions {
		name := strings.TrimPrefix(a.ID, "skill:")
		s, ok := byName[name]
		if !ok || !s.Enabled {
			return Evidence{}, fmt.Errorf("Codex did not load enabled skill %s", name)
		}
		want := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		source := filepath.Join(a.Source, "SKILL.md")
		if !samePath(s.Path, want) && !samePath(s.Path, source) {
			return Evidence{}, fmt.Errorf("skill %s loaded from %s, want projected target or source", name, s.Path)
		}
		fp, e := localprojection.FingerprintTree(a.Source)
		if e != nil {
			return Evidence{}, e
		}
		skills = append(skills, SkillEvidence{Name: name, Path: s.Path, SHA256: fp, Invocation: invocations[name], Enabled: s.Enabled})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	var promptCommands []CommandEvidence
	for i := range skills {
		command, err := verifyPromptInvocation(ctx, cfg.Codex, cfg.SearchPath, home, work, skills[i].Name, skills[i].Invocation)
		if err != nil {
			return Evidence{}, err
		}
		skills[i].InvocationAvailable = true
		promptCommands = append(promptCommands, command)
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
	commands := []CommandEvidence{cmd1, cmd2}
	commands = append(commands, promptCommands...)
	commands = append(commands, cmd3)
	e := Evidence{1, cfg.PackyRef, cfg.PackySHA, vercelacceptance.ExactArchiveSHA256, strings.TrimSpace(versionOut), cfg.Integrity, digest, []string{"$SANDBOX/home", "$SANDBOX/bundle", "$SANDBOX/work"}, []string{"codex --version", "codex app-server", "codex debug prompt-input"}, commands, sanitizeSkills(skills, sandbox), missing, true, true, true, true}
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
	enc := json.NewEncoder(in)
	_ = enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "packy-codex-smoke", "version": "1"}}})
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
		if json.Unmarshal(scan.Bytes(), &msg) != nil {
			continue
		}
		if msg.ID == 1 {
			_ = enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}})
			_ = enc.Encode(map[string]any{"id": 2, "method": "skills/list", "params": map[string]any{"cwds": []string{cwd}, "forceReload": true}})
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
	_ = in.Close()
	if len(found) == 0 {
		_ = cmd.Process.Kill()
		_, _ = io.Copy(io.Discard, out)
		_ = cmd.Wait()
		return nil, CommandEvidence{}, fmt.Errorf("skills/list returned no skills: %s", stderr.String())
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return found, CommandEvidence{"codex", []string{"app-server", "--stdio"}, 0}, nil
}
func runVersion(ctx context.Context, binary, searchPath, home, cwd string) (string, CommandEvidence, error) {
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "PATH=" + searchPath}
	b, e := cmd.CombinedOutput()
	ce := CommandEvidence{"codex", []string{"--version"}, 0}
	if e != nil {
		ce.ExitCode = 1
	}
	return string(b), ce, e
}

func verifyPromptInvocation(ctx context.Context, binary, searchPath, home, cwd, name, invocation string) (CommandEvidence, error) {
	args := []string{"debug", "prompt-input", invocation}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "PATH=" + searchPath, "NO_COLOR=1"}
	out, err := cmd.Output()
	evidence := CommandEvidence{Name: "codex", Args: args}
	if err != nil {
		evidence.ExitCode = 1
		return evidence, fmt.Errorf("Codex prompt-input for %s: %w", invocation, err)
	}
	if err := validatePromptInput(out, name, invocation); err != nil {
		return evidence, err
	}
	return evidence, nil
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
