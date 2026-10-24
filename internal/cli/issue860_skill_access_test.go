package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue860SkillAccessExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build public executable: %v\n%s", err, out)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	assertReport := func(t *testing.T, out []byte) {
		t.Helper()
		assertStructuredOutput(t, repository, "skill-access.schema.json", string(out))
	}
	t.Run("missing catalog is actionable and versioned", func(t *testing.T) {
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		out, _, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
		if err == nil || !bytes.Contains(out, []byte(`"catalog_unavailable"`)) || !bytes.Contains(out, []byte("packy init")) {
			t.Fatalf("missing catalog: %v\n%s", err, out)
		}
		assertReport(t, out)
		for _, test := range []struct {
			args []string
			code string
		}{
			{[]string{"--surface", "codex"}, "invalid_arguments"},
			{[]string{"guide", "--surface", "codex"}, "invalid_identity"},
			{[]string{"../bad:guide", "--surface", "codex"}, "invalid_identity"},
			{[]string{"reviewed:guide"}, "unsupported_surface"},
			{[]string{"reviewed:guide", "--surface", "opencode"}, "unsupported_surface"},
			{[]string{"reviewed:guide", "--wat", "--surface", "codex"}, "invalid_arguments"},
		} {
			args := append([]string{"get", "skill"}, test.args...)
			out, _, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, append(args, "--json")...)
			if err == nil || !bytes.Contains(out, []byte(`"`+test.code+`"`)) {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			assertReport(t, out)
		}
	})
	t.Run("host effective complete tree without installation", func(t *testing.T) {
		assets := skillAccessAssets(t, strings.Repeat("a", 40), "first", nil)
		workspace := filepath.Join(t.TempDir(), "candidate")
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		directory := t.TempDir() // Deliberately outside Git, with no script runtimes.
		if out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "list", "--json"); err != nil {
			t.Fatalf("prepare candidate: %v %s %s", err, out, stderr)
		}
		protected := map[string]string{}
		for _, relative := range []string{"project/packy.json", "project/packy.lock.json", "project/AGENTS.md", "project/.agents/skills/guide/SKILL.md", "home/.packy/packs.json", "home/.agents/skills/guide/SKILL.md", "home/.claude/skills/guide/SKILL.md", "home/.codex/AGENTS.md", "config/foreign.txt"} {
			path := filepath.Join(workspace, relative)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("local modified sentinel\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		for _, relative := range []string{"snapshots", "project", "config", "home/.packy", "home/.agents", "home/.claude", "home/.codex"} {
			protected[relative] = snapshotTree(t, filepath.Join(workspace, relative))
		}
		for _, surface := range []string{"claude", "codex"} {
			args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "get", "skill", "reviewed:guide", "--surface", surface, "--json"}
			out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, args...)
			if err != nil {
				t.Fatalf("retrieve %s: %v\n%s\n%s", surface, err, out, stderr)
			}
			assertReport(t, out)
			var result struct {
				SchemaVersion  int      `json:"schema_version"`
				Report         string   `json:"report"`
				Result         string   `json:"result"`
				QualifiedSkill string   `json:"qualified_skill"`
				PackVersion    string   `json:"pack_version"`
				Surface        string   `json:"surface"`
				SnapshotID     string   `json:"snapshot_id"`
				EntryDocument  string   `json:"entry_document"`
				ResourceRoot   string   `json:"resource_root"`
				Prerequisites  []string `json:"prerequisites"`
			}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			if result.SchemaVersion != 1 || result.Report != "skill-access" || result.Result != "prepared" || result.QualifiedSkill != "reviewed:guide" || result.PackVersion != "1.0.0" || result.Surface != surface || result.SnapshotID != strings.Repeat("a", 40) || strings.Join(result.Prerequisites, ",") != "missing-fixture-runtime" {
				t.Fatalf("incomplete retrieval identity: %s", out)
			}
			body, err := os.ReadFile(result.EntryDocument)
			if err != nil || !strings.Contains(string(body), surface+" first") {
				t.Fatalf("entry document: %q, %v", body, err)
			}
			if result.EntryDocument != filepath.Join(result.ResourceRoot, "SKILL.md") {
				t.Fatal("entry document does not belong to returned resource root")
			}
			for _, relative := range []string{"references/detail.md", "agents/openai.yaml", "scripts/run.sh", "asset.bin", "../../assets/data.bin", "../../notices/reviewed-mit"} {
				prepared := filepath.Join(result.ResourceRoot, relative)
				source := filepath.Join(workspace, "snapshots", result.SnapshotID, "packs/reviewed/skills", surface, relative)
				want, err := os.ReadFile(source)
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(prepared)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("prepared %s = %q, %v; want %q", relative, got, err, want)
				}
				wantInfo, _ := os.Stat(source)
				gotInfo, err := os.Stat(prepared)
				if err != nil || gotInfo.Mode().Perm() != wantInfo.Mode().Perm() {
					t.Fatalf("mode changed for %s", relative)
				}
			}
			if _, err := os.Stat(filepath.Join(workspace, "home/sentinel-executed")); !os.IsNotExist(err) {
				t.Fatal("retrieval executed a catalog script")
			}
			for relative, before := range protected {
				if snapshotTree(t, filepath.Join(workspace, relative)) != before {
					t.Fatalf("retrieval changed %s", relative)
				}
			}
		}
	})
	t.Run("official local acquisition refresh and retained identity", func(t *testing.T) {
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		env := adoptionEnvironmentMap(environment)
		directory := t.TempDir()
		firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
		first := skillAccessAssets(t, firstID, "first", nil)
		acquireSkillAccessCatalog(t, env, directory, first, firstID, "init")
		get := func(snapshot string) (string, []byte) {
			t.Helper()
			args := []string{"get", "skill", "reviewed:guide", "--surface", "codex", "--json"}
			if snapshot != "" {
				args = append(args, "--snapshot", snapshot)
			}
			out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, args...)
			if err != nil {
				t.Fatalf("retrieve: %v %s %s", err, out, stderr)
			}
			assertReport(t, out)
			var result struct {
				EntryDocument string `json:"entry_document"`
				SnapshotID    string `json:"snapshot_id"`
			}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			want := snapshot
			if want == "" {
				want = nextID
			}
			if result.SnapshotID != want {
				t.Fatalf("snapshot = %s, want %s", result.SnapshotID, want)
			}
			body, err := os.ReadFile(result.EntryDocument)
			if err != nil {
				t.Fatal(err)
			}
			return result.EntryDocument, body
		}
		path, body := get(firstID)
		for _, identity := range []string{"reviewed:missing", "missing:guide"} {
			out, _, err := runSkillAccessCLI(t, binary, directory, environment, "get", "skill", identity, "--surface", "codex", "--json")
			if err == nil || !bytes.Contains(out, []byte(`"unknown_skill"`)) {
				t.Fatalf("unknown identity %s: %v %s", identity, err, out)
			}
			assertReport(t, out)
		}
		sourceRoot := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", firstID)
		before := snapshotTree(t, sourceRoot)
		next := skillAccessAssets(t, nextID, "next", nil)
		acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
		newPath, newBody := get("")
		if path == newPath || !bytes.Contains(newBody, []byte("codex next")) {
			t.Fatal("new request did not follow explicit refresh")
		}
		if err := os.WriteFile(newPath, []byte("consumer modification\n"), 0644); err != nil {
			t.Fatal(err)
		}
		drift, _, driftErr := runSkillAccessCLI(t, binary, directory, environment, "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
		if driftErr == nil || !bytes.Contains(drift, []byte(`"preparation_failed"`)) {
			t.Fatalf("prepared drift succeeded: %v %s", driftErr, drift)
		}
		assertReport(t, drift)
		if data, err := os.ReadFile(newPath); err != nil || string(data) != "consumer modification\n" {
			t.Fatal("retrieval overwrote prepared drift")
		}
		retainedPath, retainedBody := get(firstID)
		if retainedPath != path || !bytes.Equal(retainedBody, body) || snapshotTree(t, sourceRoot) != before {
			t.Fatal("refresh or retrieval changed retained content")
		}
		readable, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "get", "skill", "reviewed:guide", "--surface", "claude")
		if err != nil {
			t.Fatalf("readable: %v %s", err, stderr)
		}
		for _, want := range []string{"reviewed:guide", "1.0.0", "claude", nextID, "Entry document:", "Resource root:", "missing-fixture-runtime", "not verified", "no native registration or workflow execution"} {
			if !bytes.Contains(readable, []byte(want)) {
				t.Fatalf("readable omitted %s: %s", want, readable)
			}
		}
		if err := os.Remove(filepath.Join(sourceRoot, "packs/reviewed/skills/codex/asset.bin")); err != nil {
			t.Fatal(err)
		}
		out, _, err := runSkillAccessCLI(t, binary, directory, environment, "get", "skill", "reviewed:guide", "--surface", "codex", "--snapshot", firstID, "--json")
		if err == nil || !bytes.Contains(out, []byte(`"snapshot_unavailable"`)) {
			t.Fatalf("missing retained source substituted a newer revision: %v %s", err, out)
		}
		assertReport(t, out)
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, body) {
			t.Fatal("returned location no longer usable")
		}
	})
	t.Run("unreviewed and dependency bearing skills never succeed", func(t *testing.T) {
		for _, test := range []struct {
			name, code string
			edit       func(map[string]any)
		}{
			{"unreviewed", "incompatible_resource", func(manifest map[string]any) { delete(manifest["resources"].([]any)[1].(map[string]any), "on_demand") }},
			{"excluded", "unsupported_surface", func(manifest map[string]any) {
				skill := manifest["resources"].([]any)[1].(map[string]any)
				bindings := skill["bindings"].([]any)
				skill["bindings"] = []any{bindings[0], bindings[2]}
				skill["variants"] = skill["variants"].([]any)[:1]
				skill["surface_exclusions"] = []any{map[string]any{"surface": "codex", "mode": "optional", "code": "review-needed", "reason": "Codex is not reviewed"}}
			}},
			{"native template", "incompatible_resource", func(manifest map[string]any) {
				for _, value := range manifest["resources"].([]any)[1].(map[string]any)["bindings"].([]any) {
					value.(map[string]any)["capabilities"] = []any{map[string]any{"type": "skill-template", "skill_template": map[string]any{"naming": "native"}}}
				}
			}},
			{"dependency", "skill_dependencies_unsupported", func(manifest map[string]any) {
				resources := manifest["resources"].([]any)
				skill := resources[1].(map[string]any)
				encoded, _ := json.Marshal(skill)
				var helper map[string]any
				if err := json.Unmarshal(encoded, &helper); err != nil {
					t.Fatal(err)
				}
				helper["id"] = "helper"
				helper["source"] = "skills/helper"
				delete(helper, "variants")
				for _, value := range helper["bindings"].([]any) {
					binding := value.(map[string]any)
					binding["name"] = "helper"
					prefix := ""
					if binding["surface"] == "codex" {
						prefix = "$"
					}
					if binding["surface"] == "claude" {
						prefix = "/"
					}
					binding["invocation"] = prefix + "helper"
				}
				skill["requires"] = []string{"skill:helper"}
				manifest["resources"] = append(resources, helper)
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				assets := skillAccessAssets(t, strings.Repeat("d", 40), "first", test.edit)
				workspace := filepath.Join(t.TempDir(), "candidate")
				environment := testprocess.Env(t, "PATH="+t.TempDir())
				out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
				if err == nil || !bytes.Contains(out, []byte(`"`+test.code+`"`)) {
					t.Fatalf("%s: %v %s %s", test.name, err, out, stderr)
				}
				assertReport(t, out)
				if _, err := os.Stat(filepath.Join(workspace, "home/.local/share/packy/skill-access")); !os.IsNotExist(err) {
					t.Fatal("failed access left prepared partial resources")
				}
			})
		}
	})
	t.Run("linked preparation parent cannot mutate catalog sources", func(t *testing.T) {
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		env := adoptionEnvironmentMap(environment)
		directory := t.TempDir()
		id := strings.Repeat("f", 40)
		assets := skillAccessAssets(t, id, "first", nil)
		acquireSkillAccessCatalog(t, env, directory, assets, id, "init")
		dataRoot := catalogstore.DefaultDataRoot(env["HOME"])
		source := filepath.Join(dataRoot, "catalog/snapshots", id)
		before := snapshotTree(t, source)
		if err := os.Symlink(source, filepath.Join(dataRoot, "skill-access")); err != nil {
			t.Fatal(err)
		}
		out, _, err := runSkillAccessCLI(t, binary, directory, environment, "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
		if err == nil || !bytes.Contains(out, []byte(`"preparation_failed"`)) {
			t.Fatalf("linked preparation: %v %s", err, out)
		}
		assertReport(t, out)
		if snapshotTree(t, source) != before {
			t.Fatal("linked preparation mutated snapshot")
		}
	})
}

