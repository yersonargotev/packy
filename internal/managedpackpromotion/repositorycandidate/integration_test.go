package repositorycandidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/githubacquisition"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/offlinevalidation"
	"github.com/yersonargotev/packy/internal/packsync"
)

func TestPromotionModuleComposesRepresentativeManagedPackFixtures(t *testing.T) {
	sourceRoot := composedSourceRoot(t)
	executable := composedPromotePackExecutable(t, sourceRoot)

	for _, fixtureName := range []string{"small", "matty", "pstack"} {
		t.Run(fixtureName, func(t *testing.T) {
			fixture := newComposedFixture(t, sourceRoot, fixtureName)
			repositoryRoot, _, baseSHA := composedPackyRepository(t, fixture.baseTree)
			gates := &composedGates{
				t: t, sourceRepository: repositoryRoot, sourceBaseSHA: baseSHA,
				coordinate: fixture.coordinate, mutationPath: fixture.mutationPath,
			}
			publisher := &composedPublisher{t: t, fixture: fixture, gates: gates}
			source := newComposedSource(t, fixture)
			module := managedpackpromotion.NewModule(
				githubacquisition.New(source),
				offlinevalidation.New(executable),
				newWithGates(gates),
				publisher,
			)

			result, err := module.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: repositoryRoot,
				Coordinate:     fixture.coordinate,
			})
			if err != nil {
				t.Fatalf("Promote() error = %v", err)
			}
			if result.Status != managedpackpromotion.StatusProposal || result.Proposal == nil {
				t.Fatalf("Promote() result = %#v, rejection = %#v, want proposal", result, result.Rejection)
			}
			if result.Proposal.HeadSHA != publisher.headSHA || result.Proposal.Branch != "promote/"+fixture.coordinate.PackID+"-"+fixture.coordinate.Version {
				t.Fatalf("proposal = %#v, captured head = %q", result.Proposal, publisher.headSHA)
			}
			if gates.calls != 3 || publisher.calls != 1 {
				t.Fatalf("gate calls = %d, publisher calls = %d", gates.calls, publisher.calls)
			}
			if _, err := os.Stat(publisher.candidateRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("candidate root survived module cleanup: %v", err)
			}
			assertComposedSourceUnchanged(t, fixture)
			assertComposedRepositoryUnchanged(t, repositoryRoot, baseSHA)
			source.assertCleaned(t)

			admittedTree := publisher.resultTree
			if fixtureName == "small" {
				fixture = newComposedUpdateFixture(t, fixture, admittedTree)
				updateRoot, _, updateBaseSHA := composedPackyRepository(t, fixture.baseTree)
				updateGates := &composedGates{
					t: t, sourceRepository: updateRoot, sourceBaseSHA: updateBaseSHA,
					coordinate: fixture.coordinate, mutationPath: fixture.mutationPath,
				}
				updatePublisher := &composedPublisher{t: t, fixture: fixture, gates: updateGates}
				updateSource := newComposedSource(t, fixture)
				updateModule := managedpackpromotion.NewModule(
					githubacquisition.New(updateSource),
					offlinevalidation.New(executable),
					newWithGates(updateGates),
					updatePublisher,
				)
				updated, err := updateModule.Promote(context.Background(), managedpackpromotion.Request{
					RepositoryRoot: updateRoot,
					Coordinate:     fixture.coordinate,
				})
				if err != nil {
					t.Fatalf("higher-version Promote() error = %v", err)
				}
				if updated.Status != managedpackpromotion.StatusProposal || updated.Proposal == nil {
					t.Fatalf("higher-version Promote() result = %#v, rejection = %#v", updated, updated.Rejection)
				}
				if updateGates.calls != 3 || updatePublisher.calls != 1 {
					t.Fatalf("higher-version gate calls = %d, publisher calls = %d", updateGates.calls, updatePublisher.calls)
				}
				if _, err := os.Stat(updatePublisher.candidateRoot); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("higher-version candidate root survived module cleanup: %v", err)
				}
				assertComposedSourceUnchanged(t, fixture)
				assertComposedRepositoryUnchanged(t, updateRoot, updateBaseSHA)
				updateSource.assertCleaned(t)
				admittedTree = updatePublisher.resultTree
			}

			admittedRoot, _, admittedSHA := composedPackyRepository(t, admittedTree)
			repeatGates := &composedGates{
				t: t, sourceRepository: admittedRoot, sourceBaseSHA: admittedSHA,
				coordinate: fixture.coordinate, mutationPath: fixture.mutationPath,
			}
			repeatPublisher := &composedPublisher{t: t, fixture: fixture, gates: repeatGates}
			repeatSource := newComposedSource(t, fixture)
			repeatModule := managedpackpromotion.NewModule(
				githubacquisition.New(repeatSource),
				offlinevalidation.New(executable),
				newWithGates(repeatGates),
				repeatPublisher,
			)

			repeated, err := repeatModule.Promote(context.Background(), managedpackpromotion.Request{
				RepositoryRoot: admittedRoot,
				Coordinate:     fixture.coordinate,
			})
			if err != nil {
				t.Fatalf("repeat Promote() error = %v", err)
			}
			if repeated.Status != managedpackpromotion.StatusNoChange || repeated.Reason == "" {
				t.Fatalf("repeat Promote() result = %#v, want exact no-change", repeated)
			}
			if repeatGates.calls != 0 || repeatPublisher.calls != 0 {
				t.Fatalf("exact admission reran %d gates and %d publisher calls", repeatGates.calls, repeatPublisher.calls)
			}
			assertComposedRepositoryUnchanged(t, admittedRoot, admittedSHA)
			repeatSource.assertCleaned(t)
		})
	}
}

