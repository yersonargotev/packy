package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

const issue961Acme = "acme/packs"

type issue961Source map[string]catalogstore.Release

func (s issue961Source) Release(_ context.Context, registration catalogstore.Registration, _ string) (catalogstore.Release, error) {
	release, ok := s[registration.Repository.String()]
	if !ok {
		return catalogstore.Release{}, fmt.Errorf("no fixture release for %s", registration.Repository)
	}
	return release, nil
}

type issue961Skill struct {
	QualifiedSkill string `json:"qualified_skill"`
	Catalog        string `json:"catalog"`
	PackID         string `json:"pack_id"`
	PackVersion    string `json:"pack_version"`
	NativeName     string `json:"native_name"`
	Description    string `json:"description"`
	Compatible     bool   `json:"compatible"`
	Reason         string `json:"reason"`
}

type issue961Report struct {
	SchemaVersion int             `json:"schema_version"`
	Report        string          `json:"report"`
	Query         string          `json:"query"`
	Surface       string          `json:"surface"`
	Skills        []issue961Skill `json:"skills"`
}

func TestIssue961SkillSearchExecutable(t *testing.T) {
	binary := os.Getenv("PACKY_ISSUE961_BINARY")
	if binary == "" {
		binary = builtPackyExecutable(t)
	}
	repository, _ := filepath.Abs(filepath.Join("..", ".."))
	official, acme := issue961OfficialProject(t), issue961AcmeProject(t)
	officialCommit, acmeCommit := strings.Repeat("a", 40), strings.Repeat("c", 40)
	source := issue961Source{
		catalogstore.Official().Repository.String(): issue961Release(t, catalogstore.Official().Repository.String(), issue961Assets(t, official, catalogstore.Official().Repository.String(), officialCommit), officialCommit),
		issue961Acme: issue961Release(t, issue961Acme, issue961Assets(t, acme, issue961Acme, acmeCommit), acmeCommit),
	}
	environment := testprocess.Env(t, "PATH="+t.TempDir())
	env := adoptionEnvironmentMap(environment)
	target, outside := t.TempDir(), t.TempDir()
	writeTestGitWorktree(t, target)
	opts := Options{Env: env, Getwd: func() (string, error) { return target, nil }, CatalogSource: source, Runner: &fakeRunner{}, Terminal: &fakeTerminal{interactive: true, approve: true}}
	for _, args := range [][]string{{"init"}, {"catalog", "add", issue961Acme}} {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("explicit acquisition %v: %v %s", args, err, out)
		}
	}
	protected := map[string]string{}
	for _, root := range []string{official, acme, target, outside, env["HOME"], env["XDG_CONFIG_HOME"]} {
		protected[root] = snapshotTree(t, root)
	}
	search := func(t *testing.T, args ...string) issue961Report {
		t.Helper()
		out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, append([]string{"search"}, append(args, "--json")...)...)
		if err != nil {
			t.Fatalf("search %v: %v %s %s", args, err, out, stderr)
		}
		assertStructuredOutput(t, repository, "skill-search.schema.json", string(out))
		var report issue961Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatal(err)
		}
		if report.SchemaVersion != 1 || report.Report != "skill-search" || report.Skills == nil {
			t.Fatalf("envelope: %s", out)
		}
		return report
	}
	qualified := func(report issue961Report) []string {
		names := []string{}
		for _, skill := range report.Skills {
			names = append(names, skill.QualifiedSkill)
		}
		return names
	}
	guides := []string{"acme:guide", "other:guide", "reviewed:guide", "third:guide"}

	t.Run("terms match qualified skill, native name, or effective description", func(t *testing.T) {
		for _, test := range []struct {
			surface string
			query   []string
			want    []string
		}{
			{"agents", []string{"guide"}, guides},
			{"agents", []string{"third:sha"}, []string{"third:shared"}},
			{"claude", []string{"board"}, []string{"third:drafts"}},
			{"agents", []string{"board"}, []string{}},
			{"agents", []string{"RELENTLESS"}, []string{"other:grilling", "reviewed:grilling", "third:grilling"}},
			{"agents", []string{"agents", "walkthrough"}, guides},
			{"agents", []string{"claude", "walkthrough"}, []string{}},
			{"claude", []string{"claude", "walkthrough"}, guides},
			{"agents", []string{"walkthrough", "relentless"}, []string{}},
			{"agents", []string{"synthetic"}, []string{}},
			{"agents", []string{"native", "procedure"}, []string{}},
			{"agents", []string{"zanzibar"}, []string{}},
			{"agents", []string{"custom-shared"}, []string{}},
		} {
			report := search(t, append(test.query, "--surface", test.surface)...)
			if got := qualified(report); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("search %v on %s = %v, want %v", test.query, test.surface, got, test.want)
			}
			if report.Query != strings.Join(test.query, " ") || report.Surface != test.surface {
				t.Fatalf("echo: %+v", report)
			}
		}
	})

	t.Run("quoting, case, and whitespace do not change results", func(t *testing.T) {
		separate := search(t, "agents", "walkthrough", "--surface", "agents")
		quoted := search(t, "  Agents\tWALKTHROUGH ", "--surface", "agents")
		if quoted.Query != "Agents WALKTHROUGH" || !reflect.DeepEqual(quoted.Skills, separate.Skills) {
			t.Fatalf("quoted %+v, separate %+v", quoted, separate)
		}
	})

	t.Run("every candidate agrees with show and keeps its catalog", func(t *testing.T) {
		for _, host := range []string{"agents", "claude"} {
			first, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "search", ":", "--surface", host, "--json")
			again, _, againErr := runSkillAccessCLI(t, binary, outside, environment, "search", ":", "--surface", host, "--json")
			if err != nil || againErr != nil || !bytes.Equal(first, again) {
				t.Fatalf("repeated search differs: %v %v %s\n%s\n%s", err, againErr, stderr, first, again)
			}
			report := search(t, ":", "--surface", host)
			want := []issue961Skill{}
			for _, pack := range []string{"acme", "other", "reviewed", "third"} {
				out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "show", pack, "--surface", host, "--json")
				if err != nil {
					t.Fatalf("show %s: %v %s", pack, err, stderr)
				}
				var shown struct {
					Skills []issue961Skill `json:"skills"`
				}
				if err := json.Unmarshal(out, &shown); err != nil {
					t.Fatal(err)
				}
				catalog := catalogstore.Official().Repository.String()
				if pack == "acme" {
					catalog = issue961Acme
				}
				for _, skill := range shown.Skills {
					skill.Catalog = catalog
					want = append(want, skill)
				}
			}
			if !reflect.DeepEqual(report.Skills, want) {
				t.Fatalf("search on %s disagrees with show\ngot:  %+v\nwant: %+v", host, report.Skills, want)
			}
			if !slices.IsSortedFunc(report.Skills, func(a, b issue961Skill) int { return strings.Compare(a.QualifiedSkill, b.QualifiedSkill) }) {
				t.Fatalf("order: %v", qualified(report))
			}
		}
	})

	t.Run("incompatible and excluded candidates carry their reason", func(t *testing.T) {
		agents := search(t, "deployment", "checklist", "--surface", "agents")
		if len(agents.Skills) != 2 {
			t.Fatalf("agents candidates: %+v", agents)
		}
		drafts, legacy := agents.Skills[0], agents.Skills[1]
		if drafts.QualifiedSkill != "third:drafts" || drafts.Compatible || drafts.NativeName != "" || !strings.Contains(drafts.Reason, "Agents is not reviewed") {
			t.Fatalf("excluded: %+v", drafts)
		}
		if legacy.QualifiedSkill != "third:legacy" || legacy.Compatible || !strings.Contains(legacy.Reason, "reviewed on_demand") {
			t.Fatalf("unreviewed: %+v", legacy)
		}
		claude := search(t, "deployment", "checklist", "--surface", "claude")
		if len(claude.Skills) != 2 || !claude.Skills[0].Compatible || claude.Skills[0].NativeName != "draftboard" || claude.Skills[0].Reason != "" || claude.Skills[1].Compatible {
			t.Fatalf("claude candidates: %+v", claude)
		}
	})

	t.Run("readable table", func(t *testing.T) {
		out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "search", "deployment", "--surface", "agents")
		if err != nil {
			t.Fatalf("readable: %v %s", err, stderr)
		}
		lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(lines) != 4 || strings.Join(strings.Fields(lines[0]), " ") != "SKILL CATALOG COMPATIBLE DESCRIPTION" {
			t.Fatalf("table: %s", out)
		}
		for i, want := range [][]string{{"third:drafts", "yersonargotev/packy-catalog", "no", "Draft deployment checklist"}, {"third:legacy", "yersonargotev/packy-catalog", "no", "Retired deployment checklist"}} {
			if got := strings.Join(strings.Fields(lines[i+1]), " "); got != strings.Join(want, " ") {
				t.Fatalf("row %d = %q, want %q", i, got, want)
			}
		}
		if !strings.Contains(lines[3], "unknown requirements may exist") || !strings.Contains(lines[3], "not native registration") {
			t.Fatalf("limitation: %s", out)
		}
		out, _, err = runSkillAccessCLI(t, binary, outside, environment, "search", "guide", "--surface", "claude")
		if err != nil || !strings.Contains(string(out), "acme/packs") || !strings.Contains(string(out), "Claude walkthrough for release notes") || !strings.Contains(string(out), "yes") {
			t.Fatalf("compatible rows: %v %s", err, out)
		}
		out, _, err = runSkillAccessCLI(t, binary, outside, environment, "search", "absent-term", "--surface", "claude")
		if lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); err != nil || len(lines) != 2 || !strings.HasPrefix(lines[0], "SKILL") || !strings.Contains(lines[1], "unknown requirements may exist") {
			t.Fatalf("empty table: %v %s", err, out)
		}
	})

	t.Run("argument and catalog errors print no document", func(t *testing.T) {
		for _, test := range []struct {
			args []string
			want []string
		}{
			{[]string{"--surface", "agents", "--json"}, []string{"query term"}},
			{[]string{" ", "\t", "--surface", "agents", "--json"}, []string{"query term"}},
			{[]string{"guide", "--json"}, []string{"--surface agents", "--surface claude"}},
			{[]string{"guide", "--surface=", "--json"}, []string{"--surface agents", "--surface claude"}},
			{[]string{"guide", "--surface", "opencode", "--json"}, []string{"--surface agents", "--surface claude"}},
			{[]string{"guide", "--surface", "agents", "--unsupported", "--json"}, []string{"unsupported"}},
		} {
			out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, append([]string{"search"}, test.args...)...)
			if err == nil || len(out) != 0 {
				t.Fatalf("search %v: %v %s", test.args, err, out)
			}
			for _, want := range test.want {
				if !strings.Contains(stderr, want) {
					t.Fatalf("search %v diagnostic %q lacks %q", test.args, stderr, want)
				}
			}
		}
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), testprocess.Env(t, "PATH="+t.TempDir()), "search", "guide", "--surface", "agents", "--json")
		if err == nil || len(out) != 0 || !strings.Contains(stderr, "packy init") {
			t.Fatalf("missing catalog: %v %s %s", err, out, stderr)
		}
	})

	assertIssue961Unchanged(t, protected, env)
	before := search(t, "reference", "material", "--surface", "agents")
	for _, verb := range []string{"activate", "install"} {
		if out, err := executeCommand(t, NewRootCommand(opts), verb, "other", "--surface", "agents", "--resource", "skill:shared", "--alias", "skill:shared=custom-shared"); err != nil {
			t.Fatalf("seed %s: %v %s", verb, err, out)
		}
	}
	for root := range protected {
		protected[root] = snapshotTree(t, root)
	}
	if after := search(t, "reference", "material", "--surface", "agents"); !reflect.DeepEqual(after, before) || len(after.Skills) != 3 {
		t.Fatalf("installed state changed candidates\nbefore: %+v\nafter:  %+v", before, after)
	}
	if got := qualified(search(t, "custom-shared", "--surface", "agents")); len(got) != 0 {
		t.Fatalf("alias matched: %v", got)
	}
	assertIssue961Unchanged(t, protected, env)
	t.Run("each catalog's candidate is obtained from its own snapshot", func(t *testing.T) {
		for _, skill := range []string{"acme:guide", "reviewed:guide"} {
			if out, stderr, err := runSkillAccessCLI(t, binary, outside, environment, "get", "skill", skill, "--surface", "agents", "--json"); err != nil {
				t.Fatalf("get %s: %v %s %s", skill, err, out, stderr)
			}
		}
	})
}

