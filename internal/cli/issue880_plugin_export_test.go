package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	project := pluginExportProject(t, func(m map[string]any) {
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
	})
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
	for _, f := range []string{".claude-plugin/plugin.json", "skills/guide/SKILL.md", "skills/helper/SKILL.md", "skills/leaf/SKILL.md", "assets/data.bin", "notices/reviewed-mit"} {
		if _, err := os.Stat(filepath.Join(output, f)); err != nil {
			t.Fatal(err)
		}
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
