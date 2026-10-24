package ci_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/skillaccess"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func testCatalogOnDemandExecutable(t *testing.T, executable string) {
	executableBefore := catalogTreeState(t, []string{executable})
	t.Cleanup(func() {
		if !reflect.DeepEqual(executableBefore, catalogTreeState(t, []string{executable})) {
			t.Error("supplied executable bytes or mode changed")
		}
	})

	t.Run("missing catalog", func(t *testing.T) {
		environment := testprocess.Env(t, "PATH="+t.TempDir())
		directory := t.TempDir()
		for _, host := range []string{"claude", "codex"} {
			out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, "get", "skill", "alpha:root", "--surface", host, "--json")
			assertAccessFailure(t, out, diagnostic, err, "catalog_unavailable", "packy init")
			out, diagnostic, err = runCatalogCLI(t, environment, directory, executable, "show", "alpha", "--surface", host, "--json")
			if err == nil || !bytes.Contains(append(out, diagnostic...), []byte("packy init")) {
				t.Fatalf("missing discovery: %v %s %s", err, out, diagnostic)
			}
		}
	})
	commit, proxy, ca := catalogGitOrigin(t)
	buildEnvironment := testprocess.Env(t, "HTTPS_PROXY="+proxy, "SSL_CERT_FILE="+ca)
	firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
	project := writeAccessCatalog(t, "first", commit)
	assets, archive := buildAccessCatalog(t, executable, buildEnvironment, project, firstID)
	t.Run("non-Git boundary", func(t *testing.T) { testCandidateAccessWithoutGit(t, executable, assets) })
	nextProject := writeAccessCatalog(t, "next", commit)
	nextAssets, _ := buildAccessCatalog(t, executable, buildEnvironment, nextProject, nextID)
	for _, host := range []string{"claude", "codex"} {
		t.Run(host, func(t *testing.T) {
			testAccessProcedure(t, executable, host, project, nextProject, assets, nextAssets, archive, firstID, nextID)
		})
	}
	t.Run("incompatible discovery candidates", func(t *testing.T) { testAccessIncompatibleDiscovery(t, executable, buildEnvironment, commit) })
	t.Run("invalid complete catalogs", func(t *testing.T) {
		testAccessCatalogFailures(t, executable, buildEnvironment, project, assets, archive)
	})
}

