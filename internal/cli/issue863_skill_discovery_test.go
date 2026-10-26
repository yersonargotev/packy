package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue863SkillDiscoveryExecutable(t *testing.T) {
	binary := os.Getenv("PACKY_ISSUE863_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "packy")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
		build.Dir, build.Env = filepath.Join("..", ".."), testprocess.GoOfflineEnv(t)
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	t.Run("effective metadata and inert discovery", func(t *testing.T) { checkIssue863Discovery(t, binary) })
	t.Run("incompatible candidates and errors", func(t *testing.T) { checkIssue863Failures(t, binary) })
	project := crossPackAccessProject(t, "discovery")
	assets := dependencyAccessAssets(t, project, strings.Repeat("a", 40))
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	directory := t.TempDir() // Outside Git, with no content runtimes or network tools.
	env := adoptionEnvironmentMap(environment)
	acquireSkillAccessCatalog(t, env, directory, assets, strings.Repeat("a", 40), "init")
	out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "show", "reviewed", "--surface", "agents", "--json")
	if err != nil {
		t.Fatalf("discover: %v %s %s", err, out, stderr)
	}
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	assertStructuredOutput(t, repository, "skill-discovery.schema.json", string(out))
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	if report["schema_version"] != float64(1) || report["report"] != "skill-discovery" || report["pack_id"] != "reviewed" || report["pack_version"] != "1.0.0" || report["surface"] != "agents" {
		t.Fatalf("identity: %s", out)
	}
	skills := report["skills"].([]any)
	if len(skills) != 4 {
		t.Fatalf("candidates: %s", out)
	}
	guide := skills[2].(map[string]any)
	if guide["qualified_skill"] != "reviewed:guide" || guide["compatible"] != true || guide["description"] != "Reviewed variant guide" {
		t.Fatalf("guide: %s", out)
	}
	if !reflect.DeepEqual(guide["prerequisites"], []any{"missing-fixture-runtime", "missing-pack-runtime"}) {
		t.Fatalf("prerequisites: %s", out)
	}
	if !reflect.DeepEqual(guide["on_demand"].(map[string]any)["dependencies"], []any{map[string]any{"type": "skill", "target": "other:grilling"}, map[string]any{"type": "skill", "target": "third:grilling"}}) {
		t.Fatalf("access dependencies: %s", out)
	}
	if bytes.Contains(out, []byte("Native wrapper")) {
		t.Fatal("discovery dumped entry content")
	}
}

