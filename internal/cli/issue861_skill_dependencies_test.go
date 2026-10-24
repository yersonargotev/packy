package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/skillaccess"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue861SkillDependenciesExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	// Both focused slices consume the same executable; the parent owns its
	// lifetime while each child keeps independent catalog and workstation state.
	t.Run("procedures", func(t *testing.T) { checkIssue861SkillDependencies(t, binary) })
	t.Run("invalid dependencies", func(t *testing.T) { checkIssue861InvalidDependencies(t, binary) })
}

func checkIssue861SkillDependencies(t *testing.T, binary string) {
	project := dependencyAccessProject(t, "first", nil)
	assets := dependencyAccessAssets(t, project, strings.Repeat("a", 40))
	workspace := filepath.Join(t.TempDir(), "candidate")
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	if out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "list", "--json"); err != nil {
		t.Fatalf("candidate: %v %s %s", err, out, stderr)
	}
	protected := map[string]string{}
	for _, relative := range []string{"project/AGENTS.md", "project/packy.json", "project/packy.lock.json", "home/.packy/packs.json", "home/.agents/skills/grilling/SKILL.md", "home/.claude/skills/grilling/SKILL.md", "config/foreign.txt"} {
		path := filepath.Join(workspace, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("local sentinel\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, relative := range []string{"project", "config", "snapshots", "home/.packy", "home/.agents", "home/.claude"} {
		protected[relative] = snapshotTree(t, filepath.Join(workspace, relative))
	}

	for _, surface := range []string{"codex", "claude"} {
		args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "get", "skill", "reviewed:guide", "--surface", surface, "--json"}
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, args...)
		if err != nil {
			t.Fatalf("retrieve: %v %s %s", err, out, stderr)
		}
		assertStructuredOutput(t, repository, "skill-access.schema.json", string(out))
		var result skillaccess.Result
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Dependencies) != 3 {
			t.Fatalf("incomplete dependencies: %s", out)
		}
		expected := "Read [grilling](../grilling-" + surface + "/ACCESS.md) then [domain](../domain-modeling-" + surface + "/ACCESS.md). Again [grilling](../grilling-" + surface + "/ACCESS.md).\n[Nested](references/nested.md)\n"
		assertAccessFile(t, result.EntryDocument, []byte(expected), 0644)
		assertAccessFile(t, filepath.Join(result.ResourceRoot, "SKILL.md"), []byte("---\nname: guide\ndescription: Native wrapper.\n---\nNative /{{packy.skill:grilling}} call.\n"), 0644)

		nested := followAccessLink(t, result.EntryDocument, "Nested")
		assertAccessFile(t, nested, []byte("Read [grilling](../../grilling-"+surface+"/ACCESS.md).\n"), 0644)
		if followAccessLink(t, nested, "grilling") != result.Dependencies[1].EntryDocument {
			t.Fatal("nested document resolved to the wrong effective entry")
		}

		for i, id := range []string{"domain-modeling", "grilling", "shared"} {
			dependency := result.Dependencies[i]
			if dependency.QualifiedSkill != "reviewed:"+id || string(dependency.Surface) != surface || dependency.SnapshotID != result.SnapshotID || dependency.PackVersion != "1.0.0" || strings.Join(dependency.Prerequisites, ",") != "missing-fixture-runtime,missing-pack-runtime" {
				t.Fatalf("identity: %+v", dependency)
			}
			body := surface + " " + id + " first\n" + accessSupportingLinks
			if id != "shared" {
				body += "Read [shared](../shared-" + surface + "/ACCESS.md).\n"
			}
			assertAccessFile(t, dependency.EntryDocument, []byte(body), 0644)
			// Follow the authored links from the returned wrapper, including the
			// diamond's two references to the same shared entry.
			if id != "shared" {
				label := "grilling"
				if id == "domain-modeling" {
					label = "domain"
				}
				linked := followAccessLink(t, result.EntryDocument, label)
				assertAccessFile(t, linked, []byte(body), 0644)
				shared := followAccessLink(t, linked, "shared")
				assertAccessFile(t, shared, []byte(surface+" shared first\n"+accessSupportingLinks), 0644)
			}
			assertAccessFile(t, followAccessLink(t, dependency.EntryDocument, "Detail"), []byte("Complete supporting detail for "+id+"\n"), 0644)
			assertAccessFile(t, filepath.Join(dependency.ResourceRoot, "references/detail.md"), []byte("Complete supporting detail for "+id+"\n"), 0644)
			assertAccessFile(t, filepath.Join(dependency.ResourceRoot, "scripts/run.sh"), []byte("#!/bin/sh\n# {{packy.access:do-not-render}} /grilling $grilling Skill(\"grilling\")\nprintf executed > \"$HOME/sentinel-executed\"\n"), 0755)
			assertAccessFile(t, filepath.Join(dependency.ResourceRoot, "../../assets/data.bin"), []byte{0, 255, 7, 10}, 0644)
			source, err := os.ReadFile(filepath.Join(workspace, "snapshots", result.SnapshotID, "packs/reviewed/notices/reviewed-mit"))
			if err != nil {
				t.Fatal(err)
			}
			assertAccessFile(t, filepath.Join(dependency.ResourceRoot, "../../notices/reviewed-mit"), source, 0644)
		}
		again, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, args...)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("reuse: %v %s %s", err, again, stderr)
		}
		for relative, before := range protected {
			if snapshotTree(t, filepath.Join(workspace, relative)) != before {
				t.Fatalf("retrieval modified %s", relative)
			}
		}
		if _, err := os.Stat(filepath.Join(workspace, "home/sentinel-executed")); !os.IsNotExist(err) {
			t.Fatal("retrieval executed a script")
		}
		readableArgs := slices.Clone(args[:len(args)-1])
		readable, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, readableArgs...)
		if err != nil {
			t.Fatalf("readable: %v %s", err, stderr)
		}
		for _, dependency := range result.Dependencies {
			for _, want := range []string{dependency.QualifiedSkill, dependency.EntryDocument, dependency.ResourceRoot} {
				if !bytes.Contains(readable, []byte(want)) {
					t.Fatalf("readable omitted %s", want)
				}
			}
		}
		if err := os.WriteFile(result.Dependencies[0].EntryDocument, []byte("consumer drift\n"), 0644); err != nil {
			t.Fatal(err)
		}
		drift, _, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, args...)
		if err == nil || !bytes.Contains(drift, []byte("preparation_failed")) {
			t.Fatalf("rendered drift accepted: %v %s", err, drift)
		}
		assertAccessFile(t, result.Dependencies[0].EntryDocument, []byte("consumer drift\n"), 0644)

	}
	t.Run("escaped effective source path", func(t *testing.T) {
		project := dependencyAccessProject(t, "escaped", func(m map[string]any) {
			for _, v := range accessResource(m, "shared")["variants"].([]any) {
				variant := v.(map[string]any)
				if variant["surface"] == "codex" {
					variant["source"] = "skills/shared codex#(v1)"
				}
			}
		})
		packRoot := filepath.Join(project, "packs/reviewed")
		if err := os.Rename(filepath.Join(packRoot, "skills/shared-codex"), filepath.Join(packRoot, "skills/shared codex#(v1)")); err != nil {
			t.Fatal(err)
		}
		assets := dependencyAccessAssets(t, project, strings.Repeat("f", 40))
		workspace := filepath.Join(t.TempDir(), "candidate")
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "get", "skill", "reviewed:guide", "--surface", "codex", "--json")
		if err != nil {
			t.Fatalf("escaped source: %v %s %s", err, out, stderr)
		}
		var result skillaccess.Result
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		grilling := followAccessLink(t, result.EntryDocument, "grilling")
		assertAccessFile(t, grilling, []byte("codex grilling escaped\n"+accessSupportingLinks+"Read [shared](../shared%20codex%23%28v1%29/ACCESS.md).\n"), 0644)
		shared := followAccessLink(t, grilling, "shared")
		assertAccessFile(t, shared, []byte("codex shared escaped\n"+accessSupportingLinks), 0644)
	})

	t.Run("retained nested dependency after refresh", func(t *testing.T) {
		envList := testprocess.Env(t, "PATH="+t.TempDir())
		env := adoptionEnvironmentMap(envList)
		directory := t.TempDir()
		firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
		acquireSkillAccessCatalog(t, env, directory, assets, firstID, "init")
		get := func(id, snapshot string) skillaccess.Result {
			t.Helper()
			args := []string{"get", "skill", "reviewed:" + id, "--surface", "codex", "--json"}
			if snapshot != "" {
				args = append(args, "--snapshot", snapshot)
			}
			out, stderr, err := runSkillAccessCLI(t, binary, directory, envList, args...)
			if err != nil {
				t.Fatalf("get: %v %s %s", err, out, stderr)
			}
			var result skillaccess.Result
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
		first := get("guide", "")
		rootBody, err := os.ReadFile(first.EntryDocument)
		if err != nil {
			t.Fatal(err)
		}
		next := dependencyAccessAssets(t, dependencyAccessProject(t, "next", nil), nextID)
		acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
		nested := get("grilling", first.SnapshotID)
		assertAccessFile(t, nested.EntryDocument, []byte("codex grilling first\n"+accessSupportingLinks+"Read [shared](../shared-codex/ACCESS.md).\n"), 0644)
		for _, dependency := range nested.Dependencies {
			if dependency.SnapshotID != firstID {
				t.Fatal("nested snapshot substituted")
			}
		}
		current := get("guide", "")
		if current.SnapshotID != nextID {
			t.Fatal("new retrieval ignored refresh")
		}
		assertAccessFile(t, first.EntryDocument, rootBody, 0644)
		for _, dependency := range first.Dependencies {
			body, err := os.ReadFile(dependency.EntryDocument)
			if err != nil || !bytes.Contains(body, []byte("first")) {
				t.Fatalf("returned dependency lost: %v %s", err, body)
			}
		}
		sourceRoot := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog/snapshots", firstID)
		if err := os.Remove(filepath.Join(sourceRoot, "packs/reviewed/skills/shared-codex/ACCESS.md")); err != nil {
			t.Fatal(err)
		}
		out, _, err := runSkillAccessCLI(t, binary, directory, envList, "get", "skill", "reviewed:grilling", "--surface", "codex", "--snapshot", firstID, "--json")
		if err == nil || !bytes.Contains(out, []byte("snapshot_unavailable")) {
			t.Fatalf("missing retained dependency substituted: %v %s", err, out)
		}
	})

}