func testAccessProcedure(t *testing.T, executable, host, project, nextProject, assets, nextAssets, archive, firstID, nextID string) {
	var requests atomic.Int32
	offline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "offline contract", http.StatusBadGateway)
	}))
	defer offline.Close()
	environment := testprocess.Env(t, "PATH="+t.TempDir(), "HTTPS_PROXY="+offline.URL, "HTTP_PROXY="+offline.URL)
	directory := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	current := assets
	prefix := func() []string {
		return []string{"catalog", "candidate", "--snapshot", current, "--workspace", workspace, "--"}
	}
	run := func(args ...string) ([]byte, []byte, error) {
		return runCatalogCLI(t, environment, directory, executable, append(prefix(), args...)...)
	}
	mustRun := func(args ...string) []byte {
		t.Helper()
		out, diagnostic, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v %s %s", args, err, out, diagnostic)
		}
		if !bytes.Contains(diagnostic, []byte("not an official Catalog Publication")) {
			t.Fatalf("missing provenance diagnostic: %s", diagnostic)
		}
		return out
	}
	mustRun("list", "--json")
	// Real native installation and activation establish receipt and alias state.
	alias := "custom-" + host + "-leaf"
	for _, verb := range []string{"install", "activate"} {
		args := append(prefix(), verb, "beta", "--surface", host, "--resource", "skill:leaf", "--alias", "skill:leaf="+alias)
		if out, err := runCandidatePTY(t, environment, directory, executable, "y", args...); err != nil {
			t.Fatalf("seed %s: %v %s", verb, err, out)
		}
	}
	native := ".agents"
	if host == "claude" {
		native = ".claude"
	}
	installed := filepath.Join(workspace, "home", native, "skills", alias)
	// Replace the native link, preserving its immutable catalog target.
	if err := os.Remove(installed); err != nil {
		t.Fatal(err)
	}
	writeAccessFile(t, filepath.Join(installed, "SKILL.md"), []byte("Modified installed procedure\n"), 0600)
	writeAccessFile(t, filepath.Join(workspace, "project", native, "skills", alias, "SKILL.md"), []byte("Modified project procedure\n"), 0600)
	writeAccessFile(t, filepath.Join(workspace, "project", "unrelated.txt"), []byte("Unrelated project bytes\n"), 0640)
	protectedRoots := []string{project, assets, directory, filepath.Join(workspace, "project"), filepath.Join(workspace, "config"), filepath.Join(workspace, "snapshots"), filepath.Join(workspace, "home")}
	protected := accessProtectedState(t, workspace, protectedRoots)
	list := mustRun("list", "--json")
	validateCatalogCLISchema(t, "pack-list", list)
	var listed struct {
		Packs []struct {
			ID, Version, Description string
			Surfaces                 []string
		}
	}
	if err := json.Unmarshal(list, &listed); err != nil {
		t.Fatal(err)
	}
	wantList := `{"Packs":[{"ID":"alpha","Version":"1.0.0","Description":"Access contract alpha","Surfaces":["claude","codex"]},{"ID":"beta","Version":"2.0.0","Description":"Access contract beta","Surfaces":["claude","codex"]},{"ID":"zulu","Version":"2.0.0","Description":"Access contract zulu","Surfaces":["claude","codex"]}]}`
	var expectedList any
	var gotList any
	if err := json.Unmarshal([]byte(wantList), &expectedList); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(listed)
	if err := json.Unmarshal(encoded, &gotList); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expectedList, gotList) {
		t.Fatalf("complete list: %s", list)
	}
	readableList := mustRun("list")
	wantReadableList := "PACK VERSION DESCRIPTION AVAILABLE ON alpha 1.0.0 Access contract alpha claude, codex beta 2.0.0 Access contract beta claude, codex zulu 2.0.0 Access contract zulu claude, codex"
	if strings.Join(strings.Fields(string(readableList)), " ") != wantReadableList {
		t.Fatalf("complete readable list: %s", readableList)
	}
	for _, pack := range []string{"alpha", "beta", "zulu"} {
		out := mustRun("show", pack, "--surface", host, "--json")
		validateCatalogCLISchema(t, "skill-discovery", out)
		var report capabilitypack.SkillDiscoveryReport
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatal(err)
		}
		prerequisites := []string{"missing-content-runtime"}
		if pack != "beta" {
			prerequisites = append(prerequisites, "missing-pack-runtime")
		}
		version := "2.0.0"
		if pack == "alpha" {
			version = "1.0.0"
		}
		if report.PackID != pack || report.PackVersion != version || string(report.Surface) != host || report.PrerequisitesVerified || !strings.Contains(report.Limitation, "unknown requirements may exist") || len(report.Skills) != 4 {
			t.Fatalf("discovery report: %s", out)
		}
		for i, id := range []string{"leaf", "left", "right", "root"} {
			deps := []capabilitypack.ResourceIdentity{{Kind: "asset", ID: "data"}}
			if id == "leaf" {
				deps = []capabilitypack.ResourceIdentity{}
			}
			var foreign []capabilitypack.OnDemandDependency
			if id == "root" {
				deps = append(deps, capabilitypack.ResourceIdentity{Kind: "skill", ID: "left"}, capabilitypack.ResourceIdentity{Kind: "skill", ID: "right"})
			}
			if id == "left" || id == "right" {
				deps = append(deps, capabilitypack.ResourceIdentity{Kind: "skill", ID: "leaf"})
				if pack == "alpha" {
					foreign = append(foreign, capabilitypack.OnDemandDependency{Type: "skill", Target: "beta:leaf"})
				}
			}
			prerequisites := []string{"missing-content-runtime"}
			if pack != "beta" {
				prerequisites = append(prerequisites, "missing-pack-runtime")
			}
			notice := []capabilitypack.ResourceIdentity{{Kind: "notice", ID: "mit"}}
			resource := capabilitypack.ResourceIdentity{Kind: "skill", ID: id}
			expected := capabilitypack.SkillDiscovery{
				DescriptiveResource: capabilitypack.DescriptiveResource{Resource: resource, Description: host + " " + id + " procedure", Role: "operational", Dependencies: deps, Notices: notice},
				QualifiedSkill:      pack + ":" + id, PackID: pack, PackVersion: version,
				Definition:        capabilitypack.ResourceDefinition{Resource: resource, Surface: capabilitypack.Surface(host), Definition: "surface_variant", Source: "skills/" + id + "-" + host, Origin: &capabilitypack.ResourceOrigin{ID: "upstream", Path: "skill", Relationship: "adapted"}, Notices: notice},
				SupportedSurfaces: []capabilitypack.Surface{"claude", "codex"}, NativeName: id,
				OnDemand: &capabilitypack.OnDemandSkill{Protocol: "skill-tree-v1", EntryDocument: "ACCESS.md", ReferenceFiles: []string{"ACCESS.md"}, Prerequisites: []string{"missing-content-runtime"}, Dependencies: foreign}, Compatible: true, Prerequisites: prerequisites,
			}
			if !reflect.DeepEqual(report.Skills[i], expected) {
				gotJSON, _ := json.Marshal(report.Skills[i])
				wantJSON, _ := json.Marshal(expected)
				t.Fatalf("%s metadata\ngot %s\nwant %s", pack+":"+id, gotJSON, wantJSON)
			}
		}
		if bytes.Contains(out, []byte("Native fixture")) || bytes.Contains(out, []byte("Complete alpha")) {
			t.Fatal("discovery dumped instructions")
		}
		readable := mustRun("show", pack, "--surface", host)
		if string(readable) != expectedAccessDiscovery(pack, host, version) {
			t.Fatalf("complete readable discovery: %s", readable)
		}
	}
	get := func(qualified, snapshot string) skillaccess.Result {
		t.Helper()
		args := []string{"get", "skill", qualified, "--surface", host, "--json"}
		if snapshot != "" {
			args = append(args, "--snapshot", snapshot)
		}
		out := mustRun(args...)
		validateCatalogCLISchema(t, "skill-access", out)
		var result skillaccess.Result
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := get("alpha:root", "")
	verifyAccessProcedure(t, first, host, firstID, "first", project, []string{"alpha:leaf", "alpha:left", "alpha:right", "beta:leaf"})
	for _, qualified := range []string{"alpha:leaf", "beta:root"} {
		result := get(qualified, "")
		dependencies := []string{}
		if qualified == "beta:root" {
			dependencies = []string{"beta:leaf", "beta:left", "beta:right"}
		}
		verifyAccessProcedure(t, result, host, firstID, "first", project, dependencies)
	}
	again := get("alpha:root", "")
	if !reflect.DeepEqual(first, again) {
		t.Fatal("reuse changed result")
	}
	readable := mustRun("get", "skill", "alpha:root", "--surface", host)
	wantReadable := fmt.Sprintf("Prepared skill alpha:root (Pack 1.0.0, surface %s)\nCatalog Snapshot: %s\nEntry document: %s\nResource root: %s\nKnown prerequisites (not verified): [missing-content-runtime missing-pack-runtime]\nResources only: no native registration or workflow execution. Check prerequisites and follow the skill's procedure and approvals; use separate target and writable scratch/output locations.\n", host, firstID, first.EntryDocument, first.ResourceRoot)
	for _, dependency := range first.Dependencies {
		prerequisites := "missing-content-runtime missing-pack-runtime"
		if dependency.QualifiedSkill == "beta:leaf" {
			prerequisites = "missing-content-runtime"
		}
		wantReadable += fmt.Sprintf("Dependency: %s (Pack %s, surface %s, snapshot %s)\n  Entry document: %s\n  Resource root: %s\n  Known prerequisites (not verified): [%s]\n", dependency.QualifiedSkill, dependency.PackVersion, host, firstID, dependency.EntryDocument, dependency.ResourceRoot, prerequisites)
	}
	if string(readable) != wantReadable {
		t.Fatalf("complete readable access: %s", readable)
	}
	if !reflect.DeepEqual(protected, accessProtectedState(t, workspace, protectedRoots)) {
		t.Fatal("discovery/retrieval changed project, installed state, sources, or catalog assets")
	}
	if _, err := os.Stat(filepath.Join(workspace, "home", "access-executed")); !os.IsNotExist(err) {
		t.Fatal("catalog script executed")
	}
	// Refresh is explicit and cannot rewrite native state or returned resources.
	retained := catalogTreeState(t, []string{filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(first.ResourceRoot))))})
	installedRoots := []string{filepath.Join(workspace, "project"), filepath.Join(workspace, "config"), filepath.Join(workspace, "home")}
	installedBefore := accessProtectedState(t, workspace, installedRoots)
	current = nextAssets
	transition := mustRun("transition", "--expect-archive-sha256", archive, "--json")
	validateCatalogCLISchema(t, "catalog-candidate-transition", transition)
	if !bytes.Contains(transition, []byte(nextID)) || !reflect.DeepEqual(installedBefore, accessProtectedState(t, workspace, installedRoots)) {
		t.Fatalf("transition altered native state: %s", transition)
	}
	nested := get("alpha:left", firstID)
	verifyAccessProcedure(t, nested, host, firstID, "first", project, []string{"alpha:leaf", "beta:leaf"})
	currentResult := get("alpha:root", "")
	verifyAccessProcedure(t, currentResult, host, nextID, "next", nextProject, []string{"alpha:leaf", "alpha:left", "alpha:right", "beta:leaf"})
	if currentResult.SnapshotID != nextID {
		t.Fatal("new independent request ignored selection")
	}
	assertAccessBytes(t, filepath.Join(currentResult.ResourceRoot, "references/detail.md"), []byte("Complete alpha "+host+" root next\n"), 0644)
	for path, want := range retained {
		if got := catalogTreeState(t, []string{path})[path]; got != want {
			t.Fatalf("returned resource changed after refresh: %s", path)
		}
	}
	// Prepared content is not a fallback for missing or invalid retained sources.
	source := filepath.Join(workspace, "snapshots", firstID, "packs/beta/skills/leaf-"+host, "ACCESS.md")
	sourceBytes := readIssue798File(t, source)
	for _, damage := range []string{"missing", "invalid"} {
		if damage == "missing" {
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
		} else {
			writeAccessFile(t, source, []byte("corrupted retained content\n"), 0644)
		}
		before := catalogTreeState(t, []string{workspace})
		out, diagnostic, err := run("get", "skill", "alpha:left", "--surface", host, "--snapshot", firstID, "--json")
		if err == nil || bytes.Contains(out, []byte(`"prepared"`)) || !strings.Contains(string(out)+string(diagnostic), "Catalog Snapshot") {
			t.Fatalf("%s retained source substituted: %v %s %s", damage, err, out, diagnostic)
		}
		if !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
			t.Fatal("failed retained access changed state")
		}
		assertAccessBytes(t, first.EntryDocument, []byte("alpha "+host+" root first\n"+accessContractLinks+"[Left](../left-"+host+"/ACCESS.md) [Right](../right-"+host+"/ACCESS.md) [Again](../left-"+host+"/ACCESS.md)\n"), 0644)
		writeAccessFile(t, source, sourceBytes, 0644)
	}
	get("alpha:root", "")
	if requests.Load() != 0 {
		t.Fatalf("access attempted %d implicit network calls", requests.Load())
	}
}