func assertIssue961Unchanged(t *testing.T, protected map[string]string, env MapEnv) {
	t.Helper()
	for root, before := range protected {
		if snapshotTree(t, root) != before {
			t.Fatalf("search modified %s", root)
		}
	}
	for _, path := range []string{filepath.Join(env["HOME"], "sentinel-executed"), filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "skill-access")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("search prepared resources or executed content: %s %v", path, err)
		}
	}
}

// issue961OfficialProject gives every skill distinct text in its identity,
// native name, and effective descriptions, so each query targets one field.
func issue961OfficialProject(t *testing.T) string {
	t.Helper()
	project := crossPackAccessProject(t, "search")
	for _, pack := range []string{"other", "reviewed", "third"} {
		editCrossPackManifest(t, project, pack, func(m map[string]any) {
			if pack == "other" {
				// The installed-state seed needs a natively installable shared skill.
				m["external_requirements"] = []any{}
				shared := accessResource(m, "shared")
				shared["variants"], shared["requires"] = []any{}, []any{}
				shared["on_demand"].(map[string]any)["prerequisites"] = []any{}
				for _, host := range []string{"agents", "claude"} {
					if err := os.RemoveAll(filepath.Join(project, "packs/other/skills/shared-"+host)); err != nil {
						t.Fatal(err)
					}
				}
			}
			describeIssue961Skill(m, "guide", "Walkthrough for release notes", "Agents walkthrough for release notes", "Claude walkthrough for release notes")
			describeIssue961Skill(m, "grilling", "Interview the user relentlessly about a plan", "", "")
			describeIssue961Skill(m, "domain-modeling", "Sharpen the ubiquitous language of a codebase", "", "")
			describeIssue961Skill(m, "shared", "Common reference material", "", "")
		})
	}
	for _, host := range []string{"common", "agents", "claude"} {
		path := filepath.Join(project, "packs/reviewed/skills", host, "ACCESS.md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(body, "Body-only zanzibar procedure.\n"...), 0644); err != nil {
			t.Fatal(err)
		}
	}
	editCrossPackManifest(t, project, "third", func(m map[string]any) {
		legacy := cloneIssue961Skill(t, project, m, "legacy", "legacy", "Retired deployment checklist", []string{"agents", "claude"})
		delete(legacy, "on_demand")
		drafts := cloneIssue961Skill(t, project, m, "drafts", "draftboard", "Draft deployment checklist", []string{"claude"})
		drafts["surface_exclusions"] = []any{map[string]any{"surface": "agents", "mode": "optional", "code": "review-needed", "reason": "Agents is not reviewed"}}
		sortDependencyResources(m)
	})
	return project
}

