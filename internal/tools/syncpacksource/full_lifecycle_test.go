package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packsync"
)

func sourceResourcesForTest(t *testing.T, sources []packsync.SourceConfig, sourceID string) []packsync.Binding {
	t.Helper()
	for _, source := range sources {
		if source.ID == sourceID {
			return source.Resources
		}
	}
	t.Fatalf("source %q is missing", sourceID)
	return nil
}

type sandboxSource struct {
	root         string
	oldRoot      string
	oldCandidate packsync.Candidate
	candidate    packsync.Candidate
}

func fixtureCandidateWithRelease(candidate packsync.Candidate) packsync.Candidate {
	if candidate.Release != nil {
		return candidate
	}
	release := packsync.Release{
		ID: 1, NodeID: "fixture-release", Tag: "v-fixture-current", Name: "v-fixture-current", Target: "main",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PublishedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Author: packsync.Actor{Login: "fixture", ID: 1, NodeID: "fixture-actor"},
	}
	candidate.Release = &release
	candidate.TagRefName = "refs/tags/" + release.Tag
	candidate.TagRefType = "tag"
	candidate.TagRefSHA = strings.Repeat("c", 40)
	candidate.TagObjects = []packsync.TagObject{{SHA: candidate.TagRefSHA, Name: release.Tag, TargetSHA: candidate.Commit, TargetType: "commit", Verification: packsync.Verification{Reason: "unsigned"}}}
	return candidate
}

func writeFixtureLock(t *testing.T, path string, lock packsync.Lock) {
	t.Helper()
	data, _, err := packsync.CanonicalSourceLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (source *sandboxSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.oldCandidate.Release, *source.candidate.Release}, nil
}
func (source *sandboxSource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.oldCandidate
	if release.Tag == source.candidate.Release.Tag {
		candidate = source.candidate
	}
	candidate.Release = &release
	return candidate, nil
}
func (source *sandboxSource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Commit = sha
	candidate.Release = nil
	return candidate, nil
}
func (source *sandboxSource) snapshotRoot(candidate packsync.Candidate) string {
	if candidate.Commit == source.oldCandidate.Commit {
		return source.oldRoot
	}
	return source.root
}

func (source *sandboxSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	target := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeErrorForTest(source.snapshotRoot(candidate), target); err != nil {
		return err
	}
	err := visit(target)
	cleanup := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanup
}

type sandboxValidator struct{ bundleCalls, suiteCalls int }

func (validator *sandboxValidator) ValidateBundle(context.Context, string, string) error {
	validator.bundleCalls++
	return nil
}
func (validator *sandboxValidator) Validate(context.Context, string) error {
	validator.suiteCalls++
	return nil
}

func repositoryRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func readJSONForTest(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
func copyTreeForTest(t *testing.T, source, target string) {
	t.Helper()
	if err := copyTreeErrorForTest(source, target); err != nil {
		t.Fatal(err)
	}
}
func copyTreeErrorForTest(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}
func gitForTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func cloneForTest(t *testing.T, source, destination string) {
	t.Helper()
	gitForTest(t, filepath.Dir(destination), "clone", "-q", "--no-hardlinks", source, destination)
	gitForTest(t, destination, "config", "gc.auto", "0")
	gitForTest(t, destination, "config", "maintenance.auto", "false")
}

func initForTest(t *testing.T, repository string) {
	t.Helper()
	gitForTest(t, repository, "init", "-q")
	gitForTest(t, repository, "config", "gc.auto", "0")
	gitForTest(t, repository, "config", "maintenance.auto", "false")
}
