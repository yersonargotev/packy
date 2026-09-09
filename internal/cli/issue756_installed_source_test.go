package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/testprocess"
)

const installedSourceTestRelease = "v0.0.756"
const installedSourceTestManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [
    {
      "kind": "instruction",
      "id": "guidance",
      "source": "instructions/guidance.md",
      "description": "Explains the reviewed guidance",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "instruction",
          "name": "guidance",
          "invocation": "guidance",
          "mode": "native",
          "sharing": "shared",
          "capabilities": [
            {
              "type": "project-instruction",
              "project_instruction": {
                "id": "guidance",
                "source": "instructions/guidance.md"
              }
            }
          ]
        }
      ],
      "surface_exclusions": []
    }
  ]
}
`

func installedSourceWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installedSourceRepository(t *testing.T, root string) *git.Repository {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	installedSourceWrite(t, filepath.Join(project, "pack.json"), installedSourceTestManifest)
	installedSourceWrite(t, filepath.Join(project, "instructions/guidance.md"), "managed guidance\n")
	validation, err := managedpack.ValidateProject(context.Background(), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := managedpack.MaterializeClosure(context.Background(), project, filepath.Join(root, "bundle"), validation); err != nil {
		t.Fatal(err)
	}
	createSkillSourceAt(t, filepath.Join(root, "bundle/skills"))
	installedSourceWrite(t, filepath.Join(root, "managed-packs/registry.json"), `{"schema_version":1,"packs":[{"pack_id":"example","project":"owner/example"}]}`)
	record := managedpack.AdmissionRecord{SchemaVersion: 1, PackID: "example", PackVersion: "1.0.0", Project: "owner/example", RepositoryID: 101, ReleaseID: 202, ReleaseImmutable: true, Tag: "pack-v1.0.0", TagRefType: "commit", TagRefSHA: strings.Repeat("1", 40), Commit: strings.Repeat("1", 40), RootTree: strings.Repeat("2", 40), ManifestSHA256: validation.ManifestSHA256, ClosureSHA256: validation.ClosureSHA256, Files: validation.Files, TagObjects: []managedpack.TagObject{}}
	if _, err := managedpack.WriteAdmissionRecord(filepath.Join(root, "managed-packs/admissions"), record); err != nil {
		t.Fatal(err)
	}
	if err := managedpack.ValidateRepositoryIntegrity(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	installedSourceCommit(t, repo, "current")
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag(installedSourceTestRelease, head.Hash(), nil); err != nil {
		t.Fatal(err)
	}
	return repo
}

func installedSourceCommit(t *testing.T, repo *git.Repository, message string) plumbing.Hash {
	t.Helper()
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{Author: &object.Signature{Name: "Source test", Email: "source@example.invalid", When: time.Unix(1700000000, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestInstalledSourceCatalogRejectsBeforeCLIAndTUIConsumption(t *testing.T) {
	withVersion(t, installedSourceTestRelease)
	cases := []struct {
		name, want string
		mutate     func(*testing.T, string, *git.Repository)
	}{
		{"clean", "", nil},
		{"unrelated untracked", "", func(t *testing.T, r string, _ *git.Repository) {
			installedSourceWrite(t, filepath.Join(r, "notes.txt"), "personal notes")
		}},
		{"manifest", "integrity", func(t *testing.T, r string, _ *git.Repository) {
			installedSourceWrite(t, filepath.Join(r, "bundle/packs/example/pack.json"), strings.ReplaceAll(installedSourceTestManifest, "Example Managed Pack", "UNREVIEWED DEFAULT SOURCE"))
		}},
		{"resource", "integrity", func(t *testing.T, r string, _ *git.Repository) {
			installedSourceWrite(t, filepath.Join(r, "bundle/instructions/guidance.md"), "UNREVIEWED DEFAULT SOURCE")
		}},
		{"mode", "integrity", func(t *testing.T, r string, _ *git.Repository) {
			if err := os.Chmod(filepath.Join(r, "bundle/instructions/guidance.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"stale before schema", "stale", func(t *testing.T, r string, repo *git.Repository) {
			installedSourceWrite(t, filepath.Join(r, "bundle/packs/example/pack.json"), `{"schema_version":999}`)
			installedSourceCommit(t, repo, "older incompatible schema")
		}},
		{"missing", "missing or invalid", func(t *testing.T, r string, _ *git.Repository) {
			if err := os.RemoveAll(r); err != nil {
				t.Fatal(err)
			}
		}},
		{"not git", "missing or invalid", func(t *testing.T, r string, _ *git.Repository) {
			if err := os.RemoveAll(filepath.Join(r, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			root := bootstrap.DefaultInstalledSourceRoot(home)
			repo := installedSourceRepository(t, root)
			if tc.mutate != nil {
				tc.mutate(t, root, repo)
			}
			runner := &fakeRunner{}
			cwd := t.TempDir()
			opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": ""}, Getwd: func() (string, error) { return cwd, nil }, Runner: runner, SetupHealthDiagnose: func() (setuphealth.Report, error) { return setuphealth.Report{}, nil }}
			before := snapshotTree(t, home)
			commands := [][]string{{"list", "--json"}, {"show", "example"}, {"activate", "example", "--surface", "codex", "--dry-run"}}
			for _, args := range commands {
				output, err := executeCommand(t, NewRootCommand(opts), args...)
				if tc.want == "" {
					if args[0] == "list" && (err != nil || !strings.Contains(output, "Example Managed Pack")) {
						t.Fatalf("clean list: %v %s", err, output)
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "packy init") {
					t.Fatalf("%v: error=%v output=%s", args, err, output)
				}
				if strings.Contains(output, "UNREVIEWED DEFAULT SOURCE") {
					t.Fatalf("displayed unreviewed content: %s", output)
				}
			}
			dashboard, err := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults())).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (len(dashboard.Global.Packs) != 0 || len(dashboard.Setup.Blockers) == 0 || !strings.Contains(dashboard.Setup.Blockers[0].Cause, tc.want)) {
				t.Fatalf("TUI consumed invalid catalog: %#v", dashboard)
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatal("catalog diagnosis mutated installed source, Git metadata, or user state")
			}
			if len(runner.calls) != 0 {
				t.Fatalf("diagnosis ran external commands: %#v", runner.calls)
			}
		})
	}
}

func TestInstalledSourceExplicitAndRepositorySourcesRemainEditable(t *testing.T) {
	withVersion(t, installedSourceTestRelease)
	for _, origin := range []string{"override", "repository"} {
		t.Run(origin, func(t *testing.T) {
			root := t.TempDir()
			installedSourceRepository(t, root)
			installedSourceWrite(t, filepath.Join(root, "bundle/packs/example/pack.json"), strings.ReplaceAll(installedSourceTestManifest, "Example Managed Pack", "Editable source"))
			home := t.TempDir()
			cwd := t.TempDir()
			env := MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": ""}
			if origin == "override" {
				env["PACKY_SKILLS_SOURCE"] = filepath.Join(root, "bundle/skills")
			} else {
				cwd = root
			}
			output, err := executeCommand(t, NewRootCommand(Options{Env: env, Getwd: func() (string, error) { return cwd, nil }, Runner: &fakeRunner{}}), "list", "--json")
			if err != nil || !strings.Contains(output, "Editable source") {
				t.Fatalf("editable %s source: %v %s", origin, err, output)
			}
		})
	}
}

func TestPackageInstalledSourceSmokeAndSafeRemediation(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "packy")
	build := exec.Command("go", "build", "-o", executable, "-ldflags", "-X github.com/yersonargotev/packy/internal/version.Value="+installedSourceTestRelease, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	remote := t.TempDir()
	repo := installedSourceRepository(t, remote)
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	installedSourceWrite(t, filepath.Join(remote, "README"), "older release")
	old := installedSourceCommit(t, repo, "older release fixture")
	if _, err := repo.CreateTag("v0.0.755", old, nil); err != nil {
		t.Fatal(err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: head.Hash()}); err != nil {
		t.Fatal(err)
	}
	env := testprocess.Env(t)
	home := ""
	for _, entry := range env {
		if strings.HasPrefix(entry, "HOME=") {
			home = strings.TrimPrefix(entry, "HOME=")
		}
	}
	cwd := t.TempDir()
	run := func(args ...string) (string, error) {
		cmd := exec.Command(executable, args...)
		cmd.Dir = cwd
		cmd.Env = append([]string(nil), env...)
		if len(args) > 0 && args[0] == "list" {
			for i, entry := range cmd.Env {
				if strings.HasPrefix(entry, "PATH=") {
					cmd.Env[i] = "PATH="
				}
			}
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	mustRun := func(args ...string) {
		t.Helper()
		if output, err := run(args...); err != nil {
			t.Fatalf("%v: %v %s", args, err, output)
		}
	}
	mustRun("init", "--repository-url", remote, "--repository-ref", "v0.0.755")
	root := bootstrap.DefaultInstalledSourceRoot(home)
	assertRejected := func(want string) {
		t.Helper()
		before := snapshotTree(t, home)
		output, err := run("list", "--json")
		if err == nil || !strings.Contains(output, want) || !strings.Contains(output, "packy init") {
			t.Fatalf("wanted %s: %v %s", want, err, output)
		}
		if snapshotTree(t, home) != before {
			t.Fatal("failed package diagnosis mutated HOME")
		}
	}
	assertRejected("stale")
	mustRun("init", "--repository-url", remote)
	mustRun("list", "--json")
	for _, path := range []string{"bundle/packs/example/pack.json", "bundle/instructions/guidance.md"} {
		installedSourceWrite(t, filepath.Join(root, path), "UNREVIEWED DEFAULT SOURCE")
		assertRejected("integrity")
		preserved := filepath.Join(t.TempDir(), "preserved-source")
		if err := os.Rename(root, preserved); err != nil {
			t.Fatal(err)
		}
		before := snapshotTree(t, preserved)
		mustRun("init", "--repository-url", remote)
		mustRun("list", "--json")
		if snapshotTree(t, preserved) != before {
			t.Fatal("remediation modified preserved local changes")
		}
	}
	explicit := filepath.Join(t.TempDir(), "explicit-source")
	mustRun("init", "--repository-url", remote, "--source-root", explicit)
	installedSourceWrite(t, filepath.Join(explicit, "bundle/packs/example/pack.json"), strings.ReplaceAll(installedSourceTestManifest, "Example Managed Pack", "Editable package source"))
	env = append(env, "PACKY_SKILLS_SOURCE="+filepath.Join(explicit, "bundle/skills"))
	output, err := run("list", "--json")
	if err != nil {
		t.Fatalf("explicit package source: %v %s", err, output)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil || !strings.Contains(output, "Editable package source") {
		t.Fatalf("editable package output: %v %s", err, output)
	}
}

func TestInstalledSourceHealthReportsAdmissionFailureForActiveIntent(t *testing.T) {
	withVersion(t, installedSourceTestRelease)
	home := t.TempDir()
	root := bootstrap.DefaultInstalledSourceRoot(home)
	installedSourceRepository(t, root)
	cwd := t.TempDir()
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": ""}, Getwd: func() (string, error) { return cwd, nil }, Runner: runner, Terminal: &fakeTerminal{interactive: true, approve: true}}
	output, err := executeCommand(t, NewRootCommand(opts), "activate", "example", "--surface", "codex")
	if err != nil {
		t.Fatalf("prepare active intent: %v %s", err, output)
	}
	installedSourceWrite(t, filepath.Join(root, "bundle/instructions/guidance.md"), "UNREVIEWED DEFAULT SOURCE")
	before := snapshotTree(t, home)
	runner.calls = nil
	for _, command := range []string{"doctor", "audit"} {
		output, err := executeCommand(t, NewRootCommand(opts), command, "--json")
		if err == nil || !strings.Contains(output, "integrity") || !strings.Contains(output, "packy init") || strings.Contains(output, "UNREVIEWED DEFAULT SOURCE") {
			t.Fatalf("%s swallowed source failure: %v %s", command, err, output)
		}
	}
	if snapshotTree(t, home) != before {
		t.Fatal("health diagnosis mutated source or active receipt")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("health diagnosis ran commands: %#v", runner.calls)
	}
}

func TestInstalledSourceHealthPreservesLateValidationFailure(t *testing.T) {
	withVersion(t, installedSourceTestRelease)
	home := t.TempDir()
	root := bootstrap.DefaultInstalledSourceRoot(home)
	installedSourceRepository(t, root)
	cwd := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": ""}, Getwd: func() (string, error) { return cwd, nil }, Runner: &fakeRunner{}, Terminal: &fakeTerminal{interactive: true, approve: true}}
	if output, err := executeCommand(t, NewRootCommand(opts), "activate", "example", "--surface", "codex"); err != nil {
		t.Fatalf("prepare active intent: %v %s", err, output)
	}
	path := filepath.Join(root, "bundle/instructions/guidance.md")
	for _, command := range []string{"doctor", "audit"} {
		t.Run(command, func(t *testing.T) {
			installedSourceWrite(t, path, "managed guidance\n")
			before := ""
			// Composition resolves the executable only after its initial catalog load.
			// Replace the resource here to exercise the later observation's validator.
			opts.ClaudeLookPath = func(string) (string, error) {
				installedSourceWrite(t, path, "UNREVIEWED DEFAULT SOURCE")
				before = snapshotTree(t, home)
				return "", os.ErrNotExist
			}
			output, err := executeCommand(t, NewRootCommand(opts), command, "--json")
			if before == "" || err == nil || !strings.Contains(output, "integrity") || !strings.Contains(output, "packy init") || strings.Contains(output, "UNREVIEWED DEFAULT SOURCE") {
				t.Fatalf("late %s validation: %v %s", command, err, output)
			}
			if snapshotTree(t, home) != before {
				t.Fatal("late diagnosis mutated source or receipt")
			}
		})
	}
}
