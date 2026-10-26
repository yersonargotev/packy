package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue876RetiredSurfaceRejectsEveryLifecycleBeforeMutation(t *testing.T) {
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.PortableAllSurfaces("retirement"))
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts := fixture.options
	opts.Getwd = func() (string, error) { return project, nil }
	retired := filepath.Join(fixture.home, "xdg", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(retired), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retired, []byte("// personal configuration\n{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	beforeHome, beforeProject := snapshotTree(t, fixture.home), snapshotTree(t, project)
	for _, args := range [][]string{
		{"activate", "retirement"}, {"deactivate", "retirement"}, {"update", "retirement"}, {"status", "retirement"},
		{"install", "retirement"}, {"uninstall", "retirement"},
		{"activate", "retirement", "--project"}, {"deactivate", "retirement", "--project"}, {"update", "retirement", "--project"}, {"status", "retirement", "--project"},
		{"check", "retirement", "--dry-run"},
	} {
		if args[0] != "check" {
			args = append(args, "--json")
		}
		args = append(args, "--surface", "opencode")
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err == nil || !strings.Contains(strings.ToLower(out+err.Error()), "unsupported") || !strings.Contains(out+err.Error(), "claude") || !strings.Contains(out+err.Error(), "codex") {
			t.Errorf("%v: error=%v output=%s", args, err, out)
		}
		if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
			t.Fatalf("%v changed user state", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"install", "--help"}, {"activate", "--help"}, {"uninstall", "--help"}} {
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil || strings.Contains(strings.ToLower(out), "opencode") {
			t.Fatalf("advertised retired surface: %v %s", err, out)
		}
	}
}

func TestIssue876OfflineContractsAndAuditRejectRetiredState(t *testing.T) {
	for _, state := range []string{"global receipt", "project manifest", "project lock"} {
		t.Run(state, func(t *testing.T) {
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.PortableAllSurfaces("retirement"))
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts := fixture.options
			opts.Getwd = func() (string, error) { return project, nil }
			command, path := "install", filepath.Join(project, "packy.json")
			if state == "global receipt" {
				command, path = "activate", filepath.Join(fixture.home, ".packy", "packs.json")
			}
			if state == "project lock" {
				path = filepath.Join(project, "packy.lock.json")
			}
			if out, err := executeCommand(t, NewRootCommand(opts), command, "retirement", "--surface", "codex"); err != nil {
				t.Fatalf("seed: %v %s", err, out)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.ReplaceAll(string(data), `"codex"`, `"opencode"`))
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			// Offline readers must reject the retired contract independently of catalog availability.
			if state != "global receipt" {
				opts.catalogRootOverride = filepath.Join(t.TempDir(), "missing-catalog")
			}
			beforeHome, beforeProject := snapshotTree(t, fixture.home), snapshotTree(t, project)
			status := []string{"status", "--json"}
			if state != "global receipt" {
				status = append(status, "--project")
			}
			for _, args := range [][]string{status, {"audit", "--json"}} {
				out, err := executeCommand(t, NewRootCommand(opts), args...)
				rejected := err != nil && strings.Contains(strings.ToLower(out+err.Error()), "unsupported")
				if args[0] == "audit" {
					rejected = errors.Is(err, ErrAuditFailures) && strings.Contains(out, `"result":"failures"`)
				}
				if !rejected {
					t.Fatalf("%v accepted retired %s: %v %s", args, state, err, out)
				}
				if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
					t.Fatalf("inspection changed %s", state)
				}
			}
		})
	}
}

func TestIssue876TUIRejectsRetiredSurfaceBeforeObservation(t *testing.T) {
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, testsupport.PortableAllSurfaces("retirement"))
	opts := fixture.options.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	before := snapshotTree(t, fixture.home)
	for _, scope := range []string{"global", "project"} {
		for _, operation := range []string{"install", "configure", "activate", "deactivate", "update", "uninstall", "check"} {
			_, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: operation, PackID: "retirement", Surface: "opencode", Scope: scope})
			if err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
				t.Fatalf("%s %s: %v", scope, operation, err)
			}
			if snapshotTree(t, fixture.home) != before {
				t.Fatal("invalid TUI request mutated user state")
			}
		}
	}
}