type composedFixture struct {
	id                string
	project           string
	coordinate        managedpackpromotion.Coordinate
	projectRoot       string
	projectManifest   []byte
	projectTree       composedTree
	baseTree          composedTree
	mutationPath      string
	release           packsync.Release
	releaseCandidate  packsync.Candidate
	wantValidation    managedpack.Validation
	projectTreeBefore composedTree
}

func newComposedFixture(t *testing.T, sourceRoot, name string) composedFixture {
	t.Helper()
	currentRoot := t.TempDir()
	var legacyManifest []byte
	switch name {
	case "small":
		smallManifest := strings.Replace(managedManifest("1.0.0", "", nil), `"id": "example"`, `"id": "small"`, 1)
		writeTestFile(t, filepath.Join(currentRoot, "pack.json"), smallManifest, 0o644)
		writeTestFile(t, filepath.Join(currentRoot, "skills", "guide", "SKILL.md"), "small fixture guidance\n", 0o644)
		legacyManifest = composedLegacyManifest(t, []byte(smallManifest))
	case "matty", "pstack":
		manifestPath := filepath.Join(sourceRoot, "bundle", "packs", name, "pack.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		managedManifestData := composedManagedManifest(t, manifestData)
		writeTestFile(t, filepath.Join(currentRoot, "pack.json"), string(managedManifestData), 0o644)
		for _, source := range composedManifestSources(t, managedManifestData) {
			composedCopyPath(t, filepath.Join(sourceRoot, "bundle", filepath.FromSlash(source)), filepath.Join(currentRoot, filepath.FromSlash(source)))
		}
		legacyManifest = append([]byte(nil), manifestData...)
	default:
		t.Fatalf("unknown composed fixture %q", name)
	}

	currentValidation, err := managedpack.ValidateProject(context.Background(), currentRoot, nil)
	if err != nil {
		t.Fatalf("validate current %s fixture: %v", name, err)
	}
	currentVersion, err := semver.StrictNewVersion(currentValidation.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	candidateVersion := currentVersion.IncPatch().String()
	candidateRoot := t.TempDir()
	composedWriteTree(t, candidateRoot, composedReadTree(t, currentRoot))
	candidateManifest := composedSetManifestVersion(t, readTestFile(t, filepath.Join(candidateRoot, "pack.json")), candidateVersion)
	writeTestFile(t, filepath.Join(candidateRoot, "pack.json"), string(candidateManifest), 0o644)

	mutationPath := ""
	for _, file := range currentValidation.Files {
		if file.Path != "pack.json" {
			mutationPath = file.Path
			break
		}
	}
	if mutationPath == "" {
		t.Fatalf("%s fixture has no closure file to update", name)
	}
	mutationFile := filepath.Join(candidateRoot, filepath.FromSlash(mutationPath))
	info, err := os.Stat(mutationFile)
	if err != nil {
		t.Fatal(err)
	}
	updated := readTestFile(t, mutationFile) + "\nManaged Pack Promotion composed fixture update.\n"
	writeTestFile(t, mutationFile, updated, info.Mode().Perm())

	wantValidation, err := managedpack.ValidateProject(context.Background(), candidateRoot, nil)
	if err != nil {
		t.Fatalf("validate candidate %s fixture: %v", name, err)
	}
	projectManifest := []byte(readTestFile(t, filepath.Join(candidateRoot, "pack.json")))
	projectTree := composedReadTree(t, candidateRoot)
	commitSHA, treeSHA := composedGitIdentity(t, projectTree)
	project := "fixture/" + name
	tag := "pack-v" + candidateVersion
	published := time.Unix(1_700_000_000, 0).UTC()
	release := packsync.Release{
		ID: int64(1000 + len(name)), Tag: tag, Name: tag, Target: commitSHA,
		Immutable: true, CreatedAt: published, PublishedAt: published,
	}
	releaseCandidate := packsync.Candidate{
		Repository: project, RepositoryID: int64(2000 + len(name)), Public: true,
		Release: &release, TagRefName: "refs/tags/" + tag, TagRefType: "commit",
		TagRefSHA: commitSHA, Commit: commitSHA, Tree: treeSHA,
	}
	baseTree := composedTreeFromValidation(t, currentRoot, currentValidation)
	baseTree["bundle/packs/"+name+"/pack.json"] = composedFile{data: legacyManifest, mode: 0o644}
	baseTree["managed-packs/registry.json"] = composedFile{
		data: []byte(fmt.Sprintf("{\n  \"schema_version\": 1,\n  \"packs\": [\n    {\n      \"pack_id\": %q,\n      \"project\": %q\n    }\n  ]\n}\n", name, project)),
		mode: 0o644,
	}
	baseTree["docs/packs/"+name+".md"] = composedFile{data: []byte("old fixture docs\n"), mode: 0o644}
	baseTree["docs/packs/index.md"] = composedFile{data: []byte("old fixture index\n"), mode: 0o644}

	return composedFixture{
		id: name, project: project,
		coordinate:  managedpackpromotion.Coordinate{PackID: name, Version: candidateVersion},
		projectRoot: candidateRoot, projectManifest: projectManifest, projectTree: projectTree,
		baseTree: baseTree, mutationPath: mutationPath, release: release,
		releaseCandidate: releaseCandidate, wantValidation: wantValidation,
		projectTreeBefore: composedReadTree(t, candidateRoot),
	}
}

func newComposedUpdateFixture(t *testing.T, previous composedFixture, admittedTree composedTree) composedFixture {
	t.Helper()
	currentVersion, err := semver.StrictNewVersion(previous.coordinate.Version)
	if err != nil {
		t.Fatal(err)
	}
	candidateVersion := currentVersion.IncPatch().String()
	candidateRoot := t.TempDir()
	composedWriteTree(t, candidateRoot, previous.projectTree)
	candidateManifest := composedSetManifestVersion(t, readTestFile(t, filepath.Join(candidateRoot, "pack.json")), candidateVersion)
	writeTestFile(t, filepath.Join(candidateRoot, "pack.json"), string(candidateManifest), 0o644)
	mutationFile := filepath.Join(candidateRoot, filepath.FromSlash(previous.mutationPath))
	info, err := os.Stat(mutationFile)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, mutationFile, readTestFile(t, mutationFile)+"\nSecond Managed Pack Promotion composed fixture update.\n", info.Mode().Perm())
	wantValidation, err := managedpack.ValidateProject(context.Background(), candidateRoot, nil)
	if err != nil {
		t.Fatalf("validate higher-version fixture: %v", err)
	}
	projectTree := composedReadTree(t, candidateRoot)
	commitSHA, treeSHA := composedGitIdentity(t, projectTree)
	tag := "pack-v" + candidateVersion
	published := time.Unix(1_700_000_100, 0).UTC()
	release := packsync.Release{
		ID: previous.release.ID + 1, Tag: tag, Name: tag, Target: commitSHA,
		Immutable: true, CreatedAt: published, PublishedAt: published,
	}
	releaseCandidate := packsync.Candidate{
		Repository: previous.project, RepositoryID: previous.releaseCandidate.RepositoryID, Public: true,
		Release: &release, TagRefName: "refs/tags/" + tag, TagRefType: "commit",
		TagRefSHA: commitSHA, Commit: commitSHA, Tree: treeSHA,
	}
	return composedFixture{
		id: previous.id, project: previous.project,
		coordinate:  managedpackpromotion.Coordinate{PackID: previous.id, Version: candidateVersion},
		projectRoot: candidateRoot, projectManifest: []byte(readTestFile(t, filepath.Join(candidateRoot, "pack.json"))),
		projectTree: projectTree, baseTree: admittedTree, mutationPath: previous.mutationPath,
		release: release, releaseCandidate: releaseCandidate, wantValidation: wantValidation,
		projectTreeBefore: composedReadTree(t, candidateRoot),
	}
}

type composedSource struct {
	t             *testing.T
	fixture       composedFixture
	snapshotRoots []string
}

func newComposedSource(t *testing.T, fixture composedFixture) *composedSource {
	t.Helper()
	return &composedSource{t: t, fixture: fixture}
}

func (source *composedSource) Releases(_ context.Context, config packsync.SourceConfig) ([]packsync.Release, error) {
	if config.Provider != "github" || config.Repository != source.fixture.project {
		source.t.Fatalf("release config = %#v", config)
	}
	return []packsync.Release{source.fixture.release}, nil
}

func (source *composedSource) ResolveRelease(_ context.Context, config packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	if config.Repository != source.fixture.project || !reflect.DeepEqual(release, source.fixture.release) {
		source.t.Fatalf("resolved release = %#v for %#v", release, config)
	}
	return source.fixture.releaseCandidate, nil
}

func (source *composedSource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, commit string) (packsync.Candidate, error) {
	return packsync.Candidate{}, fmt.Errorf("unexpected origin commit resolution for %q", commit)
}

func (source *composedSource) WithGitTreeSnapshot(ctx context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !reflect.DeepEqual(candidate, source.fixture.releaseCandidate) {
		source.t.Fatalf("snapshot candidate = %#v", candidate)
	}
	entries, err := os.ReadDir(temporaryRoot)
	if err != nil || len(entries) != 0 {
		source.t.Fatalf("snapshot root is not caller-owned and empty: entries=%v error=%v", entries, err)
	}
	source.snapshotRoots = append(source.snapshotRoots, temporaryRoot)
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	composedWriteTree(source.t, snapshot, source.fixture.projectTree)
	visitErr := visit(snapshot)
	removeErr := os.RemoveAll(snapshot)
	return errors.Join(visitErr, removeErr)
}

func (source *composedSource) assertCleaned(t *testing.T) {
	t.Helper()
	if len(source.snapshotRoots) != 1 {
		t.Fatalf("snapshot calls = %d, want 1", len(source.snapshotRoots))
	}
	for _, root := range source.snapshotRoots {
		if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("acquisition staging root survived cleanup: %v", err)
		}
	}
}

