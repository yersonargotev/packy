package repositorycandidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestPrepareBuildsAnExactDetachedCandidateOnlyAfterEveryGatePasses(t *testing.T) {
	repositoryRoot, remote, baseSHA := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json": managedManifest("1.0.0", "old guidance\n", nil),
		"bundle/skills/guide/SKILL.md":   "old guidance\n",
		"docs/packs/example.md":          "old pack docs\n",
		"docs/packs/index.md":            "old index\n",
	})
	projectRoot, validation := managedProject(t, "1.1.0", "new guidance\n", nil)
	acquired := acquisition()
	acquired.ProjectRoot = projectRoot
	gates := &fakeGates{generateDocs: func(root string) error {
		writeTestFile(t, filepath.Join(root, "docs", "packs", "example.md"), "new pack docs\n", 0o644)
		writeTestFile(t, filepath.Join(root, "docs", "packs", "index.md"), "new index\n", 0o644)
		return nil
	}}
	preparer := newWithGates(gates)

	prepared, err := preparer.Prepare(context.Background(), repositoryRoot, acquired, validation)
	if err != nil {
		t.Fatalf("Prepare returned an error: %v", err)
	}
	if prepared.Candidate == nil || prepared.Cleanup == nil || prepared.NoChangeReason != "" {
		t.Fatalf("Prepare = %#v, want one candidate and cleanup", prepared)
	}
	candidate := prepared.Candidate
	if candidate.Coordinate.String() != "example@1.1.0" || candidate.Project != "owner/example" {
		t.Fatalf("candidate identity = %#v", candidate)
	}
	if candidate.BaseSHA != baseSHA || candidate.HeadSHA == baseSHA || len(candidate.HeadSHA) != 40 || len(candidate.ResultTreeSHA) != 40 || len(candidate.ID) != 64 {
		t.Fatalf("candidate seals = %#v", candidate)
	}
	for _, evidence := range []string{"repository `101`", "release `202`", "pack-v1.1.0", validation.ManifestSHA256, validation.ClosureSHA256, "Compatibility floor", "complete Packy suite passed"} {
		if !strings.Contains(candidate.Summary, evidence) {
			t.Fatalf("candidate summary lacks %q:\n%s", evidence, candidate.Summary)
		}
	}
	if candidate.Branch != "promote/example-1.1.0" {
		t.Fatalf("candidate branch = %q", candidate.Branch)
	}
	if gates.calls != 3 || !gates.observedDetachedBase {
		t.Fatalf("gate calls=%d detached-base=%v", gates.calls, gates.observedDetachedBase)
	}
	if got := gitOutput(t, candidate.RepositoryRoot, "symbolic-ref", "-q", "HEAD"); got != "" {
		t.Fatalf("candidate HEAD is attached to %q", got)
	}
	if got := gitOutput(t, candidate.RepositoryRoot, "remote", "get-url", "origin"); got != remote {
		t.Fatalf("candidate origin = %q, want %q", got, remote)
	}
	if got := readTestFile(t, filepath.Join(candidate.RepositoryRoot, "bundle", "skills", "guide", "SKILL.md")); got != "new guidance\n" {
		t.Fatalf("materialized guidance = %q", got)
	}
	if info, err := os.Stat(filepath.Join(candidate.RepositoryRoot, "bundle", "skills", "guide", "SKILL.md")); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("materialized guidance mode = %v, %v", info, err)
	}
	if got := readTestFile(t, filepath.Join(candidate.RepositoryRoot, "bundle", "packs", "example", "pack.json")); got != managedManifest("1.1.0", "new guidance\n", nil) {
		t.Fatalf("materialized manifest differs:\n%s", got)
	}
	record, err := managedpack.LoadAdmissionRecord(filepath.Join(candidate.RepositoryRoot, "managed-packs", "admissions", "example", "1.1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantRecord := admissionRecord(validation)
	if !reflect.DeepEqual(record, wantRecord) {
		t.Fatalf("admission record = %#v\nwant %#v", record, wantRecord)
	}
	if got := gitOutput(t, candidate.RepositoryRoot, "status", "--short"); got != "" {
		t.Fatalf("candidate worktree is dirty: %s", got)
	}
	if got := gitOutput(t, repositoryRoot, "rev-parse", "HEAD"); got != baseSHA {
		t.Fatalf("source HEAD changed to %s", got)
	}
	if got := gitOutput(t, repositoryRoot, "status", "--short"); got != "" {
		t.Fatalf("source worktree changed: %s", got)
	}
	if got := gitOutput(t, repositoryRoot, "branch", "--list", "promote/*"); got != "" {
		t.Fatalf("source gained a proposal branch: %s", got)
	}
	candidateRoot := candidate.RepositoryRoot
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate root still exists after cleanup: %v", err)
	}
}