func assertAccessBytes(t *testing.T, path string, want []byte, mode os.FileMode) {
	t.Helper()
	got := readIssue798File(t, path)
	info, err := os.Stat(path)
	if !bytes.Equal(got, want) || err != nil || info.Mode().Perm() != mode {
		t.Fatalf("resource %s bytes/mode: %q %v %v; want %q %v", path, got, info, err, want, mode)
	}
}

func verifyAccessProcedure(t *testing.T, result skillaccess.Result, host, snapshot, revision, project string, wantDependencies []string) {
	t.Helper()
	identities := []string{}
	for _, d := range result.Dependencies {
		identities = append(identities, d.QualifiedSkill)
	}
	if !slices.Equal(identities, wantDependencies) {
		t.Fatalf("closure %s: %v want %v", result.QualifiedSkill, identities, wantDependencies)
	}
	entries := map[string]string{}
	expectedFiles := map[string]bool{}
	packAssets := map[string]bool{}
	for _, skill := range append([]skillaccess.Skill{result.Skill}, result.Dependencies...) {
		parts := strings.Split(skill.QualifiedSkill, ":")
		pack, id := parts[0], parts[1]
		prerequisites := []string{"missing-content-runtime"}
		if pack != "beta" {
			prerequisites = append(prerequisites, "missing-pack-runtime")
		}
		version := "2.0.0"
		if revision == "next" {
			version = "2.1.0"
		}
		if pack == "alpha" {
			version = "1.0.0"
			if revision == "next" {
				version = "1.1.0"
			}
		}
		if skill.PackVersion != version || string(skill.Surface) != host || skill.SnapshotID != snapshot || !slices.Equal(skill.Prerequisites, prerequisites) || !filepath.IsAbs(skill.ResourceRoot) || skill.EntryDocument != filepath.Join(skill.ResourceRoot, "ACCESS.md") {
			t.Fatalf("identity: %+v", skill)
		}
		body := pack + " " + host + " " + id + " " + revision + "\n" + accessContractLinks
		if id == "root" {
			body += "[Left](../left-" + host + "/ACCESS.md) [Right](../right-" + host + "/ACCESS.md) [Again](../left-" + host + "/ACCESS.md)\n"
		}
		if id == "left" || id == "right" {
			body += "[Leaf](../leaf-" + host + "/ACCESS.md)\n"
			if pack == "alpha" {
				body += "[Foreign](../../../beta/skills/leaf-" + host + "/ACCESS.md)\n"
			}
		}
		assertAccessBytes(t, skill.EntryDocument, []byte(body), 0644)
		assertAccessBytes(t, filepath.Join(skill.ResourceRoot, "SKILL.md"), []byte("---\nname: "+id+"\ndescription: Native fixture.\n---\nNative "+pack+" "+id+"\n"), 0644)
		assertAccessBytes(t, filepath.Join(skill.ResourceRoot, "references/detail.md"), []byte("Complete "+pack+" "+host+" "+id+" "+revision+"\n"), 0644)
		assertAccessBytes(t, filepath.Join(skill.ResourceRoot, "scripts/run.sh"), []byte(accessContractScript), 0755)
		packRoot := filepath.Join(skill.ResourceRoot, "../..")
		assertAccessBytes(t, filepath.Join(packRoot, "pack.json"), readIssue798File(t, filepath.Join(project, "packs", pack, "pack.json")), 0644)
		assertAccessBytes(t, filepath.Join(skill.ResourceRoot, "assets/data.bin"), []byte{0, 255, 7, 10}, 0644)
		assertAccessBytes(t, filepath.Join(packRoot, "notices/mit"), []byte("MIT License\nFixture Authors\n"), 0644)
		for _, relative := range []string{"SKILL.md", "ACCESS.md", "references/detail.md", "scripts/run.sh", "assets/data.bin"} {
			expectedFiles[filepath.Join(skill.ResourceRoot, relative)] = true
		}
		for _, relative := range []string{"pack.json", "notices/mit"} {
			expectedFiles[filepath.Clean(filepath.Join(packRoot, relative))] = true
		}
		if id != "leaf" {
			packAssets[filepath.Clean(packRoot)] = true
		}
		entries[skill.QualifiedSkill] = skill.EntryDocument
	}
	for root := range packAssets {
		path := filepath.Join(root, "assets/data.bin")
		assertAccessBytes(t, path, []byte{0, 255, 7, 10}, 0644)
		expectedFiles[path] = true
	}
	procedureRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(result.ResourceRoot))))
	actualFiles := map[string]bool{}
	if err := filepath.WalkDir(procedureRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			actualFiles[path] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expectedFiles, actualFiles) {
		t.Fatalf("complete prepared closure differs: got %v want %v", actualFiles, expectedFiles)
	}
	// Follow actual returned Markdown links, including repeated and diamond edges.
	linkPattern := regexp.MustCompile(`\[(Left|Right|Again|Leaf|Foreign)\]\(([^)]+)\)`)
	for qualified, entry := range entries {
		pack := strings.Split(qualified, ":")[0]
		for _, link := range linkPattern.FindAllSubmatch(readIssue798File(t, entry), -1) {
			label := string(link[1])
			id := strings.ToLower(label)
			owner := pack
			if label == "Again" {
				id = "left"
			}
			if label == "Foreign" {
				owner = "beta"
				id = "leaf"
			}
			path, err := url.PathUnescape(string(link[2]))
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Clean(filepath.Join(filepath.Dir(entry), filepath.FromSlash(path)))
			if target != entries[owner+":"+id] {
				t.Fatalf("unusable reference %s %s -> %s", qualified, label, target)
			}
		}
	}
}

