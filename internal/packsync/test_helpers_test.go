package packsync

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureSource struct {
	root      string
	candidate Candidate
}

func (source *fixtureSource) Releases(context.Context, SourceConfig) ([]Release, error) {
	if source.candidate.Release == nil {
		return nil, nil
	}
	return []Release{*source.candidate.Release}, nil
}

func (source *fixtureSource) ResolveRelease(_ context.Context, _ SourceConfig, release Release) (Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}

func (source *fixtureSource) ResolveCommit(_ context.Context, _ SourceConfig, sha string) (Candidate, error) {
	candidate := source.candidate
	candidate.Release = nil
	candidate.TagObjects = nil
	candidate.TagRefName = ""
	candidate.TagRefType = ""
	candidate.TagRefSHA = ""
	candidate.Commit = sha
	return candidate, nil
}

func (source *fixtureSource) WithSnapshot(_ context.Context, _ Candidate, temporaryRoot string, visit func(string) error) error {
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeError(source.root, snapshot); err != nil {
		return err
	}
	err := visit(snapshot)
	cleanupErr := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanupErr
}

func acceptedCandidate() Candidate {
	verifiedAt := time.Date(2026, 7, 8, 13, 20, 40, 0, time.UTC)
	signatureHash := strings.Repeat("a", 64)
	payloadHash := strings.Repeat("b", 64)
	return Candidate{
		Repository:       "example/source",
		RepositoryID:     1,
		RepositoryNodeID: "repository-node",
		RepositoryHTML:   "https://github.com/example/source",
		RepositoryClone:  "https://github.com/example/source.git",
		RepositoryAPI:    "https://api.github.com/repos/example/source",
		Visibility:       "public",
		Owner:            "example",
		OwnerID:          2,
		OwnerNodeID:      "owner-node",
		Public:           true,
		Release: &Release{
			ID: 3, NodeID: "release-node", Tag: "v1.0.0", Name: "v1.0.0", Target: "main",
			CreatedAt: verifiedAt, PublishedAt: verifiedAt.Add(time.Minute),
			Author: Actor{Login: "maintainer", ID: 4, NodeID: "author-node"},
		},
		TagRefName: "refs/tags/v1.0.0", TagRefType: "tag", TagRefSHA: strings.Repeat("c", 40),
		TagObjects: []TagObject{{SHA: strings.Repeat("c", 40), Name: "v1.0.0", TargetSHA: strings.Repeat("d", 40), TargetType: "commit", Verification: Verification{Reason: "unsigned"}}},
		Commit:     strings.Repeat("d", 40), CommitNodeID: "commit-node", Tree: strings.Repeat("e", 40),
		Parents:      []string{strings.Repeat("f", 40)},
		CommitVerify: Verification{Verified: true, Reason: "valid", VerifiedAt: &verifiedAt, SignatureSHA256: &signatureHash, PayloadSHA256: &payloadHash},
	}
}

func acceptedCandidateFor(repository string) Candidate {
	candidate := acceptedCandidate()
	candidate.Repository = repository
	candidate.Owner = strings.Split(repository, "/")[0]
	candidate.RepositoryHTML = "https://github.com/" + repository
	candidate.RepositoryClone = candidate.RepositoryHTML + ".git"
	candidate.RepositoryAPI = "https://api.github.com/repos/" + repository
	return candidate
}

func acceptingBundleValidator() BundleValidator {
	return BundleValidatorFunc(func(context.Context, string, string) error { return nil })
}

func resealPlan(t *testing.T, plan *Plan) {
	t.Helper()
	plan.PlanID = ""
	id, err := seal(*plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanID = id
}

func failOnce(point FaultPoint) FaultInjector {
	fired := false
	return func(observed FaultPoint) error {
		if observed == point && !fired {
			fired = true
			return errors.New("injected " + string(point))
		}
		return nil
	}
}

func newCheckRequest(t *testing.T, repository string) CheckRequest {
	t.Helper()
	return CheckRequest{RepositoryRoot: repository, AcquisitionDir: t.TempDir()}
}

func checkWith(t *testing.T, repository string, source Source) Plan {
	t.Helper()
	plan, err := (Engine{allowBootstrap: true, Source: source}).Check(context.Background(), CheckRequest{
		RepositoryRoot: repository,
		SourceID:       "example-source",
		AcquisitionDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func tinyRepository(t *testing.T) (string, string) {
	t.Helper()
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"example-source","provider":"github","repository":"example/source","selector":{"mode":"stable-release"},"resources":[{"pack_id":"example","kind":"skill","resource_id":"one","upstream_path":"skills/one"}]}]}`)
	writeFile(t, filepath.Join(repository, "bundle", "packs", "example", "pack.json"), `{"schema_version":1,"id":"example","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/one"}]}`)
	writeFile(t, filepath.Join(repository, "bundle", "skills", "one", "SKILL.md"), "same\n")
	snapshot := t.TempDir()
	writeFile(t, filepath.Join(snapshot, "skills", "one", "SKILL.md"), "same\n")
	return repository, snapshot
}

func writeFile(t *testing.T, name, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTreeError(source, destination string) error {
	return filepath.WalkDir(source, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