func assertAccessFile(t *testing.T, path string, want []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("mode %s: %v", path, err)
	}
}

func dependencyAccessProject(t *testing.T, revision string, edit func(map[string]any)) string {
	t.Helper()
	project := skillAccessProject(t, revision, nil)
	packRoot := filepath.Join(project, "packs/reviewed")
	data, err := os.ReadFile(filepath.Join(packRoot, "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	resources := m["resources"].([]any)
	var guide map[string]any
	for _, v := range resources {
		if v.(map[string]any)["id"] == "guide" {
			guide = v.(map[string]any)
		}
	}
	guide["requires"] = []string{"asset:guide-data", "skill:domain-modeling", "skill:grilling"}
	for _, value := range guide["bindings"].([]any) {
		value.(map[string]any)["capabilities"] = []any{map[string]any{"type": "skill-template", "skill_template": map[string]any{"naming": "native"}}}
	}

	guide["on_demand"].(map[string]any)["entry_document"] = "ACCESS.md"
	guide["on_demand"].(map[string]any)["reference_files"] = []string{"ACCESS.md", "references/nested.md"}
	for _, surface := range []string{"common", "codex", "claude"} {
		nested := filepath.Join(packRoot, "skills", surface, "references/nested.md")
		if err := os.WriteFile(nested, []byte("Read [grilling]({{packy.access:grilling}}).\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packRoot, "skills", surface, "SKILL.md"), []byte("---\nname: guide\ndescription: Native wrapper.\n---\nNative /{{packy.skill:grilling}} call.\n"), 0644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(packRoot, "skills", surface, "ACCESS.md")
		if err := os.WriteFile(path, []byte("Read [grilling]({{packy.access:grilling}}) then [domain]({{packy.access:domain-modeling}}). Again [grilling]({{packy.access:grilling}}).\n[Nested](references/nested.md)\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"domain-modeling", "grilling", "shared"} {
		encoded, _ := json.Marshal(guide)
		var skill map[string]any
		if err := json.Unmarshal(encoded, &skill); err != nil {
			t.Fatal(err)
		}
		skill["id"] = id
		for _, value := range skill["bindings"].([]any) {
			value.(map[string]any)["capabilities"] = []any{}
		}
		skill["source"] = "skills/" + id
		skill["requires"] = []string{"asset:guide-data"}
		if id != "shared" {
			skill["requires"] = []string{"asset:guide-data", "skill:shared"}
		}
		for _, b := range skill["bindings"].([]any) {
			b := b.(map[string]any)
			b["name"] = id
			prefix := "/"
			if b["surface"] == "codex" {
				prefix = "$"
			}
			b["invocation"] = prefix + id
		}
		variants := []any{}
		for _, surface := range []string{"claude", "codex"} {
			source := "skills/" + id + "-" + surface
			variants = append(variants, map[string]any{"surface": surface, "source": source})
		}
		skill["variants"] = variants
		for _, surface := range []string{"common", "claude", "codex"} {
			source := "skills/" + id
			if surface != "common" {
				source += "-" + surface
			}
			root := filepath.Join(packRoot, source)
			for _, dir := range []string{"references", "scripts"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			body := surface + " " + id + " " + revision + "\n" + accessSupportingLinks
			if id != "shared" {
				body += "Read [shared]({{packy.access:shared}}).\n"
			}
			files := map[string]string{"SKILL.md": "---\nname: " + id + "\ndescription: Native procedure.\n---\nNative " + id + "\n", "ACCESS.md": body, "references/detail.md": "Complete supporting detail for " + id + "\n",
				"references/nested.md": "Read [shared]({{packy.access:shared}}).\n", "scripts/run.sh": "#!/bin/sh\n# {{packy.access:do-not-render}} /grilling $grilling Skill(\"grilling\")\nprintf executed > \"$HOME/sentinel-executed\"\n"}
			for path, body := range files {
				mode := os.FileMode(0644)
				if strings.HasSuffix(path, ".sh") {
					mode = 0755
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
		}
		resources = append(resources, skill)
	}
	m["resources"] = resources
	if edit != nil {
		edit(m)
	}
	// Catalog structural ordering is part of the public authoring contract.
	sortDependencyResources(m)
	encoded, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(packRoot, "pack.json"), encoded, 0644); err != nil {
		t.Fatal(err)
	}
	return project
}

func sortDependencyResources(m map[string]any) {
	resources := m["resources"].([]any)
	slices.SortFunc(resources, func(a, b any) int {
		x, y := a.(map[string]any), b.(map[string]any)
		return strings.Compare(x["kind"].(string)+":"+x["id"].(string), y["kind"].(string)+":"+y["id"].(string))
	})
}

func dependencyAccessAssets(t *testing.T, project, id string) string {
	t.Helper()
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(t.TempDir(), "assets")
	_, err = cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: id, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40)}, assets)
	if err != nil {
		t.Fatal(err)
	}
	return assets
}

func checkIssue861InvalidDependencies(t *testing.T, binary string) {
	for _, test := range []struct {
		name, diagnostic string
		edit             func(map[string]any)
		body             string
	}{
		{name: "missing", diagnostic: "missing", edit: func(m map[string]any) { accessResource(m, "guide")["requires"] = []string{"skill:missing"} }},
		{name: "cycle", diagnostic: "cycle", edit: func(m map[string]any) { accessResource(m, "shared")["requires"] = []string{"skill:guide"} }},
		{name: "excluded", diagnostic: "excluded", edit: func(m map[string]any) {
			s := accessResource(m, "grilling")
			s["bindings"] = slices.DeleteFunc(s["bindings"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			s["variants"] = slices.DeleteFunc(s["variants"].([]any), func(v any) bool { return v.(map[string]any)["surface"] == "codex" })
			s["surface_exclusions"] = append(s["surface_exclusions"].([]any), map[string]any{"surface": "codex", "mode": "optional", "code": "unreviewed", "reason": "Not reviewed for Codex"})
		}},
		{name: "incompatible", diagnostic: "reviewed", edit: func(m map[string]any) { delete(accessResource(m, "grilling"), "on_demand") }},
		{name: "undeclared", diagnostic: "direct requires", body: "Read {{packy.access:shared}}\n"},
		{name: "malformed", diagnostic: "invalid logical ID", body: "Read {{packy.access:Not-Valid}}\n"},
		{name: "unterminated", diagnostic: "unterminated", body: "Read {{packy.access:grilling\n"},
		{name: "foreign", diagnostic: "absent", body: "Read {{packy.access:foreign:grilling}}\n"},
		{name: "missing reference", diagnostic: "absent", body: "Read {{packy.access:missing}}\n"},
		{name: "missing reference file", diagnostic: "missing", edit: func(m map[string]any) {
			accessResource(m, "guide")["on_demand"].(map[string]any)["reference_files"] = []string{"missing.md"}
		}},
		{name: "missing entry", diagnostic: "missing", edit: func(m map[string]any) {
			accessResource(m, "guide")["on_demand"].(map[string]any)["entry_document"] = "missing.md"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := dependencyAccessProject(t, "first", test.edit)
			if test.body != "" {
				for _, host := range []string{"common", "claude", "codex"} {
					if err := os.WriteFile(filepath.Join(project, "packs/reviewed/skills", host, "ACCESS.md"), []byte(test.body), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			environment := testprocess.Env(t, "PATH="+t.TempDir())
			out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, "catalog", "validate", "--project", project, "--json")
			if err == nil || !strings.Contains(strings.ToLower(string(out)+stderr), strings.ToLower(test.diagnostic)) {
				t.Fatalf("invalid catalog accepted or wrong diagnostic: %v %s %s", err, out, stderr)
			}

		})
	}
}

func accessResource(m map[string]any, id string) map[string]any {
	for _, v := range m["resources"].([]any) {
		r := v.(map[string]any)
		if r["kind"] == "skill" && r["id"] == id {
			return r
		}
	}
	panic("missing fixture resource " + id)
}

const accessSupportingLinks = "[Detail](references/detail.md) [Notice](../../notices/reviewed-mit) [Asset](../../assets/data.bin) [Script](scripts/run.sh)\n"

func followAccessLink(t *testing.T, entry, label string) string {
	t.Helper()
	body, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`\[` + regexp.QuoteMeta(label) + `\]\(([^)]+)\)`)
	match := pattern.FindSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing %s link in %s: %s", label, entry, body)
	}
	reference, err := url.PathUnescape(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(entry), filepath.FromSlash(reference)))
}
