package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/matty/internal/corelifecycle"
)

type ActionKind string

const (
	ActionWriteFile           ActionKind = "write-file"
	ActionWriteCodexPrompt    ActionKind = "write-codex-prompt"
	ActionWriteOpenCodePrompt ActionKind = "write-opencode-prompt"
	ActionSymlink             ActionKind = "symlink"
	ActionRun                 ActionKind = "run"
	ActionSkip                ActionKind = "skip"
)

func (action PlannedAction) printDetail(w io.Writer) error {
	switch action.Kind {
	case ActionWriteOpenCodePrompt, ActionSymlink:
		_, err := fmt.Fprintf(w, " (%s -> %s)\n", action.Path, action.Target)
		return err
	case ActionRun:
		cmd := strings.Join(append([]string{action.Command}, action.Args...), " ")
		_, err := fmt.Fprintf(w, " (%s)\n", cmd)
		return err
	case ActionWriteFile, ActionWriteCodexPrompt, ActionSkip:
		_, err := fmt.Fprintf(w, " (%s)\n", action.Path)
		return err
	default:
		_, err := fmt.Fprintln(w)
		return err
	}
}

// PlannedAction is a human-reportable unit of work. Issue 02 introduced the
// planning model; later issues add concrete installers behind this seam.
type PlannedAction struct {
	Kind        ActionKind
	Path        string
	Target      string
	Command     string
	Args        []string
	Description string
	skipReason  skillLinkStatus
}

type Plan struct {
	Actions []PlannedAction
	State   corelifecycle.State
}

func buildDoctorExpectedSkillPlan(paths Paths) (Plan, error) {
	discovered, err := DiscoverManagedSkills(paths)
	if err != nil {
		return Plan{}, err
	}

	var actions []PlannedAction
	managed := make([]corelifecycle.ManagedSkill, 0, len(discovered))
	for _, skill := range discovered {
		status, err := plannedSkillLinkAction(skill)
		if err != nil {
			return Plan{}, err
		}
		if status.Kind != "" {
			actions = append(actions, status)
		}
		if status.Kind != ActionSkip {
			managed = append(managed, skill)
		}
	}
	return Plan{Actions: actions, State: corelifecycle.State{ManagedSkills: managed}}, nil
}

func plannedSkillLinkAction(skill corelifecycle.ManagedSkill) (PlannedAction, error) {
	link, err := inspectSkillLink(skill)
	if err != nil {
		return PlannedAction{}, err
	}
	behavior, ok := skillLinkBehaviors[link.status]
	if !ok {
		return PlannedAction{}, fmt.Errorf("inspect skill link %s: unknown status %s", skill.LinkPath, link.status)
	}
	return behavior.plannedAction(skill, link), nil
}

type skillLinkStatus string

const (
	skillLinkMissing          skillLinkStatus = "missing"
	skillLinkManaged          skillLinkStatus = "managed"
	skillLinkUnmanagedPath    skillLinkStatus = "unmanaged-path"
	skillLinkUnmanagedSymlink skillLinkStatus = "unmanaged-symlink"
)

type skillLinkInspection struct {
	status skillLinkStatus
	target string
}

type skillLinkDoctorProblem struct {
	missing bool
	detail  string
}

type skillLinkBehavior struct {
	plannedAction func(corelifecycle.ManagedSkill, skillLinkInspection) PlannedAction
	doctorProblem func(corelifecycle.ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool)
}

var skillLinkBehaviors = map[skillLinkStatus]skillLinkBehavior{
	skillLinkMissing: {
		plannedAction: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{missing: true, detail: skill.Name}, true
		},
	},
	skillLinkManaged: {
		plannedAction: func(corelifecycle.ManagedSkill, skillLinkInspection) PlannedAction { return PlannedAction{} },
		doctorProblem: func(corelifecycle.ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{}, false
		},
	},
	skillLinkUnmanagedPath: {
		plannedAction: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name, skipReason: skillLinkUnmanagedPath}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name + " is not a symlink"}, true
		},
	},
	skillLinkUnmanagedSymlink: {
		plannedAction: func(skill corelifecycle.ManagedSkill, link skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: link.target, Description: "preserve unmanaged symlink for skill " + skill.Name, skipReason: skillLinkUnmanagedSymlink}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name}, true
		},
	},
}

func inspectSkillLink(skill corelifecycle.ManagedSkill) (skillLinkInspection, error) {
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillLinkInspection{status: skillLinkMissing}, nil
		}
		return skillLinkInspection{}, fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return skillLinkInspection{status: skillLinkUnmanagedPath}, nil
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		return skillLinkInspection{}, fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
	}
	if sameSymlinkTarget(skill.LinkPath, target, skill.SourcePath) {
		return skillLinkInspection{status: skillLinkManaged, target: target}, nil
	}
	return skillLinkInspection{status: skillLinkUnmanagedSymlink, target: target}, nil
}

func sameSymlinkTarget(linkPath, gotTarget, wantTarget string) bool {
	if gotTarget == wantTarget {
		return true
	}
	if !filepath.IsAbs(gotTarget) {
		gotTarget = filepath.Join(filepath.Dir(linkPath), gotTarget)
	}
	gotAbs, gotErr := filepath.Abs(gotTarget)
	wantAbs, wantErr := filepath.Abs(wantTarget)
	return gotErr == nil && wantErr == nil && gotAbs == wantAbs
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func PrintPlan(w io.Writer, plan Plan) error {
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(w, "- %s: %s", action.Kind, action.Description); err != nil {
			return err
		}
		if err := action.printDetail(w); err != nil {
			return err
		}
	}
	return nil
}

type unmanagedSymlinkSummary struct {
	count   int
	example PlannedAction
}

func unmanagedSymlinkSkipSummary(plan Plan) (unmanagedSymlinkSummary, bool) {
	var summary unmanagedSymlinkSummary
	skipped := 0
	for _, action := range plan.Actions {
		if action.Kind != ActionSkip {
			continue
		}
		skipped++
		if action.skipReason == skillLinkUnmanagedSymlink {
			if summary.count == 0 {
				summary.example = action
			}
			summary.count++
		}
	}
	if summary.count == 0 {
		return unmanagedSymlinkSummary{}, false
	}
	expectedSkillLinks := len(plan.State.ManagedSkills) + skipped
	if !isMostExpectedSkillLinks(summary.count, expectedSkillLinks) {
		return unmanagedSymlinkSummary{}, false
	}
	return summary, true
}

func isMostExpectedSkillLinks(count, expectedSkillLinks int) bool {
	return expectedSkillLinks > 0 && count*2 > expectedSkillLinks
}

func unmanagedSymlinkRecoveryAdvice() string {
	return "Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links."
}