func describeIssue961Skill(m map[string]any, id, common, agents, claude string) {
	resource := accessResource(m, id)
	resource["description"] = common
	for _, value := range resource["variants"].([]any) {
		variant := value.(map[string]any)
		variant["description"] = map[string]string{"agents": agents, "claude": claude}[variant["surface"].(string)]
		if variant["description"] == "" {
			variant["description"] = common
		}
	}
}

// cloneIssue961Skill copies third:shared's common tree into a new skill bound
// only on surfaces, under one native name.
func cloneIssue961Skill(t *testing.T, project string, m map[string]any, id, native, description string, surfaces []string) map[string]any {
	t.Helper()
	encoded, _ := json.Marshal(accessResource(m, "shared"))
	var skill map[string]any
	if err := json.Unmarshal(encoded, &skill); err != nil {
		t.Fatal(err)
	}
	skill["id"], skill["source"], skill["description"], skill["variants"] = id, "skills/"+id, description, []any{}
	skill["requires"] = []any{"asset:guide-data", "skill:shared"}
	skill["bindings"] = slices.DeleteFunc(skill["bindings"].([]any), func(value any) bool {
		return !slices.Contains(surfaces, value.(map[string]any)["surface"].(string))
	})
	for _, value := range skill["bindings"].([]any) {
		binding := value.(map[string]any)
		binding["name"], binding["invocation"] = native, native
		if binding["surface"] == "claude" {
			binding["invocation"] = "/" + native
		}
	}
	pack := filepath.Join(project, "packs/third/skills")
	if err := os.CopyFS(filepath.Join(pack, id), os.DirFS(filepath.Join(pack, "shared"))); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(pack, id, "scripts/run.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(pack, id, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: "+native+"\ndescription: Native procedure.\n---\nNative "+native+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m["resources"] = append(m["resources"].([]any), skill)
	return skill
}

// issue961AcmeProject is a second catalog whose only Pack also names its skill
// guide.
func issue961AcmeProject(t *testing.T) string {
	t.Helper()
	project := skillAccessProject(t, "acme", func(m map[string]any) {
		describeIssue961Skill(m, "guide", "Walkthrough for release notes", "Agents walkthrough for release notes", "Claude walkthrough for release notes")
	})
	if err := os.Rename(filepath.Join(project, "packs/reviewed"), filepath.Join(project, "packs/acme")); err != nil {
		t.Fatal(err)
	}
	editCrossPackManifest(t, project, "acme", func(m map[string]any) { m["id"] = "acme" })
	return project
}

func issue961Assets(t *testing.T, project, repository, commit string) string {
	t.Helper()
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), project, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(t.TempDir(), "assets")
	if _, err := cataloglayout.BuildCatalogSnapshot(context.Background(), project, validation, cataloglayout.CatalogSnapshotSource{Repository: repository, Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40)}, assets); err != nil {
		t.Fatal(err)
	}
	return assets
}

func issue961Release(t *testing.T, repository, assets, commit string) catalogstore.Release {
	t.Helper()
	release := catalogstore.Release{Repository: repository, Tag: "catalog-" + commit, Commit: commit, Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true}
	for _, name := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		data, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			t.Fatal(err)
		}
		release.Assets = append(release.Assets, catalogstore.Asset{Name: name, Data: data, SHA256: catalogFixtureDigest(data)})
	}
	return release
}