type composedGates struct {
	t                *testing.T
	sourceRepository string
	sourceBaseSHA    string
	coordinate       managedpackpromotion.Coordinate
	mutationPath     string
	calls            int
	candidateRoot    string
}

func (gates *composedGates) GenerateDocs(_ context.Context, candidateRoot string) error {
	gates.assertState(candidateRoot, 0, gates.preDocsPaths())
	writeTestFile(gates.t, filepath.Join(candidateRoot, "docs", "packs", gates.coordinate.PackID+".md"), "generated fixture pack docs for "+gates.coordinate.String()+"\n", 0o644)
	writeTestFile(gates.t, filepath.Join(candidateRoot, "docs", "packs", "index.md"), "generated fixture index for "+gates.coordinate.String()+"\n", 0o644)
	gates.calls++
	return nil
}

func (gates *composedGates) ValidateResources(_ context.Context, candidateRoot string) error {
	gates.assertState(candidateRoot, 1, gates.allPaths())
	gates.calls++
	return nil
}

func (gates *composedGates) ValidateSuite(_ context.Context, candidateRoot string) error {
	gates.assertState(candidateRoot, 2, gates.allPaths())
	gates.calls++
	return nil
}

func (gates *composedGates) assertState(candidateRoot string, wantCalls int, wantPaths []string) {
	gates.t.Helper()
	if gates.calls != wantCalls {
		gates.t.Fatalf("gate order = %d, want %d", gates.calls, wantCalls)
	}
	if gates.candidateRoot == "" {
		gates.candidateRoot = candidateRoot
	} else if gates.candidateRoot != candidateRoot {
		gates.t.Fatalf("gates crossed candidate roots: %q and %q", gates.candidateRoot, candidateRoot)
	}
	if candidateRoot == gates.sourceRepository {
		gates.t.Fatal("gate ran in the source repository")
	}
	if got := gitOutput(gates.t, candidateRoot, "rev-parse", "HEAD"); got != gates.sourceBaseSHA {
		gates.t.Fatalf("gate candidate HEAD = %q, want uncommitted base %q", got, gates.sourceBaseSHA)
	}
	if got := gitOutput(gates.t, candidateRoot, "symbolic-ref", "-q", "HEAD"); got != "" {
		gates.t.Fatalf("gate candidate HEAD is attached to %q", got)
	}
	if got := composedChangedPaths(gates.t, candidateRoot); !reflect.DeepEqual(got, wantPaths) {
		gates.t.Fatalf("gate changed paths = %#v, want exact allowlist %#v", got, wantPaths)
	}
	assertComposedRepositoryUnchanged(gates.t, gates.sourceRepository, gates.sourceBaseSHA)
}

