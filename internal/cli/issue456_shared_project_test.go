package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue456CodexAndOpenCodeShareProjectSkillsByExplicitContribution(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	firstOut, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex")
	if err != nil {
		out := firstOut
		t.Fatalf("install Codex: %v\n%s", err, out)
	}
	if !strings.Contains(firstOut, "Incidentally discoverable by opencode; no installation or activation intent recorded") {
		t.Fatalf("Codex install did not disclose incidental OpenCode discovery:\n%s", firstOut)
	}
	skill := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	before, err := os.Stat(skill)
	if err != nil {
		t.Fatal(err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "opencode"); err != nil {
		t.Fatalf("install OpenCode: %v\n%s", err, out)
	}
	after, err := os.Stat(skill)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("adding the OpenCode contributor rewrote an already-correct shared skill")
	}
	if _, err := os.Lstat(filepath.Join(project, ".opencode", "skills", "ask-matt")); !os.IsNotExist(err) {
		t.Fatalf("OpenCode received a duplicate skill tree: %v", err)
	}

	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installation.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit installed surfaces = %v, want %v", got, want)
	}
	for _, projection := range installation.Lock.Projections {
		if projection.Resource.Kind == "skill" && (!containsString(projection.Contributors, "surface:codex:pack:matty") || !containsString(projection.Contributors, "surface:opencode:pack:matty")) {
			t.Fatalf("shared projection %s does not record both surface contributors: %v", projection.Resource, projection.Contributors)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--project", "--json")
	if err != nil {
		t.Fatalf("project status: %v\n%s", err, out)
	}
	var status capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(out), &status); err != nil || len(status.Packs) != 2 {
		t.Fatalf("status = %#v, err=%v\n%s", status, err, out)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("uninstall Codex contributor: %v\n%s", err, out)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("shared skill was removed while OpenCode still contributes: %v", err)
	}
	installation, err = capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installation.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceOpenCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining installed surfaces = %v, want %v", got, want)
	}
}

func TestIssue456DivergentSharedAliasesBlockWithoutDuplicateProjection(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--alias", "skill:ask-matt=shared-ask"); err != nil {
		t.Fatalf("install aliased Codex projection: %v\n%s", err, out)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "opencode", "--alias", "skill:ask-matt=other-ask")
	if err == nil || !strings.Contains(out, "divergent_shared_alias") {
		t.Fatalf("divergent alias was not blocked: %v\n%s", err, out)
	}
	if snapshotTree(t, project) != before || exists(filepath.Join(project, ".agents", "skills", "other-ask")) {
		t.Fatal("blocked divergent alias mutated the project or created a duplicate projection")
	}
}

func TestIssue456KeepsSelectionAndAliasesPerSurface(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--resource", "skill:ask-matt", "--alias", "skill:ask-matt=codex-ask"); err != nil {
		t.Fatalf("install Codex selection: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "opencode", "--resource", "skill:code-review", "--alias", "skill:code-review=opencode-review"); err != nil {
		t.Fatalf("install OpenCode selection: %v\n%s", err, out)
	}

	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	pack := installation.Manifest.Packs[0]
	if len(pack.SurfaceIntents) != 2 {
		t.Fatalf("surface intents = %#v, want one exact intent per surface", pack.SurfaceIntents)
	}
	if got, want := pack.SurfaceIntents[0].Selection.Roots, []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "ask-matt"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex roots = %v, want %v", got, want)
	}
	if got, want := pack.SurfaceIntents[1].Selection.Roots, []capabilitypack.ResourceIdentity{{Kind: "skill", ID: "code-review"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenCode roots = %v, want %v", got, want)
	}
	for _, relative := range []string{".agents/skills/codex-ask/SKILL.md", ".agents/skills/opencode-review/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("missing independently aliased project skill %s: %v", relative, err)
		}
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty", "--surface", "opencode"); err != nil {
		t.Fatalf("uninstall OpenCode selection: %v\n%s", err, out)
	}
	if _, err := capabilitypack.LoadProjectInstallation(project); err != nil {
		t.Fatalf("remaining Codex contract is invalid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "codex-ask", "SKILL.md")); err != nil {
		t.Fatalf("Codex selection was not preserved: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".agents", "skills", "opencode-review")); !os.IsNotExist(err) {
		t.Fatalf("OpenCode-only selection was retained: %v", err)
	}
}

func TestIssue456RemovingOpenCodePreservesCodexSharedSkills(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", surface); err != nil {
			t.Fatalf("install %s: %v\n%s", surface, err, out)
		}
	}
	skill := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty", "--surface", "opencode"); err != nil {
		t.Fatalf("uninstall OpenCode contributor: %v\n%s", err, out)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("shared skill was removed while Codex still contributes: %v", err)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installation.Manifest.Packs[0].Surfaces, []capabilitypack.Surface{capabilitypack.SurfaceCodex}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining installed surfaces = %v, want %v", got, want)
	}
}

func TestIssue456OpenCodeInstallsCompleteNativeProjectResources(t *testing.T) {
	for _, test := range []struct {
		pack  string
		paths []string
	}{
		{pack: "addy", paths: []string{".agents/skills/using-agent-skills/SKILL.md", ".opencode/agents/code-reviewer.md", ".opencode/commands/build.md", ".opencode/assets/definition-of-done/definition-of-done.md"}},
	} {
		t.Run(test.pack, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, _, _ := packActivationOptions(t, terminal)
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts.Getwd = func() (string, error) { return project, nil }
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", test.pack, "--surface", "opencode"); err != nil {
				t.Fatalf("install %s for OpenCode: %v\n%s", test.pack, err, out)
			}
			for _, relative := range test.paths {
				if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
					t.Fatalf("missing OpenCode project projection %s: %v", relative, err)
				}
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", test.pack, "--surface", "opencode", "--project", "--require", "installed"); err != nil {
				t.Fatalf("OpenCode project status: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", test.pack, "--surface", "opencode"); err != nil {
				t.Fatalf("OpenCode project uninstall: %v\n%s", err, out)
			}
			for _, relative := range test.paths {
				if _, err := os.Lstat(filepath.Join(project, relative)); !os.IsNotExist(err) {
					t.Fatalf("OpenCode project uninstall retained %s: %v", relative, err)
				}
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