func acquireSkillAccessCatalog(t *testing.T, env MapEnv, directory, assets, commit string, args ...string) {
	t.Helper()
	release := catalogstore.Release{Repository: "yersonargotev/packy-catalog", Tag: "catalog-" + commit, Commit: commit, Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true}
	for _, name := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			t.Fatal(err)
		}
		release.Assets = append(release.Assets, catalogstore.Asset{Name: name, Data: data, SHA256: catalogFixtureDigest(data)})
	}
	opts := Options{Env: env, Getwd: func() (string, error) { return directory, nil }, CatalogSource: &catalogSourceFixture{release: release}}
	if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
		t.Fatalf("explicit acquisition: %v %s", err, out)
	}
}

func runSkillAccessCLI(t *testing.T, binary, directory string, environment []string, args ...string) ([]byte, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir, cmd.Env = directory, environment
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.String(), err
}

// The fixture is authored data, validated and packaged by the existing catalog
// boundary. Retrieval itself is always exercised through the public executable.
func skillAccessAssets(t *testing.T, commit, revision string, edit func(map[string]any)) string {
	t.Helper()
	project := t.TempDir()
	fixture := testsupport.SkillVariants("reviewed", testsupport.SurfaceClaude, testsupport.SurfaceCodex)
	if err := fixture.WriteCatalog(project); err != nil {
		t.Fatal(err)
	}
	packRoot := filepath.Join(project, "packs", "reviewed")
	manifestPath := filepath.Join(packRoot, "pack.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["origins"] = []any{}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		delete(resource, "origin")
		if resource["kind"] != "skill" {
			continue
		}
		resource["on_demand"] = map[string]any{"protocol": "skill-tree-v1", "prerequisites": []string{"missing-fixture-runtime"}}
		for _, v := range resource["variants"].([]any) {
			delete(v.(map[string]any), "origin")
		}
	}
	if edit != nil {
		edit(manifest)
	}
	resources := manifest["resources"].([]any)
	skill := resources[1].(map[string]any)
	requires := []string{"asset:guide-data"}
	switch values := skill["requires"].(type) {
	case []any:
		for _, value := range values {
			requires = append(requires, value.(string))
		}
	case []string:
		requires = append(requires, values...)
	}
	sort.Strings(requires)
	skill["requires"] = requires
	resources = append(resources, map[string]any{"kind": "asset", "id": "guide-data", "source": "assets/data.bin", "description": "Supporting binary data", "requires": []string{}, "conflicts": []string{}, "bindings": []any{}, "surface_exclusions": []any{}})
	sort.Slice(resources, func(i, j int) bool {
		a, b := resources[i].(map[string]any), resources[j].(map[string]any)
		return a["kind"].(string)+":"+a["id"].(string) < b["kind"].(string)+":"+b["id"].(string)
	})
	manifest["resources"] = resources
	if err := os.MkdirAll(filepath.Join(packRoot, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packRoot, "assets/data.bin"), []byte{0, 255, 7, 10}, 0644); err != nil {
		t.Fatal(err)
	}
	for _, value := range resources {
		if value.(map[string]any)["id"] != "helper" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(packRoot, "skills/helper"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packRoot, "skills/helper/SKILL.md"), []byte("---\nname: helper\ndescription: Helper.\n---\nHelper\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"common", "claude", "codex"} {
		root := filepath.Join(packRoot, "skills", surface)
		declared := false
		for _, value := range resources {
			resource := value.(map[string]any)
			declared = declared || resource["source"] == "skills/"+surface
			if variants, ok := resource["variants"].([]any); ok {
				for _, v := range variants {
					declared = declared || v.(map[string]any)["source"] == "skills/"+surface
				}
			}
		}
		if !declared {
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			continue
		}
		body := "---\nname: guide\ndescription: Reviewed guide.\n---\n\n" + surface + " " + revision + "\n[Detail](references/detail.md)\n[Notice](../../notices/reviewed-mit)\n"
		if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "scripts/run.sh"), []byte("#!/bin/sh\nprintf executed > \"$HOME/sentinel-executed\"\n"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "scripts/run.sh"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "asset.bin"), []byte{0, 255, 4, 13, 10}, 0644); err != nil {
			t.Fatal(err)
		}
	}
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(t.TempDir(), "assets")
	if _, err := cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog", Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40),
	}, assets); err != nil {
		t.Fatal(err)
	}
	return assets
}