func TestPrepareReturnsNoChangeOnlyForTheExactAdmittedCoordinateAndBytes(t *testing.T) {
	projectRoot, validation := managedProject(t, "1.1.0", "admitted guidance\n", nil)
	recordData, err := managedpack.MarshalAdmissionRecord(admissionRecord(validation))
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json":              managedManifest("1.1.0", "admitted guidance\n", nil),
		"bundle/skills/guide/SKILL.md":                "admitted guidance\n",
		"managed-packs/admissions/example/1.1.0.json": string(recordData),
		"docs/packs/example.md":                       "current docs\n",
		"docs/packs/index.md":                         "current index\n",
	})
	acquired := acquisitionFor("1.1.0", projectRoot)
	gates := &fakeGates{}

	prepared, err := newWithGates(gates).Prepare(context.Background(), repositoryRoot, acquired, validation)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Candidate != nil || prepared.NoChangeReason == "" || prepared.Cleanup == nil {
		t.Fatalf("Prepare = %#v, want exact no-change", prepared)
	}
	if gates.calls != 0 {
		t.Fatalf("gates ran %d times for an exact admitted generation", gates.calls)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareDoesNotReturnNoChangeWhenTheTargetStillOwnsResidualBytes(t *testing.T) {
	projectRoot, validation := managedProject(t, "1.1.0", "admitted guidance\n", nil)
	recordData, err := managedpack.MarshalAdmissionRecord(admissionRecord(validation))
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json":              managedManifest("1.1.0", "admitted guidance\n", nil),
		"bundle/skills/guide/SKILL.md":                "admitted guidance\n",
		"bundle/skills/guide/residual.md":             "obsolete\n",
		"managed-packs/admissions/example/1.1.0.json": string(recordData),
		"docs/packs/example.md":                       "current docs\n",
		"docs/packs/index.md":                         "current index\n",
	})

	_, err = newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquisitionFor("1.1.0", projectRoot), validation)
	if got := rejectionGate(t, err); got != managedpackpromotion.GateSemVer {
		t.Fatalf("residual coordinate gate = %s, want semver instead of no-change", got)
	}
}

func TestPrepareEnforcesMonotonicSemVerAndMechanicalCompatibilityFloor(t *testing.T) {
	tests := []struct {
		name             string
		currentVersion   string
		candidateVersion string
		currentMutate    func(map[string]any)
		candidateMutate  func(map[string]any)
		wantGate         managedpackpromotion.Gate
	}{
		{name: "nonmonotonic", currentVersion: "1.0.0", candidateVersion: "1.0.0", wantGate: managedpackpromotion.GateSemVer},
		{name: "resource removal needs major", currentVersion: "1.0.0", candidateVersion: "1.1.0", currentMutate: func(manifest map[string]any) { addResource(manifest, "other", "skills/other") }, wantGate: managedpackpromotion.GateCompatibilityFloor},
		{name: "resource addition needs minor", currentVersion: "1.0.0", candidateVersion: "1.0.1", candidateMutate: func(manifest map[string]any) { addResource(manifest, "other", "skills/other") }, wantGate: managedpackpromotion.GateCompatibilityFloor},
		{name: "resource contract break needs major", currentVersion: "1.0.0", candidateVersion: "1.1.0", candidateMutate: func(manifest map[string]any) {
			binding := resourceDocument(manifest)["bindings"].([]any)[0].(map[string]any)
			binding["sharing"] = "shared"
		}, wantGate: managedpackpromotion.GateCompatibilityFloor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"bundle/packs/example/pack.json": managedManifest(test.currentVersion, "old\n", test.currentMutate),
				"bundle/skills/guide/SKILL.md":   "old\n",
				"docs/packs/example.md":          "old docs\n",
				"docs/packs/index.md":            "old index\n",
			}
			if test.currentMutate != nil {
				files["bundle/skills/other/SKILL.md"] = "old\n"
			}
			repositoryRoot, _, _ := packyRepository(t, files)
			projectRoot, validation := managedProject(t, test.candidateVersion, "new\n", test.candidateMutate)
			_, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquisitionFor(test.candidateVersion, projectRoot), validation)
			if got := rejectionGate(t, err); got != test.wantGate {
				t.Fatalf("rejection gate = %s, want %s", got, test.wantGate)
			}
		})
	}
}