func checkIssue863Discovery(t *testing.T, binary string) {
	project := crossPackAccessProject(t, "discovery")
	for _, owner := range []string{"reviewed", "other", "third"} {
		editCrossPackManifest(t, project, owner, func(m map[string]any) {
			if owner == "other" {
				m["external_requirements"] = []any{}
			}
			shared := accessResource(m, "shared")
			shared["variants"] = []any{}
			for _, host := range []string{"agents", "claude"} {
				if err := os.RemoveAll(filepath.Join(project, "packs", owner, "skills", "shared-"+host)); err != nil {
					t.Fatal(err)
				}
			}
			shared["requires"] = []any{} // Native setup needs no runtime or projected asset.
			shared["on_demand"].(map[string]any)["prerequisites"] = []any{}
			guide := accessResource(m, "guide")
			for _, v := range guide["variants"].([]any) {
				variant := v.(map[string]any)
				host := variant["surface"].(string)
				dependency, foreign := "grilling", "other:grilling"
				if host == "claude" {
					dependency, foreign = "domain-modeling", "third:grilling"
				}
				variant["description"] = host + " discovery procedure"
				variant["requires"] = []any{"asset:guide-data", "skill:" + dependency}
				native := "---\nname: guide\ndescription: Native wrapper.\n---\nNative /{{packy.skill:" + dependency + "}} call.\n"
				if err := os.WriteFile(filepath.Join(project, "packs", owner, "skills", host, "SKILL.md"), []byte(native), 0644); err != nil {
					t.Fatal(err)
				}
				variant["on_demand"] = map[string]any{"protocol": "skill-tree-v1", "entry_document": "ACCESS.md", "prerequisites": []any{host + "-runtime"}, "dependencies": []any{map[string]any{"type": "skill", "target": foreign}}}
			}
		})
	}
	revision := strings.Repeat("b", 40)
	assets := dependencyAccessAssets(t, project, revision)
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	env := adoptionEnvironmentMap(environment)
	target, outside := t.TempDir(), t.TempDir()
	writeTestGitWorktree(t, target)
	acquireSkillAccessCatalog(t, env, target, assets, revision, "init")
	opts := Options{Env: env, Getwd: func() (string, error) { return target, nil }, Runner: &fakeRunner{}, Terminal: &fakeTerminal{interactive: true, approve: true}}
	for _, verb := range []string{"activate", "install"} {
		if out, err := executeCommand(t, NewRootCommand(opts), verb, "other", "--surface", "agents", "--resource", "skill:shared", "--alias", "skill:shared=custom-shared"); err != nil {
			t.Fatalf("seed %s: %v %s", verb, err, out)
		}
	}
	installed := filepath.Join(env["HOME"], ".agents/skills/custom-shared")
	if err := os.WriteFile(filepath.Join(installed, "SKILL.md"), []byte("Modified local instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	protected := map[string]string{}
	for _, root := range []string{project, target, env["HOME"], env["XDG_CONFIG_HOME"]} {
		protected[root] = snapshotTree(t, root)
	}
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	list, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "list", "--json")
	if err != nil {
		t.Fatalf("list: %v %s", err, stderr)
	}
	assertStructuredOutput(t, repository, "pack-list.schema.json", string(list))
	var listed struct {
		Packs []packListEntryJSON `json:"packs"`
	}
	if err := json.Unmarshal(list, &listed); err != nil {
		t.Fatal(err)
	}
	wantPacks := []packListEntryJSON{
		{ID: "other", Version: "2.0.0", Description: "Synthetic reviewed Pack fixture", Surfaces: []capabilitypack.Surface{"agents", "claude"}},
		{ID: "reviewed", Version: "1.0.0", Description: "Synthetic reviewed Pack fixture", Surfaces: []capabilitypack.Surface{"agents", "claude"}},
		{ID: "third", Version: "2.0.0", Description: "Synthetic reviewed Pack fixture", Surfaces: []capabilitypack.Surface{"agents", "claude"}},
	}
	if !reflect.DeepEqual(listed.Packs, wantPacks) {
		t.Fatalf("current Packs: %s", list)
	}
	for _, host := range []string{"agents", "claude"} {
		candidates := []string{}
		for _, owner := range []string{"other", "reviewed", "third"} {
			out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "show", owner, "--surface", host, "--json")
			if err != nil {
				t.Fatalf("show: %v %s %s", err, out, stderr)
			}
			assertStructuredOutput(t, repository, "skill-discovery.schema.json", string(out))
			var report capabilitypack.SkillDiscoveryReport
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatal(err)
			}
			version := "2.0.0"
			if owner == "reviewed" {
				version = "1.0.0"
			}
			if report.PackID != owner || report.PackVersion != version || string(report.Surface) != host || report.PrerequisitesVerified || !strings.Contains(report.Limitation, "unknown requirements may exist") || len(report.Skills) != 4 {
				t.Fatalf("report: %s", out)
			}
			dependency, foreign := "grilling", "other:grilling"
			if host == "claude" {
				dependency, foreign = "domain-modeling", "third:grilling"
			}
			prereqs := []string{host + "-runtime"}
			if owner != "other" {
				prereqs = append(prereqs, "missing-pack-runtime")
			}
			notice := []capabilitypack.ResourceIdentity{{Kind: "notice", ID: "mit"}}
			resource := capabilitypack.ResourceIdentity{Kind: "skill", ID: "guide"}
			expected := capabilitypack.SkillDiscovery{
				DescriptiveResource: capabilitypack.DescriptiveResource{Resource: resource, Description: host + " discovery procedure", Role: "operational", Dependencies: []capabilitypack.ResourceIdentity{{Kind: "asset", ID: "guide-data"}, {Kind: "skill", ID: dependency}}, Notices: notice},
				QualifiedSkill:      owner + ":guide", PackID: owner, PackVersion: version,
				Definition:        capabilitypack.ResourceDefinition{Resource: resource, Surface: capabilitypack.Surface(host), Definition: "surface_variant", Source: "skills/" + host, Notices: notice},
				SupportedSurfaces: []capabilitypack.Surface{"agents", "claude"}, NativeName: "guide", OnDemand: &capabilitypack.OnDemandSkill{Protocol: "skill-tree-v1", EntryDocument: "ACCESS.md", Prerequisites: []string{host + "-runtime"}, Dependencies: []capabilitypack.OnDemandDependency{{Type: "skill", Target: foreign}}}, Compatible: true, Prerequisites: prereqs,
			}
			if !reflect.DeepEqual(report.Skills[2], expected) {
				t.Fatalf("complete effective guide metadata mismatch\ngot: %+v\nwant: %+v\n%s", report.Skills[2], expected, out)
			}
			// All duplicate native guide names remain visible, independent of installed state.
			candidates = append(candidates, report.Skills[2].QualifiedSkill)
			common := report.Skills[3]
			commonPrereqs := []string{}
			if owner != "other" {
				commonPrereqs = []string{"missing-pack-runtime"}
			}
			expectedCommon := capabilitypack.SkillDiscovery{
				DescriptiveResource: capabilitypack.DescriptiveResource{Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "shared"}, Description: "Reviewed variant guide", Role: "operational", Dependencies: []capabilitypack.ResourceIdentity{}, Notices: notice},
				QualifiedSkill:      owner + ":shared", PackID: owner, PackVersion: version,
				Definition:        capabilitypack.ResourceDefinition{Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "shared"}, Surface: capabilitypack.Surface(host), Definition: "common", Source: "skills/shared", Notices: notice},
				SupportedSurfaces: []capabilitypack.Surface{"agents", "claude"}, NativeName: "shared", OnDemand: &capabilitypack.OnDemandSkill{Protocol: "skill-tree-v1", EntryDocument: "ACCESS.md", ReferenceFiles: []string{"ACCESS.md", "references/nested.md"}, Prerequisites: []string{}}, Compatible: true, Prerequisites: commonPrereqs,
			}
			if !reflect.DeepEqual(common, expectedCommon) {
				t.Fatalf("complete common candidate: %+v, want %+v", common, expectedCommon)
			}
			readable, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "show", owner, "--surface", host)
			if err != nil {
				t.Fatalf("readable: %v %s", err, stderr)
			}
			for _, want := range []string{owner + ":guide", version, host + " discovery procedure", "surface variant", "common definition", foreign, "skill:" + dependency, host + "-runtime", "not verified", "unknown requirements may exist"} {
				if !bytes.Contains(readable, []byte(want)) {
					t.Fatalf("readable omitted %s: %s", want, readable)
				}
			}
		}
		if !reflect.DeepEqual(candidates, []string{"other:guide", "reviewed:guide", "third:guide"}) {
			t.Fatalf("ambiguous candidates: %v", candidates)
		}
	}
	for root, before := range protected {
		if snapshotTree(t, root) != before {
			t.Fatalf("discovery modified %s", root)
		}
	}
	for _, path := range []string{filepath.Join(env["HOME"], "sentinel-executed"), filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "skill-access")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("discovery prepared resources or executed content: %s %v", path, err)
		}
	}
	// Retrieval must report the same effective root requirements for each host.
	for _, host := range []string{"agents", "claude"} {
		out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "get", "skill", "reviewed:guide", "--surface", host, "--json")
		if err != nil {
			t.Fatalf("retrieval consistency: %v %s %s", err, out, stderr)
		}
		var access struct {
			Prerequisites []string `json:"prerequisites"`
			EntryDocument string   `json:"entry_document"`
		}
		if err := json.Unmarshal(out, &access); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(access.Prerequisites, []string{host + "-runtime", "missing-pack-runtime"}) || !strings.HasSuffix(filepath.ToSlash(access.EntryDocument), "/skills/"+host+"/ACCESS.md") {
			t.Fatalf("retrieval mismatch: %s", out)
		}
	}
}

