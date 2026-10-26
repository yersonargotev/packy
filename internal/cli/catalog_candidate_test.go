package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestCandidatePreviewUsesCompleteSnapshotInOwnedWorkspace(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("candidate")
	release := catalogReleaseFixture(t, strings.Repeat("a", 40), pack)
	assets := t.TempDir()
	for _, asset := range release.Assets {
		if err := os.WriteFile(filepath.Join(assets, asset.Name), asset.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	before := snapshotTree(t, fixture.home)
	args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "install", pack.ID(), "--surface", "agents", "--dry-run", "--json"}
	out, err := executeCommand(t, NewRootCommand(fixture.options), args...)
	if err != nil || !strings.Contains(out, `"report":"project-install-preview"`) || !strings.Contains(out, `"catalog_snapshot":"`+release.Commit+`"`) {
		t.Fatalf("candidate preview: %v\n%s", err, out)
	}
	if snapshotTree(t, fixture.home) != before {
		t.Fatal("candidate preview changed ambient home")
	}
	if _, err := os.Stat(filepath.Join(workspace, "project", "packy.lock.json")); !os.IsNotExist(err) {
		t.Fatalf("preview installed a project contract: %v", err)
	}
}

func TestCandidateWorkspaceRejectsConflictsAndRetainedDriftWithoutMutation(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("workspace")
	release := catalogReleaseFixture(t, strings.Repeat("a", 40), pack)
	assets := t.TempDir()
	for _, asset := range release.Assets {
		if err := os.WriteFile(filepath.Join(assets, asset.Name), asset.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name    string
		prepare func(string)
	}{
		{"existing unrelated directory", func(root string) { writeAuthoringFile(t, filepath.Join(root, "personal.txt"), "keep\n") }},
		{"retained source drift", func(root string) {
			writeAuthoringFile(t, filepath.Join(root, "snapshots", strings.Repeat("a", 40), "packs", pack.ID(), "instructions", pack.ID()+".md"), "drift\n")
		}},
		{"missing Git boundary", func(root string) {
			if err := os.RemoveAll(filepath.Join(root, "project", ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink escape", func(root string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "project", "escape")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			opts := Options{Terminal: &fakeTerminal{}}
			args := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--", "list", "--json"}
			if test.name != "existing unrelated directory" {
				if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
					t.Fatalf("prepare workspace: %v %s", err, out)
				}
			}
			test.prepare(workspace)
			before := snapshotTree(t, workspace)
			if test.name == "missing Git boundary" {
				if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
					t.Fatalf("non-Git access: %v %s", err, out)
				}
				args = append(args[:len(args)-2], "install", pack.ID(), "--surface", "agents", "--dry-run", "--json")
			}
			if out, err := executeCommand(t, NewRootCommand(opts), args...); err == nil {
				t.Fatalf("conflicting workspace accepted: %s", out)
			}
			if snapshotTree(t, workspace) != before {
				t.Fatal("rejected workspace changed existing state")
			}
		})
	}
}

func TestCandidateCannotDiscoverAnEnclosingPersonalProject(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("isolated")
	release := catalogReleaseFixture(t, strings.Repeat("a", 40), pack)
	assets := t.TempDir()
	for _, asset := range release.Assets {
		if err := os.WriteFile(filepath.Join(assets, asset.Name), asset.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, damage := range []string{"missing HEAD", "empty HEAD", "missing objects"} {
		t.Run(damage, func(t *testing.T) {
			parent := t.TempDir()
			writeTestGitWorktree(t, parent)
			writeAuthoringFile(t, filepath.Join(parent, "AGENTS.md"), "Enclosing personal guidance\n")
			workspace := filepath.Join(parent, "workspace")
			opts := Options{Terminal: &fakeTerminal{interactive: true, approve: true}}
			prefix := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}
			if out, err := executeCommand(t, NewRootCommand(opts), append(append([]string{}, prefix...), "list", "--json")...); err != nil {
				t.Fatalf("prepare workspace: %v %s", err, out)
			}
			head := filepath.Join(workspace, "project", ".git", "HEAD")
			switch damage {
			case "missing HEAD":
				if err := os.Remove(head); err != nil {
					t.Fatal(err)
				}
			case "empty HEAD":
				if err := os.WriteFile(head, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			case "missing objects":
				if err := os.RemoveAll(filepath.Join(workspace, "project", ".git", "objects")); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotTree(t, parent)
			for _, operation := range [][]string{{"install", pack.ID(), "--surface", "agents", "--dry-run", "--json"}, {"verify", "--json"}, {"uninstall", pack.ID(), "--dry-run", "--json"}} {
				out, err := executeCommand(t, NewRootCommand(opts), append(append([]string{}, prefix...), operation...)...)
				if err == nil || !strings.Contains(err.Error(), "candidate workspace Git boundary") {
					t.Fatalf("damaged Git boundary accepted: %v %s", err, out)
				}
				if snapshotTree(t, parent) != before {
					t.Fatal("candidate operation changed enclosing project")
				}
			}
		})
	}
}