func TestPrepareAllowsAHigherIncrementToSatisfyTheMechanicalFloor(t *testing.T) {
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json": managedManifest("1.0.0", "old\n", nil),
		"bundle/skills/guide/SKILL.md":   "old\n",
		"docs/packs/example.md":          "old docs\n",
		"docs/packs/index.md":            "old index\n",
	})
	projectRoot, validation := managedProject(t, "2.0.0", "new\n", func(manifest map[string]any) { addResource(manifest, "other", "skills/other") })
	prepared, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquisitionFor("2.0.0", projectRoot), validation)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Candidate == nil {
		t.Fatal("Prepare returned no candidate")
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareSealsTheSameInputsIntoTheSameDetachedCommitAndCandidateID(t *testing.T) {
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json": managedManifest("1.0.0", "old\n", nil),
		"bundle/skills/guide/SKILL.md":   "old\n",
		"docs/packs/example.md":          "old docs\n",
		"docs/packs/index.md":            "old index\n",
	})
	projectRoot, validation := managedProject(t, "1.0.1", "new\n", nil)
	acquired := acquisitionFor("1.0.1", projectRoot)
	first, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquired, validation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquired, validation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Candidate.ID != second.Candidate.ID || first.Candidate.HeadSHA != second.Candidate.HeadSHA || first.Candidate.ResultTreeSHA != second.Candidate.ResultTreeSHA || first.Candidate.Summary != second.Candidate.Summary {
		t.Fatalf("candidate sealing is not deterministic:\nfirst=%#v\nsecond=%#v", first.Candidate, second.Candidate)
	}
	if err := first.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := second.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRejectsOverlappingOrDriftingCrossPackRootsAndAllowsAnExactSharedRoot(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		content    string
		wantReject bool
	}{
		{name: "nested root", root: "skills/shared/nested", content: "nested\n", wantReject: true},
		{name: "shared root drift", root: "skills/shared", content: "drifted\n", wantReject: true},
		{name: "exact shared root", root: "skills/shared", content: "shared\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			otherManifest := managedManifest("1.0.0", "shared\n", func(manifest map[string]any) { resourceDocument(manifest)["source"] = "skills/shared" })
			otherManifest = strings.Replace(otherManifest, `"id": "example"`, `"id": "other"`, 1)
			repositoryRoot, _, _ := packyRepository(t, map[string]string{
				"bundle/packs/example/pack.json": managedManifest("1.0.0", "old\n", nil),
				"bundle/packs/other/pack.json":   otherManifest,
				"bundle/skills/guide/SKILL.md":   "old\n",
				"bundle/skills/shared/SKILL.md":  "shared\n",
				"docs/packs/example.md":          "old docs\n",
				"docs/packs/index.md":            "old index\n",
			})
			projectRoot, validation := managedProject(t, "2.0.0", test.content, func(manifest map[string]any) { resourceDocument(manifest)["source"] = test.root })
			prepared, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquisitionFor("2.0.0", projectRoot), validation)
			if test.wantReject {
				if got := rejectionGate(t, err); got != managedpackpromotion.GateOwnership {
					t.Fatalf("rejection gate = %s, want ownership", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := readTestFile(t, filepath.Join(prepared.Candidate.RepositoryRoot, "bundle", "skills", "shared", "SKILL.md")); got != "shared\n" {
				t.Fatalf("shared contribution = %q", got)
			}
			if err := prepared.Cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPrepareRetiresOnlyUnsharedTargetPathsAndPreservesAnotherPacksContribution(t *testing.T) {
	sharedCurrent := managedManifest("1.0.0", "shared\n", func(manifest map[string]any) { resourceDocument(manifest)["source"] = "skills/shared" })
	otherManifest := strings.Replace(sharedCurrent, `"id": "example"`, `"id": "other"`, 1)
	repositoryRoot, _, _ := packyRepository(t, map[string]string{
		"bundle/packs/example/pack.json": sharedCurrent,
		"bundle/packs/other/pack.json":   otherManifest,
		"bundle/skills/shared/SKILL.md":  "shared\n",
		"docs/packs/example.md":          "old docs\n",
		"docs/packs/index.md":            "old index\n",
	})
	projectRoot, validation := managedProject(t, "2.0.0", "replacement\n", func(manifest map[string]any) { resourceDocument(manifest)["source"] = "skills/replacement" })

	prepared, err := newWithGates(&fakeGates{}).Prepare(context.Background(), repositoryRoot, acquisitionFor("2.0.0", projectRoot), validation)
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(prepared.Candidate.RepositoryRoot, "bundle", "skills", "shared", "SKILL.md")); got != "shared\n" {
		t.Fatalf("other Pack contribution = %q", got)
	}
	if got := readTestFile(t, filepath.Join(prepared.Candidate.RepositoryRoot, "bundle", "skills", "replacement", "SKILL.md")); got != "replacement\n" {
		t.Fatalf("replacement contribution = %q", got)
	}
	if err := prepared.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareGateFailuresAndAllowlistViolationsLeaveTheSourceWithoutProposalState(t *testing.T) {
	tests := []struct {
		name     string
		gates    *fakeGates
		wantGate managedpackpromotion.Gate
	}{
		{name: "generated docs", gates: &fakeGates{generateDocs: func(string) error { return errors.New("docs failed") }}, wantGate: managedpackpromotion.GateGeneratedDocs},
		{name: "resource fitness", gates: &fakeGates{resourceError: errors.New("catalog failed")}, wantGate: managedpackpromotion.GateResourceSurfaces},
		{name: "complete suite", gates: &fakeGates{suiteError: errors.New("suite failed")}, wantGate: managedpackpromotion.GatePackySuite},
		{name: "allowlist", gates: &fakeGates{generateDocs: func(root string) error {
			if err := os.WriteFile(filepath.Join(root, "unexpected.txt"), []byte("drift\n"), 0o644); err != nil {
				return err
			}
			return nil
		}}, wantGate: managedpackpromotion.GateValidation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repositoryRoot, _, baseSHA := packyRepository(t, map[string]string{
				"bundle/packs/example/pack.json": managedManifest("1.0.0", "old\n", nil),
				"bundle/skills/guide/SKILL.md":   "old\n",
				"docs/packs/example.md":          "old docs\n",
				"docs/packs/index.md":            "old index\n",
			})
			projectRoot, validation := managedProject(t, "1.0.1", "new\n", nil)
			_, err := newWithGates(test.gates).Prepare(context.Background(), repositoryRoot, acquisitionFor("1.0.1", projectRoot), validation)
			if got := rejectionGate(t, err); got != test.wantGate {
				t.Fatalf("rejection gate = %s, want %s", got, test.wantGate)
			}
			if got := gitOutput(t, repositoryRoot, "rev-parse", "HEAD"); got != baseSHA {
				t.Fatalf("source HEAD changed to %s", got)
			}
			if got := gitOutput(t, repositoryRoot, "status", "--short"); got != "" {
				t.Fatalf("source worktree changed: %s", got)
			}
			if got := gitOutput(t, repositoryRoot, "branch", "--list", "promote/*"); got != "" {
				t.Fatalf("source gained proposal branch: %s", got)
			}
		})
	}
}

func TestGateEnvironmentIsAnExplicitCredentialFreeOfflineAllowlist(t *testing.T) {
	t.Setenv("PATH", "/safe/bin")
	t.Setenv("GITHUB_TOKEN", "must-not-cross-the-gate")
	t.Setenv("UNRELATED_AMBIENT_VALUE", "must-not-cross-the-gate")
	cacheRoot := t.TempDir()
	t.Setenv("GOCACHE", filepath.Join(cacheRoot, "build"))
	t.Setenv("GOMODCACHE", filepath.Join(cacheRoot, "modules"))
	t.Setenv("GOPATH", filepath.Join(cacheRoot, "path"))
	environment, err := gateEnvironment(context.Background(), "/sandbox/home", "/sandbox/config", "/sandbox/cache", "/sandbox/tmp")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, forbidden := range []string{"GITHUB_TOKEN", "UNRELATED_AMBIENT_VALUE", "must-not-cross-the-gate"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("gate environment retained %q:\n%s", forbidden, joined)
		}
	}
	for _, required := range []string{
		"HOME=/sandbox/home", "XDG_CONFIG_HOME=/sandbox/config", "XDG_CACHE_HOME=/sandbox/cache", "TMPDIR=/sandbox/tmp",
		"PATH=/safe/bin", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOVCS=*:off", "GIT_CONFIG_GLOBAL=/dev/null",
		"GOCACHE=" + filepath.Join(cacheRoot, "build"), "GOMODCACHE=" + filepath.Join(cacheRoot, "modules"), "GOPATH=" + filepath.Join(cacheRoot, "path"),
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("gate environment lacks %q:\n%s", required, joined)
		}
	}
}

type fakeGates struct {
	calls                int
	observedDetachedBase bool
	generateDocs         func(string) error
	resourceError        error
	suiteError           error
}

func (g *fakeGates) GenerateDocs(_ context.Context, root string) error {
	g.observe(root)
	if g.generateDocs != nil {
		return g.generateDocs(root)
	}
	return nil
}

func (g *fakeGates) ValidateResources(_ context.Context, root string) error {
	g.observe(root)
	return g.resourceError
}

func (g *fakeGates) ValidateSuite(_ context.Context, root string) error {
	g.observe(root)
	return g.suiteError
}

func (g *fakeGates) observe(root string) {
	g.calls++
	head := gitOutputNoTest(root, "rev-parse", "HEAD")
	subject := gitOutputNoTest(root, "log", "-1", "--format=%s")
	if len(head) == 40 && subject == "fixture" && gitOutputNoTest(root, "symbolic-ref", "-q", "HEAD") == "" {
		g.observedDetachedBase = true
	}
}

func acquisition() managedpackpromotion.Acquisition {
	return managedpackpromotion.Acquisition{
		Release: managedpackpromotion.Release{
			Project: "owner/example", RepositoryID: 101, ReleaseID: 202,
			Immutable: true, Tag: "pack-v1.1.0",
			TagRef:     managedpackpromotion.GitObject{SHA: strings.Repeat("a", 40), Type: managedpackpromotion.GitObjectTag},
			TagObjects: []managedpackpromotion.TagObject{{SHA: strings.Repeat("a", 40), TargetSHA: strings.Repeat("b", 40), TargetType: managedpackpromotion.GitObjectCommit}},
			CommitSHA:  strings.Repeat("b", 40), RootTreeSHA: strings.Repeat("c", 40),
		},
		ProjectRoot: "replaced by test",
	}
}

func acquisitionFor(version, projectRoot string) managedpackpromotion.Acquisition {
	value := acquisition()
	value.ProjectRoot = projectRoot
	value.Release.Tag = "pack-v" + version
	return value
}

func admissionRecord(validation managedpack.Validation) managedpack.AdmissionRecord {
	return managedpack.AdmissionRecord{
		SchemaVersion: 1, PackID: "example", PackVersion: validation.Manifest.Version,
		Project: "owner/example", RepositoryID: 101, ReleaseID: 202, ReleaseImmutable: true,
		Tag: "pack-v" + validation.Manifest.Version, TagRefType: "tag", TagRefSHA: strings.Repeat("a", 40),
		TagObjects: []managedpack.TagObject{{SHA: strings.Repeat("a", 40), TargetSHA: strings.Repeat("b", 40), TargetType: "commit"}},
		Commit:     strings.Repeat("b", 40), RootTree: strings.Repeat("c", 40),
		ManifestSHA256: validation.ManifestSHA256, ClosureSHA256: validation.ClosureSHA256,
		Files: append([]managedpack.FileRecord(nil), validation.Files...),
	}
}

func managedProject(t *testing.T, version, content string, mutate func(map[string]any)) (string, managedpack.Validation) {
	t.Helper()
	root := t.TempDir()
	manifest := managedManifest(version, content, mutate)
	var document struct {
		Resources []struct {
			Source string `json:"source"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(manifest), &document); err != nil {
		t.Fatal(err)
	}
	for _, resource := range document.Resources {
		if resource.Source == "" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(resource.Source))
		if filepath.Ext(path) == "" {
			path = filepath.Join(path, "SKILL.md")
		}
		writeTestFile(t, path, content, 0o644)
	}
	writeTestFile(t, filepath.Join(root, "pack.json"), manifest, 0o644)
	validation, err := managedpack.ValidateProject(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("validate fixture: %v\n%s", err, manifest)
	}
	return root, validation
}

func managedManifest(version, _ string, mutate func(map[string]any)) string {
	document := map[string]any{
		"schema_version": 1, "id": "example", "version": version,
		"description": "Example Pack", "selectable": true,
		"surfaces": []any{"codex"}, "readiness_obligations": []any{}, "external_requirements": []any{}, "origins": []any{},
		"resources": []any{map[string]any{
			"kind": "skill", "id": "guide", "source": "skills/guide", "description": "Example guidance",
			"requires": []any{}, "conflicts": []any{},
			"bindings":           []any{map[string]any{"surface": "codex", "projection": "skill", "name": "guide", "invocation": "guide", "mode": "native", "sharing": "exclusive", "capabilities": []any{}}},
			"surface_exclusions": []any{},
		}},
	}
	if mutate != nil {
		mutate(document)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(append(data, '\n'))
}

func packyRepository(t *testing.T, files map[string]string) (root, remote, baseSHA string) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), "packy.git")
	runTestCommand(t, "", "git", "init", "--bare", remote)
	root = filepath.Join(t.TempDir(), "packy")
	runTestCommand(t, "", "git", "init", "-b", "main", root)
	for path, content := range files {
		writeTestFile(t, filepath.Join(root, path), content, 0o644)
	}
	runTestCommand(t, root, "git", "add", ".")
	runTestCommandEnv(t, root, []string{"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.test", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.test"}, "git", "commit", "-m", "fixture")
	runTestCommand(t, root, "git", "remote", "add", "origin", remote)
	runTestCommand(t, root, "git", "push", "-u", "origin", "main")
	return root, remote, gitOutput(t, root, "rev-parse", "HEAD")
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = testGitEnvironment(t.TempDir(), nil)
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

func gitOutputNoTest(root string, args ...string) string {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = testGitEnvironment(filepath.Join(os.TempDir(), "packy-test-no-home"), nil)
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

func runTestCommand(t *testing.T, root, name string, args ...string) {
	t.Helper()
	runTestCommandEnv(t, root, nil, name, args...)
}

func runTestCommandEnv(t *testing.T, root string, environment []string, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = root
	command.Env = testGitEnvironment(t.TempDir(), environment)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
}

func testGitEnvironment(home string, additions []string) []string {
	values := map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
		"HOME":                home,
		"LANG":                "C",
		"LC_ALL":              "C",
		"PATH":                os.Getenv("PATH"),
		"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
	}
	for _, entry := range additions {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		values[name] = value
	}
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, name := range keys {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

func rejectionGate(t *testing.T, err error) managedpackpromotion.Gate {
	t.Helper()
	var rejection *managedpackpromotion.RejectionError
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want deterministic rejection", err)
	}
	return rejection.Gate
}

func resourceDocument(manifest map[string]any) map[string]any {
	return manifest["resources"].([]any)[0].(map[string]any)
}

func addResource(manifest map[string]any, id, source string) {
	resource := map[string]any{
		"kind": "skill", "id": id, "source": source, "description": fmt.Sprintf("%s guidance", id),
		"requires": []any{}, "conflicts": []any{},
		"bindings":           []any{map[string]any{"surface": "codex", "projection": "skill", "name": id, "invocation": id, "mode": "native", "sharing": "exclusive", "capabilities": []any{}}},
		"surface_exclusions": []any{},
	}
	manifest["resources"] = append(manifest["resources"].([]any), resource)
}
