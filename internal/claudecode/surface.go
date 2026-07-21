package claudecode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const (
	ActionSkillLink               capabilitypack.ProjectionActionKind = "claude-skill-link"
	ActionInstructionContribution capabilitypack.ProjectionActionKind = "claude-instruction-contribution"
	ActionAgentFile               capabilitypack.ProjectionActionKind = "claude-agent-file"
	ActionCommandHook             capabilitypack.ProjectionActionKind = "claude-command-hook"
	ActionUserMCP                 capabilitypack.ProjectionActionKind = "claude-user-mcp"
)

type SurfaceAdapter struct {
	layout                            CanonicalLayout
	bundleRoot, stateRoot, executable string
	runner                            Runner
	ownership                         OwnershipSnapshot
}

func NewSurfaceAdapter(bundleRoot string, layout CanonicalLayout, stateRoot, executable string, runner Runner, ownership OwnershipSnapshot) *SurfaceAdapter {
	return &SurfaceAdapter{bundleRoot: bundleRoot, layout: layout, stateRoot: stateRoot, executable: executable, runner: runner, ownership: ownership}
}

func (a *SurfaceAdapter) InspectSurface(_ context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	pack := transition.Desired
	if pack.ID == "" {
		pack = transition.Prior
	}
	var result capabilitypack.SurfaceInspection
	revision := []string{}
	for _, r := range pack.Resources {
		b, ok := claudeBinding(r)
		if !ok {
			continue
		}
		id := r.Kind + ":" + b.Name
		switch b.Projection {
		case "skill":
			source := filepath.Join(a.bundleRoot, filepath.Clean(r.Source))
			desired, err := localprojection.FingerprintTree(source)
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return result, err
			}
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Source: source, Target: target, Description: "link Claude Code skill " + b.Name}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, ExternallyManaged: exists && !a.ownsExact(id, observed), Action: action})
			revision = append(revision, id+observed)
		case "instruction":
			content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			current, _ := os.ReadFile(a.layout.InstructionsFile)
			merged, err := UpsertInstructionContribution(string(current), InstructionContribution{ContributorID: "pack:" + pack.ID + ":" + r.ID, Content: string(content)})
			if err != nil {
				return result, err
			}
			desired := Fingerprint([]byte(strings.TrimSpace(string(content))))
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Content: merged, Command: Fingerprint(current), Description: "merge Claude Code instruction " + r.ID}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: strings.Contains(string(current), "contributor:pack:"+pack.ID+":"+r.ID), ObservedFingerprint: Fingerprint(current), DesiredFingerprint: desired, Action: action})
			revision = append(revision, "instructions="+Fingerprint(current))
		case "agent":
			content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.AgentsDir, b.Name+".md")
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return result, err
			}
			desired := Fingerprint(content)
			current, _ := os.ReadFile(target)
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, ExternallyManaged: exists && !a.ownsExact(id, observed), Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionAgentFile, Target: target, Content: string(content), Command: Fingerprint(current), Description: "write Claude Code agent " + b.Name}})
			revision = append(revision, id+observed)
		case "command_hook":
			if b.Hook == nil {
				return result, errors.New("Claude command hook is missing typed definition")
			}
			hook := fromBindingHook(b)
			settings, _ := os.ReadFile(a.layout.SettingsFile)
			merged, err := MergeCommandHook(settings, hook, false)
			if err != nil {
				return result, err
			}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: false, ObservedFingerprint: Fingerprint(settings), DesiredFingerprint: hook.Fingerprint(), Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionCommandHook, Target: a.layout.SettingsFile, Content: string(merged), Command: Fingerprint(settings), Description: "merge Claude Code command hook " + b.Name}})
			revision = append(revision, "settings="+Fingerprint(settings))
		case "mcp_server":
			o := ObserveUserMCP(a.layout.UserMCPFile, b.Name)
			identity := NewMCPIdentity(b.Name, r.Command, r.Args, map[string]string{})
			args := []string{"mcp", "add", b.Name, "--scope", "user", "--", r.Command}
			args = append(args, r.Args...)
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: o.Present, ObservedFingerprint: o.DefinitionFingerprint, DesiredFingerprint: canonicalFingerprint(identity), Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionUserMCP, Command: a.executable, Args: args, Description: "configure redacted Claude Code user MCP " + b.Name}})
			revision = append(revision, id+o.DefinitionFingerprint)
		}
	}
	sort.Strings(revision)
	result.Revision = Fingerprint([]byte(strings.Join(revision, "\n")))
	result.Readiness = capabilitypack.ReadinessObservation{AuthorizationObserved: true, PendingHumanActions: []string{"supply explicit Claude Code runtime loading evidence"}, Evidence: []string{"filesystem and static user MCP definitions inspected; runtime use was not invoked"}}
	return result, nil
}

