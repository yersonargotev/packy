package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestInstallRuntimeStagePlanAddsCommunityToolStepsInSelectionOrder(t *testing.T) {
	runtime := &installRuntime{
		homeDir:      t.TempDir(),
		workspaceDir: "/work/project",
		selection: model.Selection{
			CommunityTools: []model.CommunityToolID{model.CommunityToolCodeGraph},
		},
		resolved: planner.ResolvedPlan{},
		profile:  system.PlatformProfile{},
		state:    &runtimeState{},
	}

	plan := runtime.stagePlan()
	var got []string
	for _, step := range plan.Apply {
		got = append(got, step.ID())
	}
	want := []string{"apply:rollback-restore", "community-tool:codegraph"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("apply step IDs = %#v, want %#v", got, want)
	}
}

func TestCodeGraphGuidanceMarkdownForSDDOnlyWhenSelectedOrConfigured(t *testing.T) {
	tests := []struct {
		name      string
		setupHome func(t *testing.T, home string)
		lookPath  func(string) (string, error)
		selected  []model.CommunityToolID
		want      bool
	}{
		{
			name:     "CLI missing and no selection",
			lookPath: func(string) (string, error) { return "", errors.New("not found") },
		},
		{
			name:     "CLI available but not configured",
			lookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		},
		{
			name:     "selected CodeGraph",
			lookPath: func(string) (string, error) { return "", errors.New("not found") },
			selected: []model.CommunityToolID{model.CommunityToolCodeGraph},
			want:     true,
		},
		{
			name: "configured guidance marker",
			setupHome: func(t *testing.T, home string) {
				mustWriteFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), []byte(strings.Join([]string{
					"existing Claude guidance",
					"<!-- gentle-ai:codegraph-guidance -->",
					"CodeGraph guidance with `codegraph init <project-root>`",
					"<!-- /gentle-ai:codegraph-guidance -->",
				}, "\n")))
			},
			lookPath: func(string) (string, error) { return "/bin/codegraph", nil },
			want:     true,
		},
		{
			name: "configured MCP marker",
			setupHome: func(t *testing.T, home string) {
				mustWriteFile(t, filepath.Join(home, ".codex", "config.toml"), []byte(strings.Join([]string{
					`[mcp_servers.codegraph]`,
					`command = "codegraph"`,
				}, "\n")))
			},
			lookPath: func(string) (string, error) { return "/bin/codegraph", nil },
			want:     true,
		},
		{
			name: "legacy marker with CLI available",
			setupHome: func(t *testing.T, home string) {
				mustWriteFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), []byte(`{}`))
				mustWriteFile(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), []byte(strings.Join([]string{
					"custom notes",
					"<!-- CODEGRAPH_START -->",
					"old CodeGraph instructions",
					"<!-- CODEGRAPH_END -->",
				}, "\n")))
			},
			lookPath: func(string) (string, error) { return "/bin/codegraph", nil },
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			previousLookPath := cmdLookPath
			t.Cleanup(func() { cmdLookPath = previousLookPath })
			cmdLookPath = tc.lookPath

			home := t.TempDir()
			if tc.setupHome != nil {
				tc.setupHome(t, home)
			}

			got := codeGraphGuidanceMarkdownForSDD(home, tc.selected)
			if !tc.want {
				if got != "" {
					t.Fatalf("guidance = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, "codegraph init <project-root>") {
				t.Fatalf("CodeGraph guidance missing search-order rule:\n%s", got)
			}
		})
	}
}

func TestComponentApplyStepInjectsCodeGraphGuidanceWhenCodeGraphSelected(t *testing.T) {
	home := t.TempDir()
	withCodeGraphLookPath(t, func(string) (string, error) { return "", errors.New("not found") })

	step := componentApplyStep{
		id:           "apply:sdd",
		component:    model.ComponentSDD,
		homeDir:      home,
		workspaceDir: "/work/project",
		scope:        ScopeGlobal,
		agents:       []model.AgentID{model.AgentOpenCode},
		selection: model.Selection{
			CommunityTools: []model.CommunityToolID{model.CommunityToolCodeGraph},
			SDDMode:        model.SDDModeMulti,
		},
	}
	if err := step.Run(); err != nil {
		t.Fatalf("componentApplyStep.Run() error = %v", err)
	}

	assertOpenCodeSharedPromptCodeGraphGuidance(t, home, true)
}

