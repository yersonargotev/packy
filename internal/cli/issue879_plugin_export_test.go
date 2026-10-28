package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue879PluginExportExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	assertReport := func(t *testing.T, out []byte) {
		t.Helper()
		assertStructuredOutput(t, repository, "plugin-export.schema.json", string(out))
	}
	t.Run("default and partial complete deterministic standalone packages", func(t *testing.T) {
		project := pluginExportProject(t, nil)
		assets := pluginExportAssets(t, project, strings.Repeat("a", 40))
		workspace := filepath.Join(t.TempDir(), "candidate")
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		directory := t.TempDir()
		run := func(output string, extra ...string) capabilitypack.PluginExportResult {
			t.Helper()
			args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--output", output, "--json"}
			args = append(args, extra...)
			out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, args...)
			if err != nil {
				t.Fatalf("export: %v %s %s", err, out, stderr)
			}
			assertReport(t, out)
			var result capabilitypack.PluginExportResult
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
		first := filepath.Join(t.TempDir(), "plugin")
		report := run(first)
		if report.PackID != "reviewed" || report.PluginName != "reviewed" || report.PackVersion != "1.0.0" || report.SnapshotID != strings.Repeat("a", 40) || report.Surface != "agents" || report.Format != "agent-plugins" || strings.Join(report.SelectedSkills, ",") != "guide,helper,leaf" {
			t.Fatalf("report identity: %+v", report)
		}
		for _, relative := range []string{"plugin.json", "packy-provenance.json", "skills/guide/SKILL.md", "skills/guide/agents/openai.yaml", "skills/guide/scripts/run.sh", "skills/guide/references/detail.md", "skills/guide/references/Deep Guide.md", "skills/helper/SKILL.md", "skills/leaf/SKILL.md", "assets/data.bin", "notices/reviewed-mit"} {
			if _, err := os.Stat(filepath.Join(first, relative)); err != nil {
				t.Fatal(err)
			}
		}
		body, err := os.ReadFile(filepath.Join(first, "skills/guide/SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"name: guide", "allowed-tools: Read", "custom-field: retained", "author: reviewed", "../helper", "../../notices/reviewed-mit", "../../assets/data.bin", "invoke-helper-explicitly", "ordinary guide prose stays unchanged"} {
			if !bytes.Contains(body, []byte(want)) {
				t.Fatalf("metadata/reference omitted %q: %s", want, body)
			}
		}
		metadata, err := os.ReadFile(filepath.Join(first, "skills/guide/agents/openai.yaml"))
		if err != nil || string(metadata) != "interface:\n  display_name: Reviewed Guide\npolicy:\n  allow_implicit_invocation: false\n" {
			t.Fatalf("metadata changed: %s %v", metadata, err)
		}
		info, err := os.Stat(filepath.Join(first, "skills/guide/scripts/run.sh"))
		if err != nil || info.Mode().Perm() != 0755 {
			t.Fatal("executable mode lost")
		}
		for _, absent := range []string{"AGENTS.md", "CLAUDE.md", "instructions", "agents/reviewer.md", ".codex", "skills/native-only", "mcp.json"} {
			if _, err := os.Lstat(filepath.Join(first, absent)); !os.IsNotExist(err) {
				t.Fatalf("unexpected exported content %s", absent)
			}
		}
		protected := map[string]string{}
		for _, relative := range []string{"snapshots", "project", "home", "config"} {
			protected[relative] = snapshotTree(t, filepath.Join(workspace, relative))
		}
		second := t.TempDir() // Existing empty output is supported atomically.
		repeated := run(second)
		if pluginPackageBytesAndModes(t, first) != pluginPackageBytesAndModes(t, second) || report.ContentSHA256 != repeated.ContentSHA256 {
			t.Fatal("repeated export changed bytes or modes")
		}
		partial := filepath.Join(t.TempDir(), "partial")
		partialReport := run(partial, "--skill", "guide")
		if strings.Join(partialReport.SelectedSkills, ",") != "guide" {
			t.Fatalf("partial roots: %+v", partialReport)
		}
		for _, skill := range []string{"guide", "helper", "leaf"} {
			if _, err := os.Stat(filepath.Join(partial, "skills", skill, "SKILL.md")); err != nil {
				t.Fatal("transitive closure missing", err)
			}
		}
		leaf := filepath.Join(t.TempDir(), "leaf")
		run(leaf, "--skill", "leaf")
		if _, err := os.Stat(filepath.Join(leaf, "skills/guide")); !os.IsNotExist(err) {
			t.Fatal("partial selection exported unrelated skill")
		}
		for relative, before := range protected {
			if snapshotTree(t, filepath.Join(workspace, relative)) != before {
				t.Fatalf("export mutated %s", relative)
			}
		}
		if _, err := os.Stat(filepath.Join(workspace, "home/sentinel-executed")); !os.IsNotExist(err) {
			t.Fatal("export executed content")
		}
		before := snapshotTree(t, first)
		for _, args := range [][]string{
			{"--skill", "native-only"}, {"--skill", "missing"}, {"--skill", "../guide"}, {"--surface", "claude"}, {"--wat"},
		} {
			output := filepath.Join(t.TempDir(), "failed")
			base := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--output", output}
			base = append(base, args...)
			base = append(base, "--json")
			out, _, err := runSkillAccessCLI(t, binary, directory, environment, base...)
			if err == nil {
				t.Fatalf("invalid selection succeeded: %v %s", args, out)
			}
			assertReport(t, out)
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatal("failed export published output")
			}
		}
		out, _, err := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--output", first, "--json")
		if err == nil || snapshotTree(t, first) != before {
			t.Fatalf("nonempty output changed: %v %s", err, out)
		}
		assertReport(t, out)
		for _, output := range []string{filepath.Join(workspace, "snapshots/new"), filepath.Join(workspace, "home/.agents/plugins/new"), filepath.Join(directory, ".agents/plugins/new")} {
			out, _, err := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--output", output, "--json")
			if err == nil {
				t.Fatalf("unsafe destination succeeded: %s %s", output, out)
			}
			assertReport(t, out)
		}
	})

	t.Run("standalone provenance resolves only the selected immutable origins", func(t *testing.T) {
		project := pluginExportProject(t, func(m map[string]any) {
			m["origins"] = []any{map[string]any{"id": "unused", "repository": "fixture/unused", "commit": strings.Repeat("1", 40)}, map[string]any{"id": "upstream", "repository": "fixture/upstream", "commit": strings.Repeat("2", 40), "revision": "v1.2.3"}}
			for _, value := range m["resources"].([]any) {
				r := value.(map[string]any)
				if r["kind"] == "skill" && r["id"] == "guide" {
					r["plugin_exports"].([]any)[0].(map[string]any)["origin"] = map[string]any{"id": "upstream", "path": ".", "relationship": "adapted"}
				}
			}
		})
		upstream := t.TempDir()
		writePluginFixtureFile(t, filepath.Join(upstream, "upstream.txt"), "Immutable upstream fixture\n", 0644)
		resolver := pluginExportOriginFixture{root: upstream}
		validation, err := cataloglayout.ValidateCatalogProject(context.Background(), project, "", resolver)
		if err != nil {
			t.Fatal(err)
		}
		assets := filepath.Join(t.TempDir(), "assets")
		if _, err := cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: strings.Repeat("6", 40), Builder: "yersonargotev/packy@" + strings.Repeat("e", 40)}, assets); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(t.TempDir(), "plugin")
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "agents", "--skill", "guide", "--output", output, "--json")
		if err != nil {
			t.Fatalf("origin export: %v %s %s", err, out, stderr)
		}
		assertReport(t, out)
		data, err := os.ReadFile(filepath.Join(output, "packy-provenance.json"))
		if err != nil {
			t.Fatal(err)
		}
		var provenance struct {
			Origins []capabilitypack.PackOrigin `json:"origins"`
		}
		if err := json.Unmarshal(data, &provenance); err != nil {
			t.Fatal(err)
		}
		if len(provenance.Origins) != 1 || provenance.Origins[0].ID != "upstream" || provenance.Origins[0].Repository != "fixture/upstream" || provenance.Origins[0].Commit != strings.Repeat("2", 40) || provenance.Origins[0].Revision != "v1.2.3" {
			t.Fatalf("standalone origin registry: %s", data)
		}
		if bytes.Contains(data, []byte("fixture/unused")) {
			t.Fatal("export copied unrelated origin")
		}
	})
	t.Run("reviewed source and reference filenames can contain spaces", func(t *testing.T) {
		project := pluginExportProject(t, func(m map[string]any) {
			for _, value := range m["resources"].([]any) {
				r := value.(map[string]any)
				if r["kind"] == "skill" && r["id"] == "guide" {
					c := r["plugin_exports"].([]any)[0].(map[string]any)
					c["source"] = "skills/plugin guide"
					c["references"].([]any)[0].(map[string]any)["file"] = "references/Deep Guide.md"
				}
			}
		})
		root := filepath.Join(project, "packs/reviewed")
		if err := os.Rename(filepath.Join(root, "skills/plugin-guide"), filepath.Join(root, "skills/plugin guide")); err != nil {
			t.Fatal(err)
		}
		entry := filepath.Join(root, "skills/plugin guide/SKILL.md")
		data, readErr := os.ReadFile(entry)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err := os.WriteFile(entry, bytes.ReplaceAll(data, []byte("[Asset]({{packy.export:asset}})\n"), nil), 0644); err != nil {
			t.Fatal(err)
		}
		writePluginFixtureFile(t, filepath.Join(root, "skills/plugin guide/references/Deep Guide.md"), "Reviewed data path: {{packy.export:asset}}\n", 0644)
		assets := pluginExportAssets(t, project, strings.Repeat("7", 40))
		output := filepath.Join(t.TempDir(), "plugin")
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "agents", "--skill", "guide", "--output", output, "--json")
		if err != nil {
			t.Fatalf("spaced reviewed path: %v %s %s", err, out, stderr)
		}
		assertReport(t, out)
		if !bytes.Contains(out, []byte("skills/plugin guide")) {
			t.Fatal("reviewed source identity omitted")
		}
		content, err := os.ReadFile(filepath.Join(output, "skills/guide/references/Deep Guide.md"))
		if err != nil || string(content) != "Reviewed data path: ../../../assets/data.bin\n" {
			t.Fatalf("spaced reference: %s %v", content, err)
		}
	})
	t.Run("logical identity stays distinct from exported skill name", func(t *testing.T) {
		logical := strings.Repeat("a", 65)
		project := pluginExportProject(t, func(m map[string]any) {
			resources := m["resources"].([]any)
			resources = append(resources, map[string]any{"kind": "skill", "id": logical, "source": "skills/long-logical", "description": "Long logical identity with a short reviewed native name", "requires": []string{}, "conflicts": []string{}, "notices": []string{"notice:mit"}, "bindings": []any{map[string]any{"surface": "agents", "projection": "skill", "name": "short-name", "invocation": "short-name", "mode": "native", "sharing": "exclusive", "capabilities": []any{}}, map[string]any{"surface": "claude", "projection": "skill", "name": "short-name", "invocation": "/short-name", "mode": "native", "sharing": "exclusive", "capabilities": []any{}}}, "surface_exclusions": []any{}, "plugin_exports": []any{pluginExportContract("short-name")}})
			m["resources"] = resources
		})
		writePluginFixtureFile(t, filepath.Join(project, "packs/reviewed/skills/long-logical/SKILL.md"), "---\nname: short-name\ndescription: Reviewed logical identity.\n---\nBody\n", 0644)
		assets := pluginExportAssets(t, project, strings.Repeat("8", 40))
		output := filepath.Join(t.TempDir(), "plugin")
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "agents", "--skill", logical, "--output", output, "--json")
		if err != nil {
			t.Fatalf("logical identity: %v %s %s", err, out, stderr)
		}
		assertReport(t, out)
		if _, err := os.Stat(filepath.Join(output, "skills/short-name/SKILL.md")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("retained offline export uses exact snapshot without installed state", func(t *testing.T) {
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		env := adoptionEnvironmentMap(environment)
		directory := t.TempDir()
		firstID, nextID := strings.Repeat("b", 40), strings.Repeat("c", 40)
		first := pluginExportAssets(t, pluginExportProject(t, nil), firstID)
		acquireSkillAccessCatalog(t, env, directory, first, firstID, "init")
		next := pluginExportAssets(t, pluginExportProject(t, nil), nextID)
		acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
		root := catalogstore.DefaultDataRoot(env["HOME"])
		before := snapshotTree(t, root)
		output := filepath.Join(t.TempDir(), "retained")
		out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "export", "reviewed", "--surface", "agents", "--snapshot", firstID, "--skill", "guide", "--output", output, "--json")
		if err != nil {
			t.Fatalf("retained offline export: %v %s %s", err, out, stderr)
		}
		assertReport(t, out)
		if !bytes.Contains(out, []byte(firstID)) || snapshotTree(t, root) != before {
			t.Fatal("retained identity or state changed")
		}
		if err := os.Remove(filepath.Join(root, "catalog/snapshots", firstID, "packs/reviewed/skills/plugin-guide/SKILL.md")); err != nil {
			t.Fatal(err)
		}
		failed := filepath.Join(t.TempDir(), "missing-retained")
		out, _, err = runSkillAccessCLI(t, binary, directory, environment, "export", "reviewed", "--surface", "agents", "--snapshot", firstID, "--output", failed, "--json")
		if err == nil || !bytes.Contains(out, []byte("snapshot_unavailable")) {
			t.Fatalf("missing retained content substituted latest: %v %s", err, out)
		}
		assertReport(t, out)
		if _, err := os.Lstat(failed); !os.IsNotExist(err) {
			t.Fatal("missing retained snapshot published output")
		}
	})

	t.Run("invalid references and incompatible dependencies publish nothing", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(map[string]any)
			file string
			body string
		}{
			{name: "incompatible dependency", edit: func(m map[string]any) {
				for _, value := range m["resources"].([]any) {
					r := value.(map[string]any)
					if r["id"] == "helper" {
						delete(r, "plugin_exports")
					}
				}
			}},
			{name: "missing reference file", edit: func(m map[string]any) {
				for _, value := range m["resources"].([]any) {
					r := value.(map[string]any)
					if r["id"] == "guide" {
						r["plugin_exports"].([]any)[0].(map[string]any)["references"].([]any)[0].(map[string]any)["file"] = "missing.md"
					}
				}
			}},
			{name: "unresolved marker", file: "skills/plugin-guide/references/detail.md", body: "{{packy.export:undeclared}}"},
			{name: "unsupported native metadata", file: "skills/plugin-guide/SKILL.md", body: "---\nname: guide\ndescription: Reviewed skill.\nagent: native-role\n---\nMust use reviewed adaptation"},
			{name: "invalid skill metadata", file: "skills/plugin-guide/SKILL.md", body: "---\nname: guide\ndescription: 123\n---\ninvalid metadata"},
			{name: "instruction dependency", edit: func(m map[string]any) {
				for _, value := range m["resources"].([]any) {
					r := value.(map[string]any)
					if r["id"] == "guide" {
						r["requires"] = []string{"asset:guide-data", "instruction:guide-instructions", "skill:helper"}
					}
				}
			}},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				project := pluginExportProject(t, test.edit)
				if test.file != "" {
					writePluginFixtureFile(t, filepath.Join(project, "packs/reviewed", test.file), test.body, 0644)
				}
				assets := pluginExportAssets(t, project, strings.Repeat("f", 40))
				workspace := filepath.Join(t.TempDir(), "candidate")
				output := filepath.Join(t.TempDir(), "plugin")
				out, _, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--skill", "guide", "--output", output, "--json")
				if err == nil {
					t.Fatalf("invalid export succeeded: %s", out)
				}
				assertReport(t, out)
				if _, err := os.Lstat(output); !os.IsNotExist(err) {
					t.Fatal("invalid export published partial output")
				}
			})
		}
	})
	t.Run("write failure preserves empty destination and managed state", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory write permissions")
		}
		assets := pluginExportAssets(t, pluginExportProject(t, nil), strings.Repeat("9", 40))
		workspace := filepath.Join(t.TempDir(), "candidate")
		environment := testprocess.Env(t)
		directory := t.TempDir()
		if out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "list", "--json"); err != nil {
			t.Fatalf("candidate: %v %s %s", err, out, stderr)
		}
		before := snapshotTree(t, workspace)
		parent := t.TempDir()
		output := filepath.Join(parent, "empty")
		if err := os.Mkdir(output, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, 0500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(parent, 0700)
		out, _, err := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "export", "reviewed", "--surface", "agents", "--output", output, "--json")
		if err == nil {
			t.Fatalf("unwritable output succeeded: %s", out)
		}
		assertReport(t, out)
		entries, err := os.ReadDir(output)
		if err != nil || len(entries) != 0 {
			t.Fatal("write failure modified empty destination")
		}
		if snapshotTree(t, workspace) != before {
			t.Fatal("write failure changed managed state")
		}
	})
	t.Run("reviewed external and adapted roles never register agents", func(t *testing.T) {
		for _, resolution := range []string{"external", "adapted", "unresolved"} {
			t.Run(resolution, func(t *testing.T) {
				project := pluginExportProject(t, func(manifest map[string]any) {
					resources := manifest["resources"].([]any)
					for _, value := range resources {
						r := value.(map[string]any)
						if r["id"] == "guide" && r["kind"] == "skill" {
							r["requires"] = []string{"agent:reviewer", "asset:guide-data", "skill:helper"}
							if resolution != "unresolved" {
								r["plugin_exports"].([]any)[0].(map[string]any)["roles"] = []any{map[string]any{"resource": "agent:reviewer", "resolution": resolution, "description": "Client must provide the reviewed reviewer role"}}
							}
						}
					}
					resources = append(resources, map[string]any{"kind": "agent", "id": "reviewer", "source": "agents/reviewer.md", "description": "Reviewed role", "mode": "subagent", "tools": []string{}, "permissions": []string{}, "requires": []string{"skill:leaf"}, "conflicts": []string{}, "notices": []string{"notice:mit"}, "bindings": []any{map[string]any{"surface": "agents", "projection": "agent", "name": "reviewer", "invocation": "reviewer", "mode": "native", "sharing": "exclusive", "capabilities": []any{}, "client_projection": map[string]any{"client": "codex", "scopes": []string{"global"}}}}, "surface_exclusions": []any{map[string]any{"surface": "claude", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed"}}})
					manifest["resources"] = resources
				})
				writePluginFixtureFile(t, filepath.Join(project, "packs/reviewed/agents/reviewer.md"), "Native reviewer instructions", 0644)
				assets := pluginExportAssets(t, project, strings.Repeat("d", 40))
				output := filepath.Join(t.TempDir(), "plugin")
				out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t), "catalog", "candidate", "--snapshot", assets, "--workspace", filepath.Join(t.TempDir(), "candidate"), "--", "export", "reviewed", "--surface", "agents", "--skill", "guide", "--output", output, "--json")
				assertReport(t, out)
				if resolution == "unresolved" {
					if err == nil || !bytes.Contains(out, []byte("reviewed plugin adaptation or external prerequisite")) {
						t.Fatalf("unresolved role accepted: %v %s %s", err, out, stderr)
					}
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatal("unresolved role published partial output")
					}
					return
				}
				if err != nil {
					t.Fatalf("reviewed role: %v %s %s", err, out, stderr)
				}
				var report capabilitypack.PluginExportResult
				json.Unmarshal(out, &report)
				if resolution == "external" && !strings.Contains(strings.Join(report.Prerequisites, ","), "agent:reviewer: Client must provide") {
					t.Fatal("external role prerequisite omitted")
				}
				if resolution == "adapted" && len(report.Prerequisites) != 0 {
					t.Fatal("adaptation falsely declared an external requirement")
				}
				if _, err := os.Stat(filepath.Join(output, "agents")); !os.IsNotExist(err) {
					t.Fatal("export registered a native role")
				}
			})
		}
	})
}