func (a *SurfaceAdapter) ownsExact(id, fingerprint string) bool {
	matches := 0
	for _, r := range a.ownership.Records {
		if r.ID == id && fingerprintsEqual(r.Fingerprint, fingerprint) {
			matches++
		}
	}
	return matches == 1
}

func (a *SurfaceAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	if err := validateActions(actions, a); err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	unlock, err := a.lock()
	if err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	defer unlock()
	appliedShared := map[string]string{}
	for _, action := range actions {
		if action.Kind == ActionInstructionContribution || action.Kind == ActionCommandHook {
			if content, ok := appliedShared[action.Target]; ok && content == action.Content {
				continue
			}
			appliedShared[action.Target] = action.Content
		}
		if err := a.apply(ctx, action); err != nil {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
		}
	}
	return nil
}
func validateActions(actions []capabilitypack.ProjectionAction, a *SurfaceAdapter) error {
	seen := map[string]bool{}
	targets := map[string]capabilitypack.ProjectionAction{}
	for _, x := range actions {
		if x.ID == "" || seen[x.ID] {
			return errors.New("duplicate or empty Claude projection identity")
		}
		seen[x.ID] = true
		switch x.Kind {
		case ActionSkillLink:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode != capabilitypack.ProjectionDeleteTarget {
				info, err := os.Stat(x.Source)
				if err != nil || !info.IsDir() {
					return errors.New("Claude skill source must be an existing directory")
				}
				if !filepath.IsAbs(x.Source) {
					return errors.New("Claude skill source must be absolute")
				}
			}
			if _, ok := targets[x.Target]; ok {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionAgentFile:
			if !directChild(x.Target, a.layout.AgentsDir) || filepath.Ext(x.Target) != ".md" {
				return errors.New("Claude agent target must be one Markdown file in the canonical agents directory")
			}
			if _, ok := targets[x.Target]; ok {
				return errors.New("overlapping exclusive Claude agent target")
			}
			targets[x.Target] = x
		case ActionInstructionContribution:
			if filepath.Clean(x.Target) != filepath.Clean(a.layout.InstructionsFile) {
				return errors.New("Claude instruction target must be canonical CLAUDE.md")
			}
			if prior, ok := targets[x.Target]; ok && prior.Content != x.Content {
				return errors.New("shared Claude instruction actions must be aggregated into one sealed document")
			}
			targets[x.Target] = x
		case ActionCommandHook:
			if filepath.Clean(x.Target) != filepath.Clean(a.layout.SettingsFile) {
				return errors.New("Claude hook target must be canonical settings.json")
			}
			if prior, ok := targets[x.Target]; ok && prior.Content != x.Content {
				return errors.New("shared Claude hook actions must be aggregated into one sealed document")
			}
			targets[x.Target] = x
		case ActionUserMCP:
			if x.Command == "" || a.runner == nil {
				return errors.New("Claude executable is required for user MCP effect")
			}
			name, remove := mcpActionName(x.Args)
			if name == "" || !hasUserScope(x.Args) {
				return errors.New("Claude user MCP action must be an official named add/remove with --scope user")
			}
			if remove && x.Mode != capabilitypack.ProjectionDeleteTarget {
				return errors.New("Claude user MCP removal must be sealed as delete-target")
			}
		default:
			return fmt.Errorf("unsealed Claude projection action kind %q", x.Kind)
		}
	}
	return nil
}
func (a *SurfaceAdapter) apply(ctx context.Context, x capabilitypack.ProjectionAction) error {
	if err := a.validateFreshOwnership(x); err != nil {
		return err
	}
	if x.Mode == capabilitypack.ProjectionDeleteTarget && x.Kind != ActionUserMCP {
		if _, err := os.Lstat(x.Target); os.IsNotExist(err) {
			return nil
		}
	}
	if x.Kind == ActionInstructionContribution || x.Kind == ActionAgentFile || x.Kind == ActionCommandHook {
		current, err := os.ReadFile(x.Target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if x.Command != "" && Fingerprint(current) != x.Command {
			return errors.New("stale Claude shared document revision")
		}
	}
	switch x.Kind {
	case ActionSkillLink:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		return replaceSymlink(x.Source, x.Target)
	case ActionAgentFile, ActionInstructionContribution, ActionCommandHook:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		return atomicWrite(x.Target, []byte(x.Content), 0644)
	case ActionUserMCP:
		r := a.runner.Run(ctx, Command{Executable: x.Command, Args: append([]string(nil), x.Args...), Timeout: 15_000_000_000, Description: x.Description})
		if r.TimedOut {
			return context.DeadlineExceeded
		}
		if r.Err != nil {
			return errors.New("Claude Code user MCP command failed")
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("Claude Code user MCP command failed with status %d", r.ExitCode)
		}
		name, remove := mcpActionName(x.Args)
		observed := ObserveUserMCP(a.layout.UserMCPFile, name)
		if observed.Err != nil {
			return errors.New("Claude Code user MCP verification failed")
		}
		if remove && observed.Present {
			return errors.New("Claude Code user MCP removal did not converge")
		}
		if !remove && !observed.Present {
			return errors.New("Claude Code user MCP addition did not converge")
		}
		if !remove && x.Content != "" && observed.DefinitionFingerprint != x.Content {
			return errors.New("Claude Code user MCP definition did not match the sealed action")
		}
		return nil
	}
	return nil
}

func (a *SurfaceAdapter) validateFreshOwnership(x capabilitypack.ProjectionAction) error {
	if a.ownership.Revision != "" && a.ownership.Revision != canonicalFingerprint(a.ownership.Records) {
		return errors.New("stale composite Claude ownership snapshot")
	}
	var record *OwnershipRecord
	ambiguous := false
	for i := range a.ownership.Records {
		r := &a.ownership.Records[i]
		if r.ID == x.ID {
			if record != nil {
				ambiguous = true
			}
			record = r
		}
	}
	if ambiguous {
		return errors.New("ambiguous Claude ownership")
	}
	if x.Kind == ActionUserMCP {
		if x.Mode == capabilitypack.ProjectionDeleteTarget && (record == nil || !record.DeletionAuthorized || len(record.Contributors) > 1) {
			return errors.New("Claude user MCP deletion is not authorized by the fresh composite ownership snapshot")
		}
		return nil
	}
	fp, exists, err := localprojection.FingerprintPath(x.Target)
	if err != nil {
		return err
	}
	if x.Mode == capabilitypack.ProjectionDeleteTarget {
		if !exists {
			return nil
		}
		if record == nil || !record.DeletionAuthorized || len(record.Contributors) > 1 {
			return errors.New("Claude deletion is not authorized by the fresh composite ownership snapshot")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
		if x.Kind == ActionInstructionContribution || x.Kind == ActionCommandHook {
			return errors.New("shared Claude documents cannot be deleted as exclusive targets")
		}
		return nil
	}
	if exists && (x.Kind == ActionSkillLink || x.Kind == ActionAgentFile) {
		if record == nil {
			return errors.New("foreign Claude projection collision")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
	}
	return nil
}
func fingerprintsEqual(a, b string) bool { return a == b || "sha256:"+a == b || a == "sha256:"+b }
func directChild(path, root string) bool {
	return filepath.Dir(filepath.Clean(path)) == filepath.Clean(root) && filepath.Base(path) != "." && filepath.Base(path) != ".."
}
func mcpActionName(args []string) (string, bool) {
	if len(args) >= 3 && args[0] == "mcp" && (args[1] == "add" || args[1] == "remove") {
		return args[2], args[1] == "remove"
	}
	return "", false
}
func hasUserScope(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--scope" && args[i+1] == "user" {
			return true
		}
	}
	return false
}
func (a *SurfaceAdapter) lock() (func(), error) {
	if a.stateRoot == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(a.stateRoot, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(a.stateRoot, "claude-host-effect.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire Claude host-effect lock: %w", err)
	}
	f.Close()
	return func() { _ = os.Remove(p) }, nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".packy-claude-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func replaceSymlink(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp := target + ".packy-new"
	_ = os.Remove(tmp)
	if err := os.Symlink(source, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return os.Rename(tmp, target)
}
func removeExact(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func firstID(a []capabilitypack.ProjectionAction) string {
	if len(a) > 0 {
		return a[0].ID
	}
	return "batch"
}
func claudeBinding(r capabilitypack.Resource) (capabilitypack.Binding, bool) {
	for _, b := range r.Bindings {
		if b.Surface == capabilitypack.SurfaceClaude {
			return b, true
		}
	}
	return capabilitypack.Binding{}, false
}
func fromBindingHook(b capabilitypack.Binding) CommandHookEntry {
	h := b.Hook
	return CommandHookEntry{Type: h.Type, Event: h.Event, Matcher: h.Matcher, Command: h.Command, Args: append([]string(nil), h.Args...), TimeoutSeconds: h.TimeoutSeconds, Blocking: h.Blocking, Failure: h.Failure, Authorities: append([]string(nil), h.Authorities...)}
}