func (gates *composedGates) preDocsPaths() []string {
	return composedSortedStrings(
		"bundle/"+gates.mutationPath,
		"bundle/packs/"+gates.coordinate.PackID+"/pack.json",
		"managed-packs/admissions/"+gates.coordinate.PackID+"/"+gates.coordinate.Version+".json",
	)
}

func (gates *composedGates) allPaths() []string {
	return composedSortedStrings(append(gates.preDocsPaths(),
		"docs/packs/"+gates.coordinate.PackID+".md",
		"docs/packs/index.md",
	)...)
}

type composedPublisher struct {
	t             *testing.T
	fixture       composedFixture
	gates         *composedGates
	calls         int
	candidateRoot string
	headSHA       string
	resultTree    composedTree
}

func (publisher *composedPublisher) Publish(_ context.Context, candidate managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
	publisher.calls++
	if publisher.calls != 1 || publisher.gates.calls != 3 {
		publisher.t.Fatalf("publication ran after %d calls with %d completed gates", publisher.calls, publisher.gates.calls)
	}
	if candidate.Coordinate != publisher.fixture.coordinate || candidate.Project != publisher.fixture.project {
		publisher.t.Fatalf("candidate identity = %#v", candidate)
	}
	if candidate.RepositoryRoot == publisher.gates.sourceRepository {
		publisher.t.Fatal("publisher received the source repository")
	}
	if got := gitOutput(publisher.t, candidate.RepositoryRoot, "status", "--short"); got != "" {
		publisher.t.Fatalf("published candidate is dirty: %s", got)
	}
	if got := gitOutput(publisher.t, candidate.RepositoryRoot, "rev-parse", "HEAD^{tree}"); got != candidate.ResultTreeSHA {
		publisher.t.Fatalf("candidate tree = %q, sealed = %q", got, candidate.ResultTreeSHA)
	}
	if got := gitOutput(publisher.t, candidate.RepositoryRoot, "rev-parse", "HEAD"); got != candidate.HeadSHA {
		publisher.t.Fatalf("candidate HEAD = %q, sealed = %q", got, candidate.HeadSHA)
	}
	if got := composedCommitPaths(publisher.t, candidate.RepositoryRoot, candidate.HeadSHA); !reflect.DeepEqual(got, publisher.gates.allPaths()) {
		publisher.t.Fatalf("candidate output paths = %#v, want exact allowlist %#v", got, publisher.gates.allPaths())
	}
	manifestPath := filepath.Join(candidate.RepositoryRoot, "bundle", "packs", publisher.fixture.id, "pack.json")
	if got := []byte(readTestFile(publisher.t, manifestPath)); !reflect.DeepEqual(got, publisher.fixture.projectManifest) {
		publisher.t.Fatalf("materialized manifest differs from acquired manifest")
	}
	for _, record := range publisher.fixture.wantValidation.Files {
		sourcePath := filepath.Join(publisher.fixture.projectRoot, filepath.FromSlash(record.Path))
		destination := record.Path
		if record.Path == "pack.json" {
			destination = filepath.ToSlash(filepath.Join("packs", publisher.fixture.id, "pack.json"))
		}
		destinationPath := filepath.Join(candidate.RepositoryRoot, "bundle", filepath.FromSlash(destination))
		sourceInfo, sourceErr := os.Lstat(sourcePath)
		destinationInfo, destinationErr := os.Lstat(destinationPath)
		if sourceErr != nil || destinationErr != nil || !sourceInfo.Mode().IsRegular() || !destinationInfo.Mode().IsRegular() ||
			sourceInfo.Mode()&os.ModeSymlink != 0 || destinationInfo.Mode()&os.ModeSymlink != 0 ||
			sourceInfo.Mode().Perm() != destinationInfo.Mode().Perm() ||
			readTestFile(publisher.t, sourcePath) != readTestFile(publisher.t, destinationPath) {
			publisher.t.Fatalf("materialized closure path %q differs in mode or bytes: source=%v/%v destination=%v/%v", record.Path, sourceInfo, sourceErr, destinationInfo, destinationErr)
		}
		wantMode := "100644"
		if destinationInfo.Mode().Perm() == 0o755 {
			wantMode = "100755"
		}
		if wantMode != record.Mode {
			publisher.t.Fatalf("materialized closure path %q mode = %s, validation = %s", record.Path, wantMode, record.Mode)
		}
	}
	record, err := managedpack.LoadAdmissionRecord(filepath.Join(candidate.RepositoryRoot, "managed-packs", "admissions", publisher.fixture.id, publisher.fixture.coordinate.Version+".json"))
	if err != nil {
		publisher.t.Fatal(err)
	}
	if record.Project != publisher.fixture.project || record.RepositoryID != publisher.fixture.releaseCandidate.RepositoryID || record.ReleaseID != publisher.fixture.release.ID ||
		record.Commit != publisher.fixture.releaseCandidate.Commit || record.RootTree != publisher.fixture.releaseCandidate.Tree ||
		record.ManifestSHA256 != publisher.fixture.wantValidation.ManifestSHA256 || record.ClosureSHA256 != publisher.fixture.wantValidation.ClosureSHA256 ||
		!reflect.DeepEqual(record.Files, publisher.fixture.wantValidation.Files) {
		publisher.t.Fatalf("admission record does not seal exact fixture evidence: %#v", record)
	}
	publisher.candidateRoot = candidate.RepositoryRoot
	publisher.headSHA = candidate.HeadSHA
	publisher.resultTree = composedReadTree(publisher.t, candidate.RepositoryRoot)
	return managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{
		Branch: candidate.Branch, Number: 698,
		URL: "https://example.test/pulls/698", HeadSHA: candidate.HeadSHA,
	}}, nil
}