func pluginExportContract(name string) map[string]any {
	return map[string]any{"format": "agent-plugins", "name": name, "prerequisites": []string{}, "references": []any{}, "roles": []any{}}
}

func pluginExportProject(t *testing.T, edit func(map[string]any)) string {
	t.Helper()
	project := skillAccessProject(t, "plugin", func(manifest map[string]any) {
		manifest["external_requirements"] = []string{}
		resources := manifest["resources"].([]any)
		for _, value := range resources {
			r := value.(map[string]any)
			delete(r, "on_demand")
			if r["kind"] == "skill" {
				for _, v := range r["variants"].([]any) {
					variant := v.(map[string]any)
					delete(variant, "on_demand")
					if variant["surface"] == "claude" {
						variant["requires"] = []string{}
						variant["plugin_exports"] = []any{}
					}
				}
				contract := pluginExportContract("guide")
				contract["source"] = "skills/plugin-guide"
				contract["references"] = []any{
					map[string]any{"id": "asset", "file": "SKILL.md", "target": "asset:guide-data", "kind": "path"},
					map[string]any{"id": "helper-call", "file": "SKILL.md", "target": "skill:helper", "kind": "invocation", "value": "invoke-helper-explicitly"},
					map[string]any{"id": "helper-name", "file": "SKILL.md", "target": "skill:helper", "kind": "name"},
					map[string]any{"id": "helper-path", "file": "SKILL.md", "target": "skill:helper", "kind": "path"},
					map[string]any{"id": "notice", "file": "SKILL.md", "target": "notice:mit", "kind": "path"},
				}
				r["plugin_exports"] = []any{contract}
				r["requires"] = []string{"asset:guide-data", "skill:helper"}
			}
		}
		for _, id := range []string{"helper", "leaf", "native-only"} {
			r := map[string]any{"kind": "skill", "id": id, "source": "skills/" + id, "description": "Reviewed " + id, "requires": []string{}, "conflicts": []string{}, "notices": []string{"notice:mit"}, "bindings": []any{map[string]any{"surface": "agents", "projection": "skill", "name": id, "invocation": id, "mode": "native", "sharing": "exclusive", "capabilities": []any{}}}, "surface_exclusions": []any{map[string]any{"surface": "claude", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed"}}}
			r["bindings"] = append(r["bindings"].([]any), map[string]any{"surface": "claude", "projection": "skill", "name": id, "invocation": "/" + id, "mode": "native", "sharing": "exclusive", "capabilities": []any{}})
			r["surface_exclusions"] = []any{}
			if id != "native-only" {
				r["plugin_exports"] = []any{pluginExportContract(id)}
			}
			if id == "helper" {
				r["requires"] = []string{"skill:leaf"}
			}
			resources = append(resources, r)
		}
		resources = append(resources, map[string]any{"kind": "instruction", "id": "guide-instructions", "source": "instructions/guide.md", "description": "Project instructions excluded from plugin", "requires": []string{}, "conflicts": []string{}, "bindings": []any{map[string]any{"surface": "agents", "projection": "instruction", "name": "guide-instructions", "invocation": "AGENTS.md", "mode": "native", "sharing": "exclusive", "capabilities": []any{map[string]any{"type": "project-instruction", "project_instruction": map[string]any{"id": "guide", "source": "instructions/guide.md"}}}}}, "surface_exclusions": []any{map[string]any{"surface": "claude", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed"}}})
		manifest["resources"] = resources
		if edit != nil {
			edit(manifest)
		}
		resources = manifest["resources"].([]any)
		sort.Slice(resources, func(i, j int) bool {
			a, b := resources[i].(map[string]any), resources[j].(map[string]any)
			return a["kind"].(string)+":"+a["id"].(string) < b["kind"].(string)+":"+b["id"].(string)
		})
	})
	root := filepath.Join(project, "packs/reviewed")
	for _, id := range []string{"helper", "leaf", "native-only"} {
		writePluginFixtureFile(t, filepath.Join(root, "skills", id, "SKILL.md"), "---\nname: "+id+"\ndescription: Reviewed helper.\n---\nHelper content\n", 0644)
	}
	writePluginFixtureFile(t, filepath.Join(root, "instructions/guide.md"), "Project instructions must not be exported\n", 0644)
	writePluginFixtureFile(t, filepath.Join(root, "skills/plugin-guide/SKILL.md"), "---\nname: original-guide\ndescription: Reviewed guide.\nallowed-tools: Read\nmetadata:\n  custom-field: retained\n  author: reviewed\n---\nordinary guide prose stays unchanged\nHelper {{packy.export:helper-name}} at {{packy.export:helper-path}}\nCall {{packy.export:helper-call}}\n[Asset]({{packy.export:asset}})\n[Notice]({{packy.export:notice}})\n[Detail](references/detail.md)\n", 0644)
	writePluginFixtureFile(t, filepath.Join(root, "skills/plugin-guide/agents/openai.yaml"), "interface:\n  display_name: Reviewed Guide\npolicy:\n  allow_implicit_invocation: false\n", 0644)
	writePluginFixtureFile(t, filepath.Join(root, "skills/plugin-guide/references/Deep Guide.md"), "Reviewed supporting filename with spaces.\n", 0644)
	writePluginFixtureFile(t, filepath.Join(root, "skills/plugin-guide/references/detail.md"), "Complete supporting reference\n", 0644)
	writePluginFixtureFile(t, filepath.Join(root, "skills/plugin-guide/scripts/run.sh"), "#!/bin/sh\nprintf executed > \"$HOME/sentinel-executed\"\n", 0755)
	return project
}

func writePluginFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
func pluginExportAssets(t *testing.T, project, commit string) string {
	t.Helper()
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(t.TempDir(), "assets")
	if _, err := cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("e", 40)}, assets); err != nil {
		t.Fatal(err)
	}
	return assets
}

func TestIssue879PluginCompatibilitySchemaMatchesCatalog(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas/pack/v3/pack.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("pack.schema.json", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("pack.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		valid bool
		edit  func(map[string]any)
	}{
		{"reviewed", true, nil},
		{"unknown format", false, func(c map[string]any) { c["format"] = "unknown" }},
		{"unknown field", false, func(c map[string]any) { c["execute"] = true }},
		{"missing prerequisites", false, func(c map[string]any) { delete(c, "prerequisites") }},
		{"null prerequisites", false, func(c map[string]any) { c["prerequisites"] = nil }},
		{"control source", false, func(c map[string]any) { c["source"] = "skills/\u0085adapted" }},
		{"control reference file", false, func(c map[string]any) {
			c["references"].([]any)[0].(map[string]any)["file"] = "references/\u0085file.md"
		}},
		{"long reference identity", true, func(c map[string]any) { c["references"].([]any)[4].(map[string]any)["id"] = strings.Repeat("z", 65) }},
		{"empty source", false, func(c map[string]any) { c["source"] = "" }},
		{"null source", false, func(c map[string]any) { c["source"] = nil }},
		{"null references", false, func(c map[string]any) { c["references"] = nil }},
		{"escaping source", false, func(c map[string]any) { c["source"] = "../escape" }},
		{"escaping reference", false, func(c map[string]any) { c["references"].([]any)[0].(map[string]any)["file"] = "../foreign.md" }},
		{"implicit invocation", false, func(c map[string]any) { delete(c["references"].([]any)[1].(map[string]any), "value") }},
		{"unknown role resolution", false, func(c map[string]any) {
			c["roles"] = []any{map[string]any{"resource": "agent:reviewer", "resolution": "automatic", "description": "Do not infer adaptation"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := pluginExportProject(t, func(m map[string]any) {
				if test.edit == nil {
					return
				}
				for _, value := range m["resources"].([]any) {
					r := value.(map[string]any)
					if r["kind"] == "skill" && r["id"] == "guide" {
						test.edit(r["plugin_exports"].([]any)[0].(map[string]any))
					}
				}
			})
			data, err := os.ReadFile(filepath.Join(project, "packs/reviewed/pack.json"))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(instance); (err == nil) != test.valid {
				t.Fatalf("schema valid=%v: %v", test.valid, err)
			}
			_, err = cataloglayout.ValidateCatalogProject(context.Background(), project, "", nil)
			if (err == nil) != test.valid {
				t.Fatalf("catalog valid=%v: %v", test.valid, err)
			}
		})
	}
}

func pluginPackageBytesAndModes(t *testing.T, root string) string {
	t.Helper()
	var result strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&result, "%s %s\n", relative, info.Mode())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&result, "%x\n", data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.String()
}

type pluginExportOriginFixture struct{ root string }

func (fixture pluginExportOriginFixture) Resolve(context.Context, cataloglayout.Origin) (string, error) {
	return fixture.root, nil
}
