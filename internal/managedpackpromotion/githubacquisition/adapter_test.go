package githubacquisition_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/githubacquisition"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsync/githubsource"
)

var _ githubacquisition.Source = (*githubsource.Client)(nil)

func TestAdapterAcquireCopiesOneImmutableReleaseAndItsOrigins(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	projectCommit := strings.Repeat("a", 40)
	originCommit := strings.Repeat("b", 40)
	source := newFakeSource(t)
	source.releases = []packsync.Release{validSourceRelease("pack-v1.2.3")}
	source.releaseCandidates = []packsync.Candidate{
		validReleaseCandidate("Example/Managed", projectCommit),
		validReleaseCandidate("example/managed", projectCommit),
	}
	source.commitCandidates[originCommit] = validCommitCandidate("UPSTREAM/Source", originCommit)
	source.snapshots[projectCommit] = map[string]fakeFile{
		"pack.json": {content: fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"upstream/source","commit":%q}]}`, originCommit), mode: 0o644},
		"tool.sh":   {content: "#!/bin/sh\nexit 0\n", mode: 0o755},
	}
	source.snapshots[originCommit] = map[string]fakeFile{
		"LICENSE": {content: "license\n", mode: 0o644},
	}

	acquisition, err := githubacquisition.New(source).Acquire(
		context.Background(),
		"example/managed",
		managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"},
	)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	t.Cleanup(func() { _ = acquisition.Cleanup() })

	if acquisition.Release.Project != "example/managed" || acquisition.Release.RepositoryID != 101 || acquisition.Release.ReleaseID != 202 {
		t.Fatalf("Release = %#v", acquisition.Release)
	}
	if acquisition.Release.Tag != "pack-v1.2.3" || !acquisition.Release.Immutable || acquisition.Release.CommitSHA != projectCommit || acquisition.Release.RootTreeSHA != strings.Repeat("c", 40) {
		t.Fatalf("Release = %#v", acquisition.Release)
	}
	assertFile(t, filepath.Join(acquisition.ProjectRoot, "tool.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	assertFile(t, filepath.Join(acquisition.OriginRoots["upstream"], "LICENSE"), "license\n", 0o644)
	if len(source.releaseCandidates) != 0 {
		t.Fatalf("ResolveRelease calls left = %d, want 0", len(source.releaseCandidates))
	}

	root := filepath.Dir(acquisition.ProjectRoot)
	if err := acquisition.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("acquisition root still exists after cleanup: %v", err)
	}
	if err := acquisition.Cleanup(); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}
}

func TestAdapterAcquireRejectsUnacceptableReleaseBeforeSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeSource)
	}{
		{"wrong tag", func(source *fakeSource) { source.releases[0].Tag = "v1.2.3" }},
		{"duplicate matching tag", func(source *fakeSource) { source.releases = append(source.releases, source.releases[0]) }},
		{"mutable", func(source *fakeSource) { source.releases[0].Immutable = false }},
		{"unpublished", func(source *fakeSource) { source.releases[0].PublishedAt = time.Time{} }},
		{"draft", func(source *fakeSource) { source.releases[0].Draft = true }},
		{"prerelease", func(source *fakeSource) { source.releases[0].Prerelease = true }},
		{"private project", func(source *fakeSource) { source.releaseCandidates[0].Public = false }},
		{"wrong project identity", func(source *fakeSource) { source.releaseCandidates[0].Repository = "example/other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			temporary := t.TempDir()
			t.Setenv("TMPDIR", temporary)
			source := releaseOnlySource(t)
			test.mutate(source)

			acquisition, err := githubacquisition.New(source).Acquire(context.Background(), "example/managed", testCoordinate())
			assertRejection(t, err, managedpackpromotion.GateRelease)
			assertZeroAcquisition(t, acquisition)
			assertEmptyDirectory(t, temporary)
		})
	}
}

func TestAdapterAcquireRejectsPrereleaseCoordinate(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)

	acquisition, err := githubacquisition.New(releaseOnlySource(t)).Acquire(
		context.Background(), "example/managed",
		managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3-rc.1"},
	)
	assertRejection(t, err, managedpackpromotion.GateRelease)
	assertZeroAcquisition(t, acquisition)
	assertEmptyDirectory(t, temporary)
}

func TestAdapterAcquireRejectsReleaseEvidenceThatMovesDuringAcquisition(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	source := validAcquisitionSource(t, `{"origins":[]}`)
	moved := source.releaseCandidates[1]
	moved.TagRefSHA = strings.Repeat("f", 40)
	moved.TagObjects[0].SHA = moved.TagRefSHA
	source.releaseCandidates[1] = moved

	acquisition, err := githubacquisition.New(source).Acquire(context.Background(), "example/managed", testCoordinate())
	assertRejection(t, err, managedpackpromotion.GateRelease)
	assertZeroAcquisition(t, acquisition)
	assertEmptyDirectory(t, temporary)
}

func TestAdapterAcquireRejectsOriginDriftAndMalformedOrigins(t *testing.T) {
	originCommit := strings.Repeat("b", 40)
	tests := []struct {
		name     string
		manifest string
		mutate   func(*fakeSource)
	}{
		{
			name:     "commit drift",
			manifest: fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"upstream/source","commit":%q}]}`, originCommit),
			mutate: func(source *fakeSource) {
				source.commitCandidates[originCommit] = validCommitCandidate("upstream/source", strings.Repeat("9", 40))
			},
		},
		{
			name:     "private origin",
			manifest: fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"upstream/source","commit":%q}]}`, originCommit),
			mutate: func(source *fakeSource) {
				candidate := source.commitCandidates[originCommit]
				candidate.Public = false
				source.commitCandidates[originCommit] = candidate
			},
		},
		{
			name:     "wrong origin identity",
			manifest: fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"upstream/source","commit":%q}]}`, originCommit),
			mutate: func(source *fakeSource) {
				candidate := source.commitCandidates[originCommit]
				candidate.Repository = "upstream/other"
				source.commitCandidates[originCommit] = candidate
			},
		},
		{name: "missing origins", manifest: `{}`, mutate: func(*fakeSource) {}},
		{name: "null origins", manifest: `{"origins":null}`, mutate: func(*fakeSource) {}},
		{name: "malformed repository", manifest: fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"https://example.test/repo","commit":%q}]}`, originCommit), mutate: func(*fakeSource) {}},
		{name: "malformed commit", manifest: `{"origins":[{"id":"upstream","repository":"upstream/source","commit":"main"}]}`, mutate: func(*fakeSource) {}},
		{name: "oversized manifest", manifest: strings.Repeat(" ", (8<<20)+1), mutate: func(*fakeSource) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			temporary := t.TempDir()
			t.Setenv("TMPDIR", temporary)
			source := validAcquisitionSource(t, test.manifest)
			test.mutate(source)

			acquisition, err := githubacquisition.New(source).Acquire(context.Background(), "example/managed", testCoordinate())
			assertRejection(t, err, managedpackpromotion.GateOrigins)
			assertZeroAcquisition(t, acquisition)
			assertEmptyDirectory(t, temporary)
		})
	}
}

func TestAdapterAcquireRejectsOriginEvidenceThatMovesDuringSnapshot(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	originCommit := strings.Repeat("b", 40)
	manifest := fmt.Sprintf(`{"origins":[{"id":"upstream","repository":"upstream/source","commit":%q}]}`, originCommit)
	source := validAcquisitionSource(t, manifest)
	source.snapshotHook = func(_ string, candidate packsync.Candidate) error {
		if candidate.Commit == originCommit {
			moved := source.commitCandidates[originCommit]
			moved.RepositoryID++
			source.commitCandidates[originCommit] = moved
		}
		return nil
	}

	acquisition, err := githubacquisition.New(source).Acquire(context.Background(), "example/managed", testCoordinate())
	assertRejection(t, err, managedpackpromotion.GateOrigins)
	assertZeroAcquisition(t, acquisition)
	assertEmptyDirectory(t, temporary)
}

func TestAdapterAcquireRejectsLinksAndCleansPartialSnapshots(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	source := validAcquisitionSource(t, `{"origins":[]}`)
	source.snapshotHook = func(snapshot string, _ packsync.Candidate) error {
		return os.Symlink("pack.json", filepath.Join(snapshot, "alias"))
	}

	acquisition, err := githubacquisition.New(source).Acquire(context.Background(), "example/managed", testCoordinate())
	if err == nil || !strings.Contains(err.Error(), "forbidden symbolic link") {
		t.Fatalf("Acquire() error = %v, want forbidden symbolic link", err)
	}
	assertZeroAcquisition(t, acquisition)
	assertEmptyDirectory(t, temporary)
}

type fakeFile struct {
	content string
	mode    os.FileMode
}

type fakeSource struct {
	t                 *testing.T
	releases          []packsync.Release
	releaseCandidates []packsync.Candidate
	commitCandidates  map[string]packsync.Candidate
	snapshots         map[string]map[string]fakeFile
	snapshotHook      func(string, packsync.Candidate) error
}

func newFakeSource(t *testing.T) *fakeSource {
	t.Helper()
	return &fakeSource{t: t, commitCandidates: map[string]packsync.Candidate{}, snapshots: map[string]map[string]fakeFile{}}
}

func (source *fakeSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return append([]packsync.Release(nil), source.releases...), nil
}

func (source *fakeSource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, _ packsync.Release) (packsync.Candidate, error) {
	if len(source.releaseCandidates) == 0 {
		return packsync.Candidate{}, fmt.Errorf("unexpected ResolveRelease call")
	}
	result := source.releaseCandidates[0]
	source.releaseCandidates = source.releaseCandidates[1:]
	return result, nil
}

func (source *fakeSource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	result, ok := source.commitCandidates[sha]
	if !ok {
		return packsync.Candidate{}, fmt.Errorf("unexpected ResolveCommit(%q)", sha)
	}
	return result, nil
}

func (source *fakeSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	if err := os.MkdirAll(temporaryRoot, 0o755); err != nil {
		return err
	}
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := os.Mkdir(snapshot, 0o755); err != nil {
		return err
	}
	for path, file := range source.snapshots[candidate.Commit] {
		target := filepath.Join(snapshot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(file.content), file.mode); err != nil {
			return err
		}
	}
	if source.snapshotHook != nil {
		if err := source.snapshotHook(snapshot, candidate); err != nil {
			return err
		}
	}
	err := visit(snapshot)
	removeErr := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return removeErr
}

func validSourceRelease(tag string) packsync.Release {
	return packsync.Release{ID: 202, Tag: tag, Immutable: true, PublishedAt: mustTime()}
}

func validReleaseCandidate(repository, commit string) packsync.Candidate {
	release := validSourceRelease("pack-v1.2.3")
	return packsync.Candidate{
		Repository: repository, RepositoryID: 101, Public: true, Release: &release,
		TagRefName: "refs/tags/pack-v1.2.3", TagRefType: "tag", TagRefSHA: strings.Repeat("d", 40),
		TagObjects: []packsync.TagObject{{SHA: strings.Repeat("d", 40), TargetSHA: commit, TargetType: "commit"}},
		Commit:     commit, Tree: strings.Repeat("c", 40),
	}
}

func validCommitCandidate(repository, commit string) packsync.Candidate {
	return packsync.Candidate{Repository: repository, RepositoryID: 303, Public: true, Commit: commit, Tree: strings.Repeat("e", 40)}
}

func releaseOnlySource(t *testing.T) *fakeSource {
	t.Helper()
	commit := strings.Repeat("a", 40)
	source := newFakeSource(t)
	source.releases = []packsync.Release{validSourceRelease("pack-v1.2.3")}
	source.releaseCandidates = []packsync.Candidate{validReleaseCandidate("example/managed", commit)}
	return source
}

func validAcquisitionSource(t *testing.T, manifest string) *fakeSource {
	t.Helper()
	projectCommit := strings.Repeat("a", 40)
	originCommit := strings.Repeat("b", 40)
	source := releaseOnlySource(t)
	source.releaseCandidates = append(source.releaseCandidates, validReleaseCandidate("example/managed", projectCommit))
	source.commitCandidates[originCommit] = validCommitCandidate("upstream/source", originCommit)
	source.snapshots[projectCommit] = map[string]fakeFile{"pack.json": {content: manifest, mode: 0o644}}
	source.snapshots[originCommit] = map[string]fakeFile{"LICENSE": {content: "license\n", mode: 0o644}}
	return source
}

func testCoordinate() managedpackpromotion.Coordinate {
	return managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"}
}

func mustTime() (result time.Time) {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

func assertFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != content {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("mode(%q) = %04o, want %04o", path, info.Mode().Perm(), mode)
	}
}

func assertRejection(t *testing.T, err error, gate managedpackpromotion.Gate) {
	t.Helper()
	var rejection *managedpackpromotion.RejectionError
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want typed rejection", err)
	}
	if rejection.Gate != gate {
		t.Fatalf("rejection gate = %q, want %q", rejection.Gate, gate)
	}
}

func assertZeroAcquisition(t *testing.T, acquisition managedpackpromotion.Acquisition) {
	t.Helper()
	if acquisition.ProjectRoot != "" || acquisition.OriginRoots != nil || acquisition.Cleanup != nil || acquisition.Release.Project != "" {
		t.Fatalf("partial acquisition returned: %#v", acquisition)
	}
}

func assertEmptyDirectory(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", root, err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary root contains partial acquisition: %#v", entries)
	}
}