type composedFile struct {
	data []byte
	mode os.FileMode
}

type composedTree map[string]composedFile

func composedManagedManifest(t *testing.T, data []byte) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["schema_version"] = 1
	document["origins"] = []any{}
	delete(document, "exclusions")
	delete(document, "source_reference")
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func composedLegacyManifest(t *testing.T, data []byte) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "schema_version")
	delete(document, "origins")
	document["exclusions"] = []any{}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func composedSetManifestVersion(t *testing.T, manifest, version string) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(manifest), &document); err != nil {
		t.Fatal(err)
	}
	document["version"] = version
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func composedManifestSources(t *testing.T, manifest []byte) []string {
	t.Helper()
	var document any
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if key == "source" {
					if source, ok := nested.(string); ok && source != "" {
						set[source] = true
					}
				}
				visit(nested)
			}
		case []any:
			for _, nested := range typed {
				visit(nested)
			}
		}
	}
	visit(document)
	result := make([]string, 0, len(set))
	for source := range set {
		result = append(result, source)
	}
	sort.Strings(result)
	return result
}

func composedTreeFromValidation(t *testing.T, root string, validation managedpack.Validation) composedTree {
	t.Helper()
	result := composedTree{}
	for _, record := range validation.Files {
		sourcePath := filepath.Join(root, filepath.FromSlash(record.Path))
		destination := "bundle/" + record.Path
		if record.Path == "pack.json" {
			destination = filepath.ToSlash(filepath.Join("bundle", "packs", validation.Manifest.ID, "pack.json"))
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		result[destination] = composedFile{data: []byte(readTestFile(t, sourcePath)), mode: info.Mode().Perm()}
	}
	return result
}

func composedReadTree(t *testing.T, root string) composedTree {
	t.Helper()
	result := composedTree{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".git" || strings.HasPrefix(filepath.ToSlash(relative), ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture contains non-regular path %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = composedFile{data: data, mode: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func composedWriteTree(t *testing.T, root string, tree composedTree) {
	t.Helper()
	for path, file := range tree {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), string(file.data), file.mode)
	}
}

func composedCopyPath(t *testing.T, source, destination string) {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().IsRegular() {
		writeTestFile(t, destination, readTestFile(t, source), info.Mode().Perm())
		return
	}
	if !info.IsDir() {
		t.Fatalf("fixture path %q is not regular content", source)
	}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("fixture path %q is not regular content", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		writeTestFile(t, filepath.Join(destination, relative), readTestFile(t, path), entryInfo.Mode().Perm())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func composedPackyRepository(t *testing.T, tree composedTree) (root, remote, baseSHA string) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), "packy.git")
	runTestCommand(t, "", "git", "init", "--bare", remote)
	root = filepath.Join(t.TempDir(), "packy")
	runTestCommand(t, "", "git", "init", "-b", "main", root)
	composedWriteTree(t, root, tree)
	runTestCommand(t, root, "git", "add", ".")
	runTestCommandEnv(t, root, []string{
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.test",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.test",
	}, "git", "commit", "-m", "fixture")
	runTestCommand(t, root, "git", "remote", "add", "origin", remote)
	runTestCommand(t, root, "git", "push", "-u", "origin", "main")
	return root, remote, gitOutput(t, root, "rev-parse", "HEAD")
}

func composedGitIdentity(t *testing.T, tree composedTree) (commitSHA, treeSHA string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	runTestCommand(t, "", "git", "init", "-b", "main", root)
	composedWriteTree(t, root, tree)
	runTestCommand(t, root, "git", "add", ".")
	runTestCommandEnv(t, root, []string{
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.test",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.test",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	}, "git", "commit", "-m", "managed fixture")
	return gitOutput(t, root, "rev-parse", "HEAD"), gitOutput(t, root, "rev-parse", "HEAD^{tree}")
}

func composedChangedPaths(t *testing.T, repositoryRoot string) []string {
	t.Helper()
	changed := strings.Fields(gitOutput(t, repositoryRoot, "diff", "--name-only", "HEAD", "--"))
	untracked := strings.Fields(gitOutput(t, repositoryRoot, "ls-files", "--others", "--exclude-standard"))
	return composedSortedStrings(append(changed, untracked...)...)
}

func composedCommitPaths(t *testing.T, repositoryRoot, headSHA string) []string {
	t.Helper()
	return composedSortedStrings(strings.Fields(gitOutput(t, repositoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", headSHA))...)
}

func composedSortedStrings(values ...string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func assertComposedRepositoryUnchanged(t *testing.T, repositoryRoot, baseSHA string) {
	t.Helper()
	if got := gitOutput(t, repositoryRoot, "rev-parse", "HEAD"); got != baseSHA {
		t.Fatalf("source repository HEAD = %q, want %q", got, baseSHA)
	}
	if got := gitOutput(t, repositoryRoot, "status", "--short"); got != "" {
		t.Fatalf("source repository changed: %s", got)
	}
	if got := gitOutput(t, repositoryRoot, "branch", "--list", "promote/*"); got != "" {
		t.Fatalf("source repository gained proposal state: %s", got)
	}
}

func assertComposedSourceUnchanged(t *testing.T, fixture composedFixture) {
	t.Helper()
	if got := composedReadTree(t, fixture.projectRoot); !reflect.DeepEqual(got, fixture.projectTreeBefore) {
		t.Fatalf("Managed Pack Project fixture changed during promotion")
	}
	if got := []byte(readTestFile(t, filepath.Join(fixture.projectRoot, "pack.json"))); !reflect.DeepEqual(got, fixture.projectManifest) {
		t.Fatalf("Managed Pack Project manifest changed during promotion")
	}
}

func composedSourceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository source root: %v", err)
	}
	return root
}

func composedPromotePackExecutable(t *testing.T, sourceRoot string) string {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "promotepack")
	if err := runSanitized(context.Background(), sourceRoot, "go", "build", "-o", executable, "./internal/tools/promotepack"); err != nil {
		t.Fatalf("build private promotepack worker: %v", err)
	}
	return executable
}

var _ githubacquisition.Source = (*composedSource)(nil)
var _ managedpackpromotion.Publisher = (*composedPublisher)(nil)
