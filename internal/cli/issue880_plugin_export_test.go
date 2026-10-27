package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue880ClaudePluginExportExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	claudeContracts := func(m map[string]any) {
		for _, value := range m["resources"].([]any) {
			r := value.(map[string]any)
			if r["kind"] != "skill" || r["id"] == "native-only" {
				continue
			}
			name := r["id"].(string)
			c := pluginExportContract(name)
			c["format"] = "claude-plugin"
			if name == "guide" {
				c["source"] = "skills/plugin-guide"
				c["references"] = []any{
					map[string]any{"id": "asset", "file": "SKILL.md", "target": "asset:guide-data", "kind": "path"},
					map[string]any{"id": "helper-call", "file": "SKILL.md", "target": "skill:helper", "kind": "invocation", "value": "/reviewed:helper"},
					map[string]any{"id": "helper-name", "file": "SKILL.md", "target": "skill:helper", "kind": "name"},
					map[string]any{"id": "helper-path", "file": "SKILL.md", "target": "skill:helper", "kind": "path"},
					map[string]any{"id": "notice", "file": "SKILL.md", "target": "notice:mit", "kind": "path"},
				}
				for _, v := range r["variants"].([]any) {
					x := v.(map[string]any)
					if x["surface"] == "claude" {
						x["requires"] = []string{"asset:guide-data", "skill:helper"}
						x["plugin_exports"] = []any{c}
					}
				}
			} else {
				r["plugin_exports"] = append(r["plugin_exports"].([]any), c)
			}
		}
	}
	project := pluginExportProject(t, claudeContracts)
	entry := filepath.Join(project, "packs/reviewed/skills/plugin-guide/SKILL.md")
	content, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	content = bytes.Replace(content, []byte("allowed-tools: Read\n"), []byte("allowed-tools: [Read, Grep]\ndisable-model-invocation: true\nargument-hint: '[issue]'\n"), 1)
	if err := os.WriteFile(entry, content, 0644); err != nil {
		t.Fatal(err)
	}
	assets := pluginExportAssets(t, project, strings.Repeat("a", 40))
	output := filepath.Join(t.TempDir(), "plugin")
	out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--skill", "guide", "--output", output, "--json")
	if err != nil {
		t.Fatalf("export: %v %s %s", err, out, stderr)
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	assertStructuredOutput(t, repo, "plugin-export.schema.json", string(out))
	discovery, discoveryErr, e := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "show", "reviewed", "--surface", "claude", "--json")
	if e != nil {
		t.Fatalf("discovery: %v %s %s", e, discovery, discoveryErr)
	}
	assertStructuredOutput(t, repo, "skill-discovery.schema.json", string(discovery))
	if !bytes.Contains(discovery, []byte(`"format":"claude-plugin"`)) {
		t.Fatalf("missing reviewed Claude compatibility: %s", discovery)
	}

	if !bytes.Contains(out, []byte(`"format":"claude-plugin"`)) {
		t.Fatalf("wrong format report: %s", out)
	}
	failed := filepath.Join(t.TempDir(), "incompatible")
	failOut, _, failErr := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--skill", "native-only", "--output", failed, "--json")
	if failErr == nil {
		t.Fatalf("unreviewed skill exported: %s", failOut)
	}
	assertStructuredOutput(t, repo, "plugin-export.schema.json", string(failOut))
	if _, err := os.Lstat(failed); !os.IsNotExist(err) {
		t.Fatal("failure published output")
	}
	// Default export and repeated output include exactly the reviewed skills.
	defaultOutput := filepath.Join(t.TempDir(), "default")
	defaultOut, defaultErrOut, defaultErr := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--output", defaultOutput, "--json")
	if defaultErr != nil {
		t.Fatalf("default export: %v %s %s", defaultErr, defaultOut, defaultErrOut)
	}
	assertStructuredOutput(t, repo, "plugin-export.schema.json", string(defaultOut))
	if !bytes.Contains(defaultOut, []byte(`"selected_skills":["guide","helper","leaf"]`)) {
		t.Fatalf("default selection: %s", defaultOut)
	}
	repeated := filepath.Join(t.TempDir(), "repeated")
	_, errOut, repeatErr := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--skill", "guide", "--output", repeated, "--json")
	if repeatErr != nil || pluginPackageBytesAndModes(t, output) != pluginPackageBytesAndModes(t, repeated) {
		t.Fatalf("repeat export: %v %s", repeatErr, errOut)
	}
	for _, fixture := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"incompatible dependency", func(m map[string]any) {
			for _, v := range m["resources"].([]any) {
				r := v.(map[string]any)
				if r["kind"] == "skill" && r["id"] == "helper" {
					r["plugin_exports"] = []any{pluginExportContract("helper")}
				}
			}
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			p := pluginExportProject(t, func(m map[string]any) { claudeContracts(m); fixture.edit(m) })
			a := pluginExportAssets(t, p, strings.Repeat("b", 40))
			destination := filepath.Join(t.TempDir(), "plugin")
			result, _, e := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", a, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--skill", "guide", "--output", destination, "--json")
			if e == nil || !bytes.Contains(result, []byte("required skill")) {
				t.Fatalf("incompatible dependency succeeded: %v %s", e, result)
			}
			if _, e := os.Lstat(destination); !os.IsNotExist(e) {
				t.Fatal("failure published output")
			}
		})
	}
	for _, resolved := range []bool{false, true} {
		t.Run(fmt.Sprintf("role-resolved-%v", resolved), func(t *testing.T) {
			p := pluginExportProject(t, func(m map[string]any) {
				claudeContracts(m)
				resources := m["resources"].([]any)
				for _, v := range resources {
					r := v.(map[string]any)
					if r["kind"] == "skill" && r["id"] == "guide" {
						for _, vv := range r["variants"].([]any) {
							variant := vv.(map[string]any)
							if variant["surface"] == "claude" {
								variant["requires"] = []string{"agent:reviewer", "asset:guide-data", "skill:helper"}
								if resolved {
									variant["plugin_exports"].([]any)[0].(map[string]any)["roles"] = []any{map[string]any{"resource": "agent:reviewer", "resolution": "external", "description": "Client must provide the reviewer role"}}
								}
							}
						}
					}
				}
				resources = append(resources, map[string]any{"kind": "agent", "id": "reviewer", "source": "agents/reviewer.md", "description": "Reviewed role", "mode": "subagent", "tools": []string{}, "permissions": []string{}, "requires": []string{}, "conflicts": []string{}, "notices": []string{"notice:mit"}, "bindings": []any{map[string]any{"surface": "claude", "projection": "agent", "name": "reviewer", "invocation": "reviewer", "mode": "native", "sharing": "exclusive", "capabilities": []any{}, "client_projection": map[string]any{"client": "claude", "scopes": []string{"global", "project"}}}}, "surface_exclusions": []any{map[string]any{"surface": "agents", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed"}}})
				m["resources"] = resources
			})
			writePluginFixtureFile(t, filepath.Join(p, "packs/reviewed/agents/reviewer.md"), "Reviewed native role\n", 0644)
			a := pluginExportAssets(t, p, strings.Repeat("c", 40))
			destination := filepath.Join(t.TempDir(), "plugin")
			result, stderr, e := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", a, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "claude", "--skill", "guide", "--output", destination, "--json")
			if !resolved {
				if e == nil || !bytes.Contains(result, []byte("reviewed plugin adaptation or external prerequisite")) {
					t.Fatalf("unresolved role: %v %s %s", e, result, stderr)
				}
				if _, e := os.Lstat(destination); !os.IsNotExist(e) {
					t.Fatal("unresolved role published output")
				}
				return
			}
			if e != nil || !bytes.Contains(result, []byte("agent:reviewer: Client must provide the reviewer role")) {
				t.Fatalf("resolved role: %v %s %s", e, result, stderr)
			}
			if _, e := os.Stat(filepath.Join(destination, "agents")); !os.IsNotExist(e) {
				t.Fatal("native role was packaged")
			}
		})
	}
	t.Run("installed state and destination boundaries", func(t *testing.T) {
		workspace := filepath.Join(t.TempDir(), "candidate")
		directory := t.TempDir()
		environment := testprocess.Env(t)
		args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "install", "reviewed", "--surface", "claude", "--resource", "skill:guide"}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = environment
		cmd.Dir = directory
		master, slave, e := pty.Open()
		if e != nil {
			t.Fatal(e)
		}
		defer master.Close()
		defer slave.Close()
		cmd.Stdin = slave
		var install bytes.Buffer
		cmd.Stdout = &install
		cmd.Stderr = &install
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		if _, e = master.Write([]byte("y\n")); e != nil {
			t.Fatal(e)
		}
		if e = cmd.Wait(); e != nil {
			t.Fatalf("install: %v %s", e, install.String())
		}
		before := snapshotTree(t, workspace)
		bad := filepath.Join(workspace, "project", ".claude", "bad")
		result, _, e := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "claude", "--output", bad, "--json")
		if e == nil {
			t.Fatalf("managed destination accepted: %s", result)
		}
		occupied := t.TempDir()
		sentinel := filepath.Join(occupied, "keep")
		if e = os.WriteFile(sentinel, []byte("keep"), 0644); e != nil {
			t.Fatal(e)
		}
		result, _, e = runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "claude", "--output", occupied, "--json")
		if e == nil {
			t.Fatalf("nonempty destination accepted: %s", result)
		}
		content, e := os.ReadFile(sentinel)
		if e != nil || string(content) != "keep" {
			t.Fatal("existing output changed")
		}
		if os.Geteuid() != 0 {
			parent := t.TempDir()
			empty := filepath.Join(parent, "empty")
			if e = os.Mkdir(empty, 0700); e != nil {
				t.Fatal(e)
			}
			if e = os.Chmod(parent, 0500); e != nil {
				t.Fatal(e)
			}
			result, _, e = runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "claude", "--output", empty, "--json")
			if restore := os.Chmod(parent, 0700); restore != nil {
				t.Fatal(restore)
			}
			if e == nil {
				t.Fatalf("write failure succeeded: %s", result)
			}
			entries, readErr := os.ReadDir(empty)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("write failure changed empty destination")
			}
		}
		destination := filepath.Join(t.TempDir(), "plugin")
		result, stderr, e := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "claude", "--skill", "guide", "--output", destination, "--json")
		if e != nil {
			t.Fatalf("export with installation: %v %s %s", e, result, stderr)
		}
		if snapshotTree(t, workspace) != before {
			t.Fatal("export changed direct-install state")
		}
	})
	for _, f := range []string{".claude-plugin/plugin.json", "skills/guide/SKILL.md", "skills/helper/SKILL.md", "skills/leaf/SKILL.md", "assets/data.bin", "notices/reviewed-mit"} {
		if _, err := os.Stat(filepath.Join(output, f)); err != nil {
			t.Fatal(err)
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(output, ".claude-plugin/plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 3 || manifest["name"] != "reviewed" || manifest["version"] != "1.0.0" || manifest["description"] != "Synthetic reviewed Pack fixture" {
		t.Fatalf("Claude manifest: %s", manifestData)
	}
	data, err := os.ReadFile(filepath.Join(output, "skills/guide/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: guide", "/reviewed:helper", "../helper", "../../assets/data.bin", "disable-model-invocation: true", "allowed-tools: [Read, Grep]"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("missing %q: %s", want, data)
		}
	}
}