func assertAccessFailure(t *testing.T, out, diagnostic []byte, err error, code, message string) {
	t.Helper()
	validateCatalogCLISchema(t, "skill-access", out)
	var report struct {
		Result string
		Error  struct{ Code, Message string }
	}
	if decodeErr := json.Unmarshal(out, &report); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if err == nil || report.Result != "failed" || report.Error.Code != code || !strings.Contains(report.Error.Message, message) {
		t.Fatalf("wanted %s/%s: %v %s %s", code, message, err, out, diagnostic)
	}
}

// Only verified prepared resources and the directories needed to contain them
// may be added by access. All other HOME paths, including unknown tool or host
// state, remain part of the before/after observation.
func accessProtectedState(t *testing.T, workspace string, roots []string) map[string]string {
	t.Helper()
	state := catalogTreeState(t, roots)
	data := filepath.Join(workspace, "home", ".local", "share", "packy")
	prepared := filepath.Join(data, "skill-access")
	for path := range state {
		if path == prepared || strings.HasPrefix(path, prepared+string(filepath.Separator)) || path == data || path == filepath.Dir(data) || path == filepath.Dir(filepath.Dir(data)) {
			delete(state, path)
		}
	}
	return state
}

func expectedAccessDiscovery(pack, host, version string) string {
	prerequisites := "missing-content-runtime"
	if pack != "beta" {
		prerequisites += "; missing-pack-runtime"
	}
	text := fmt.Sprintf("%s %s — skill discovery on %s\nPrerequisites are known declarations, not verified; unknown requirements may exist. Compatibility describes resource access, not native registration, runtime readiness, or workflow success.\n", pack, version, host)
	// Literal authored relationships keep this expectation independent of graph resolution.
	for _, row := range []struct{ id, dependencies string }{{"leaf", "none"}, {"left", "asset:data -> skill:leaf"}, {"right", "asset:data -> skill:leaf"}, {"root", "asset:data -> skill:left -> skill:right"}} {
		text += fmt.Sprintf("Skill: %s:%s (Pack %s) — %s %s procedure\nskill:%s on %s: surface variant; source=skills/%s-%s; origin=upstream:skill (adapted)\nNative name: %s; supported surfaces: claude, codex\nOn-demand compatible: yes; reason: \nNative dependencies (Pack %s): %s; notices: notice:mit\nKnown prerequisites (not verified): %s\nReviewed on-demand protocol: skill-tree-v1; entry document: ACCESS.md\n", pack, row.id, version, host, row.id, row.id, host, row.id, host, row.id, pack, row.dependencies, prerequisites)
		if pack == "alpha" && (row.id == "left" || row.id == "right") {
			text += "On-demand dependency: skill beta:leaf\n"
		}
	}
	return text
}