func checkIssue863Failures(t *testing.T, binary string) {
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	for _, test := range []struct {
		name, reason string
		edit         func(map[string]any)
	}{
		{"unreviewed", "reviewed on_demand", func(m map[string]any) { delete(accessResource(m, "guide"), "on_demand") }},
		{"excluded", "Agents is not reviewed", func(m map[string]any) {
			r := accessResource(m, "guide")
			b := r["bindings"].([]any)
			r["bindings"] = slices.DeleteFunc(b, func(value any) bool { return value.(map[string]any)["surface"] == "agents" })
			r["variants"] = slices.DeleteFunc(r["variants"].([]any), func(value any) bool { return value.(map[string]any)["surface"] == "agents" })
			r["surface_exclusions"] = []any{map[string]any{"surface": "agents", "mode": "optional", "code": "review-needed", "reason": "Agents is not reviewed"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assets := skillAccessAssets(t, strings.Repeat("c", 40), "discovery", test.edit)
			workspace := filepath.Join(t.TempDir(), "candidate")
			env := testprocess.Env(t, "PATH="+t.TempDir())
			out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), env, "catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "show", "reviewed", "--surface", "agents", "--json")
			if err != nil {
				t.Fatalf("inspect incompatible: %v %s %s", err, out, stderr)
			}
			assertStructuredOutput(t, repository, "skill-discovery.schema.json", string(out))
			var report capabilitypack.SkillDiscoveryReport
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Skills) != 1 || report.Skills[0].QualifiedSkill != "reviewed:guide" || report.Skills[0].Compatible || !strings.Contains(report.Skills[0].Reason, test.reason) {
				t.Fatalf("incompatible candidate: %s", out)
			}
			if test.name == "excluded" && (report.Skills[0].Exclusion == nil || report.Skills[0].Exclusion.Code != "review-needed" || report.Skills[0].NativeName != "") {
				t.Fatalf("exclusion: %s", out)
			}
			if test.name == "unreviewed" && report.Skills[0].OnDemand != nil {
				t.Fatalf("unreviewed contract: %s", out)
			}
			for _, args := range [][]string{{"show", "reviewed", "--surface", "opencode", "--json"}, {"show", "reviewed", "--surface", "unknown", "--json"}, {"show", "reviewed", "--surface=", "--json"}, {"show", "absent", "--surface", "agents", "--json"}, {"show", "--surface", "agents", "--json"}, {"show", "reviewed", "--surface", "agents", "--unsupported", "--json"}} {
				command := append([]string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}, args...)
				stdout, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), env, command...)
				if err == nil || len(stdout) != 0 || stderr == "" {
					t.Fatalf("invalid request %v: %v %s %s", args, err, stdout, stderr)
				}
				if len(args) > 3 && (args[3] == "opencode" || args[3] == "unknown") && !strings.Contains(stderr, "--surface agents") {
					t.Fatalf("host remediation: %s", stderr)
				}
			}
		})
	}
	for _, args := range [][]string{{"list", "--json"}, {"show", "reviewed", "--surface", "agents", "--json"}} {
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), args...)
		if err == nil || len(out) != 0 || !strings.Contains(stderr, "packy init") {
			t.Fatalf("missing catalog: %v %s %s", err, out, stderr)
		}
	}
}