func TestComponentApplyStepInjectsCodeGraphGuidanceWhenCodeGraphConfigured(t *testing.T) {
	home := t.TempDir()
	withCodeGraphLookPath(t, func(string) (string, error) { return "/bin/codegraph", nil })
	mustWriteFile(t, filepath.Join(home, ".codex", "config.toml"), []byte(strings.Join([]string{
		`[mcp_servers.codegraph]`,
		`command = "codegraph"`,
	}, "\n")))
	mustWriteFile(t, filepath.Join(home, ".codex", "AGENTS.md"), []byte(strings.Join([]string{
		"existing Codex guidance",
		"<!-- gentle-ai:codegraph-guidance -->",
		"CodeGraph guidance with `codegraph init <project-root>`",
		"<!-- /gentle-ai:codegraph-guidance -->",
	}, "\n")))

	step := componentApplyStep{
		id:           "apply:sdd",
		component:    model.ComponentSDD,
		homeDir:      home,
		workspaceDir: "/work/project",
		scope:        ScopeGlobal,
		agents:       []model.AgentID{model.AgentOpenCode},
		selection:    model.Selection{SDDMode: model.SDDModeMulti},
	}
	if err := step.Run(); err != nil {
		t.Fatalf("componentApplyStep.Run() error = %v", err)
	}

	assertOpenCodeSharedPromptCodeGraphGuidance(t, home, true)
}

func TestComponentApplyStepOmitsCodeGraphGuidanceWhenOnlyCLIAvailable(t *testing.T) {
	home := t.TempDir()
	withCodeGraphLookPath(t, func(string) (string, error) { return "/bin/codegraph", nil })

	step := componentApplyStep{
		id:           "apply:sdd",
		component:    model.ComponentSDD,
		homeDir:      home,
		workspaceDir: "/work/project",
		scope:        ScopeGlobal,
		agents:       []model.AgentID{model.AgentOpenCode},
		selection:    model.Selection{SDDMode: model.SDDModeMulti},
	}
	if err := step.Run(); err != nil {
		t.Fatalf("componentApplyStep.Run() error = %v", err)
	}

	assertOpenCodeSharedPromptCodeGraphGuidance(t, home, false)
}

func TestComponentSyncStepInjectsCodeGraphGuidanceFromLegacyMarker(t *testing.T) {
	home := t.TempDir()
	withCodeGraphLookPath(t, func(string) (string, error) { return "/bin/codegraph", nil })
	mustWriteFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), []byte(`{}`))
	mustWriteFile(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), []byte(strings.Join([]string{
		"custom notes",
		"<!-- CODEGRAPH_START -->",
		"old CodeGraph instructions",
		"<!-- CODEGRAPH_END -->",
	}, "\n")))

	var changed []string
	step := componentSyncStep{
		id:           "sync:sdd",
		component:    model.ComponentSDD,
		homeDir:      home,
		workspaceDir: "/work/project",
		agents:       []model.AgentID{model.AgentOpenCode},
		selection:    model.Selection{SDDMode: model.SDDModeMulti},
		changedFiles: &changed,
	}
	if err := step.Run(); err != nil {
		t.Fatalf("componentSyncStep.Run() error = %v", err)
	}

	assertOpenCodeSharedPromptCodeGraphGuidance(t, home, true)
}

func TestCommunityToolInstallStepUsesInjectableInstaller(t *testing.T) {
	previousInstall := installCommunityTool
	previousRunCommand := runCommand
	t.Cleanup(func() {
		installCommunityTool = previousInstall
		runCommand = previousRunCommand
	})

	runCommand = func(string, ...string) error {
		t.Fatal("communityToolInstallStep should not call real command runner when installer is injected")
		return nil
	}

	var gotTool model.CommunityToolID
	var gotWorkspace string
	var runner communitytool.Runner
	installCommunityTool = func(tool model.CommunityToolID, workspaceDir string, r communitytool.Runner) (communitytool.Result, error) {
		gotTool = tool
		gotWorkspace = workspaceDir
		runner = r
		return communitytool.Result{Tool: tool}, nil
	}

	step := communityToolInstallStep{id: "community-tool:codegraph", tool: model.CommunityToolCodeGraph, workspaceDir: "/work/project"}
	if err := step.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotTool != model.CommunityToolCodeGraph || gotWorkspace != "/work/project" || runner == nil {
		t.Fatalf("installer args = (%q, %q, %#v), want CodeGraph, workspace, runner", gotTool, gotWorkspace, runner)
	}
}

func withCodeGraphLookPath(t *testing.T, lookPath func(string) (string, error)) {
	t.Helper()
	previousLookPath := cmdLookPath
	t.Cleanup(func() { cmdLookPath = previousLookPath })
	cmdLookPath = func(name string) (string, error) {
		if name != "codegraph" {
			return "", errors.New("not found")
		}
		return lookPath(name)
	}
}

func assertOpenCodeSharedPromptCodeGraphGuidance(t *testing.T, home string, want bool) {
	t.Helper()
	promptPath := filepath.Join(home, ".config", "opencode", "prompts", "sdd", "sdd-apply.md")
	content, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, err)
	}
	text := string(content)
	hasGuidance := strings.Contains(text, "<!-- gentle-ai:codegraph-guidance -->") && strings.Contains(text, "codegraph init <project-root>")
	if hasGuidance != want {
		t.Fatalf("CodeGraph guidance present = %v, want %v in %s", hasGuidance, want, promptPath)
	}
}
