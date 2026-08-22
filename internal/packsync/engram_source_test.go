package packsync

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestEngramInspectionRejectsAlteredOrIncompleteSelectedContent(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var lock Lock
	lockData, err := os.ReadFile(filepath.Join(root, "bundle", "sources", "engram-source.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	snapshot := t.TempDir()
	if err := copyTreeError(filepath.Join(root, "bundle", "skills", "engram-memory-cli"), filepath.Join(snapshot, "skills", "engram-memory-cli")); err != nil {
		t.Fatal(err)
	}
	license, err := os.ReadFile(filepath.Join(root, "bundle", "notices", "engram-mit"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "LICENSE"), license, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "altered upstream skill content", mutate: func(t *testing.T, repository string) {
			writeFile(t, filepath.Join(repository, "bundle", "skills", "engram-memory-cli", "SKILL.md"), "altered\n")
		}},
		{name: "incomplete selected tree", mutate: func(t *testing.T, repository string) {
			if err := os.Remove(filepath.Join(repository, "bundle", "skills", "engram-memory-cli", "references", "curation.md")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			if err := os.CopyFS(filepath.Join(repository, "bundle"), os.DirFS(filepath.Join(root, "bundle"))); err != nil {
				t.Fatal(err)
			}
			environment := testprocess.Env(t)
			for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "packy@example.invalid"}, {"config", "user.name", "Packy Test"}, {"add", "bundle"}, {"commit", "-qm", "fixture"}} {
				command := exec.Command("git", args...)
				command.Dir = repository
				command.Env = environment
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, output)
				}
			}
			test.mutate(t, repository)
			plan, err := (Engine{Source: &fixtureSource{root: snapshot, candidate: lock.Candidate}}).Check(context.Background(), CheckRequest{
				RepositoryRoot: repository,
				SourceID:       "engram-source",
				AcquisitionDir: t.TempDir(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Status != "blocked" || !strings.Contains(strings.Join(plan.Blockers, "\n"), "local selected-resource drift from authoritative lock: engram/skill/engram-memory-cli") {
				t.Fatalf("altered selection was not rejected: status=%s blockers=%v", plan.Status, plan.Blockers)
			}
		})
	}
}
