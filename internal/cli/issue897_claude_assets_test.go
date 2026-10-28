package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue897ClaudeAssetClosureExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	project := issue897CatalogProject(t)
	firstID, nextID := strings.Repeat("a", 40), strings.Repeat("b", 40)
	first := pluginExportAssets(t, project, firstID)
	packRoot := filepath.Join(project, "packs", "external-assets")
	editCrossPackManifest(t, project, "external-assets", func(manifest map[string]any) {
		manifest["version"] = "1.0.1"
	})
	writePluginFixtureFile(t, filepath.Join(packRoot, "assets/external-assets-reference.md"), "Updated required asset\n", 0644)
	next := pluginExportAssets(t, project, nextID)

	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			environment := testprocess.Env(t)
			env := adoptionEnvironmentMap(environment)
			directory := t.TempDir()
			writeTestGitWorktree(t, directory)
			acquireSkillAccessCatalog(t, env, directory, first, firstID, "init")
			run := func(args ...string) []byte {
				t.Helper()
				out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, args...)
				if err != nil {
					t.Fatalf("%v: %v\n%s\n%s", args, err, out, stderr)
				}
				return out
			}
			status := func(targeted bool) capabilitypack.JSONStatusReport {
				t.Helper()
				args := []string{"status", "--json"}
				if targeted {
					args = append(args, "external-assets", "--surface", "claude")
				}
				out := run(args...)
				assertStructuredOutput(t, repository, "pack-status.schema.json", string(out))
				var report capabilitypack.JSONStatusReport
				if err := json.Unmarshal(out, &report); err != nil {
					t.Fatal(err)
				}
				if targeted && (len(report.Entries) != 1 || report.Entries[0].Pack != "external-assets") {
					t.Fatalf("targeted status = %s", out)
				}
				return report
			}
			before := snapshotTree(t, env["HOME"])
			if len(status(false).Entries) != 2 {
				t.Fatal("general status omitted an uninstalled Pack")
			}
			status(true)
			if snapshotTree(t, env["HOME"]) != before {
				t.Fatal("empty-state status mutated installation state")
			}
			issue897ApproveCLI(t, binary, directory, environment, "activate", "independent", "--surface", "claude")
			root, install, remove := env["HOME"], "activate", "deactivate"
			if scope == "project" {
				root, install, remove = directory, "install", "uninstall"
			}
			foreign := filepath.Join(root, ".claude", "settings.json")
			writePluginFixtureFile(t, foreign, "{\"personal\":true}\n", 0600)
			issue897ApproveCLI(t, binary, directory, environment, install, "external-assets", "--surface", "claude", "--resource", "skill:workflow")
			tree := filepath.Join(root, ".claude", "skills", "workflow")
			asset := filepath.Join(tree, "references", "external-assets-reference.md")
			issue878RequireContent(t, asset, "Synthetic reference asset")
			issue878RequireContent(t, filepath.Join(tree, "agents", "openai.yaml"), "Reviewed workflow")
			if info, err := os.Stat(filepath.Join(tree, "scripts", "inert.sh")); err != nil || info.Mode().Perm() != 0755 {
				t.Fatal("supporting script mode was not retained")
			}
			if scope == "project" {
				run("verify", "--json")
				issue878RequireContent(t, filepath.Join(directory, "PACKY-NOTICES.md"), "Apache License 2.0")
			} else if status(true).Entries[0].Readiness.Configured != capabilitypack.ReadinessTrue {
				t.Fatal("global composite was not configured")
			}
			foundIndependent := false
			for _, entry := range status(false).Entries {
				if entry.Pack == "independent" {
					foundIndependent = entry.Readiness.Configured == capabilitypack.ReadinessTrue && entry.Projections.Verified > 0
				}
			}
			if !foundIndependent {
				t.Fatal("general status lost the unrelated installed Pack")
			}
			original, err := os.ReadFile(asset)
			if err != nil {
				t.Fatal(err)
			}
			writePluginFixtureFile(t, asset, "User drift\n", 0644)
			before = snapshotTree(t, root)
			if _, _, err := runSkillAccessCLI(t, binary, directory, environment, remove, "external-assets", "--surface", "claude"); err == nil || snapshotTree(t, root) != before {
				t.Fatal("asset drift did not block removal before mutation")
			}
			if err := os.WriteFile(asset, original, 0644); err != nil {
				t.Fatal(err)
			}
			acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
			update := []string{"update", "external-assets", "--surface", "claude"}
			if scope == "project" {
				update = append(update, "--project")
			}
			issue897ApproveCLI(t, binary, directory, environment, update...)
			issue878RequireContent(t, asset, "Updated required asset")
			issue897ApproveCLI(t, binary, directory, environment, remove, "external-assets", "--surface", "claude")
			for _, path := range []string{tree, filepath.Join(root, ".claude", "skills", "helper"), filepath.Join(root, ".claude", "assets", "reference", "RESOURCE")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("retired projection remains: %s (%v)", path, err)
				}
			}
			if got, err := os.ReadFile(foreign); err != nil || string(got) != "{\"personal\":true}\n" {
				t.Fatal("foreign Claude settings changed")
			}
			if _, err := os.Stat(filepath.Join(env["HOME"], "executed")); !os.IsNotExist(err) {
				t.Fatal("native lifecycle executed supporting content")
			}
		})
	}

	t.Run("unrelated receipt keeps its retained source across refresh", func(t *testing.T) {
		environment := testprocess.Env(t)
		env := adoptionEnvironmentMap(environment)
		directory := t.TempDir()
		writeTestGitWorktree(t, directory)
		acquireSkillAccessCatalog(t, env, directory, first, firstID, "init")
		issue897ApproveCLI(t, binary, directory, environment, "activate", "independent", "--surface", "claude")
		retained := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", firstID)
		issue897RetainUnsupportedLayout(t, retained)
		retainedBefore := snapshotTree(t, retained)
		acquireSkillAccessCatalog(t, env, directory, next, nextID, "catalog", "refresh")
		out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, "status", "--json")
		if err != nil {
			t.Fatalf("old unrelated receipt blocked corrected status: %v %s %s", err, out, stderr)
		}
		var report capabilitypack.JSONStatusReport
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range report.Entries {
			found = found || entry.Pack == "independent" && entry.Readiness.Configured == capabilitypack.ReadinessTrue
		}
		if !found {
			t.Fatal("corrected status omitted old unrelated receipt")
		}
		issue897ApproveCLI(t, binary, directory, environment, "deactivate", "independent", "--surface", "claude")
		if snapshotTree(t, retained) != retainedBefore {
			t.Fatal("refresh or removal changed immutable retained source")
		}
	})

	t.Run("qualification rejects unsupported layout before mutation", func(t *testing.T) {
		editCrossPackManifest(t, project, "external-assets", func(manifest map[string]any) {
			for _, raw := range manifest["resources"].([]any) {
				resource := raw.(map[string]any)
				if resource["id"] == "workflow" {
					resource["bindings"].([]any)[0].(map[string]any)["capabilities"] = []any{}
				}
			}
		})
		environment := testprocess.Env(t)
		env := adoptionEnvironmentMap(environment)
		before := snapshotTree(t, env["HOME"])
		out, stderr, err := runSkillAccessCLI(t, binary, t.TempDir(), environment, "catalog", "validate", "--project", project, "--json")
		if err == nil || !bytes.Contains(out, []byte("outside its complete source tree")) {
			t.Fatalf("unsupported native asset closure qualified: %v %s %s", err, out, stderr)
		}
		assertStructuredOutput(t, repository, "catalog-validation.schema.json", string(out))
		output := filepath.Join(t.TempDir(), "unpublished")
		out, stderr, err = runSkillAccessCLI(t, binary, t.TempDir(), environment, "catalog", "build", "--project", project, "--source-repository", "yersonargotev/packy-catalog", "--source-commit", strings.Repeat("c", 40), "--out-dir", output, "--json")
		if err == nil || !strings.Contains(string(out)+stderr, "outside its complete source tree") {
			t.Fatalf("unsupported native closure reached publication build: %v %s %s", err, out, stderr)
		}
		assertStructuredOutput(t, repository, "catalog-build.schema.json", string(out))
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("unsupported layout emitted snapshot assets")
		}
		if snapshotTree(t, env["HOME"]) != before {
			t.Fatal("qualification failure mutated workstation state")
		}

		// Model an internally consistent snapshot retained by the previous engine,
		// which admitted this layout. These are synthetic disposable fixture bytes.
		for _, operation := range []string{"activate", "install"} {
			t.Run(operation+" retained unsupported snapshot", func(t *testing.T) {
				environment := testprocess.Env(t)
				env := adoptionEnvironmentMap(environment)
				directory := t.TempDir()
				writeTestGitWorktree(t, directory)
				acquireSkillAccessCatalog(t, env, directory, next, nextID, "init")
				retained := filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", nextID)
				issue897RetainUnsupportedLayout(t, retained)
				homeBefore, projectBefore := snapshotTree(t, env["HOME"]), snapshotTree(t, directory)
				out, stderr, err := runSkillAccessCLI(t, binary, directory, environment, operation, "external-assets", "--surface", "claude", "--resource", "skill:workflow")
				if err == nil || !strings.Contains(string(out)+stderr, "outside its complete source tree") {
					t.Fatalf("unsupported retained layout: %v %s %s", err, out, stderr)
				}
				if snapshotTree(t, env["HOME"]) != homeBefore || snapshotTree(t, directory) != projectBefore {
					t.Fatal("unsupported retained layout mutated installation state")
				}
			})
		}
	})
}

func issue897RetainUnsupportedLayout(t *testing.T, retained string) {
	t.Helper()
	editCrossPackManifest(t, retained, "external-assets", func(manifest map[string]any) {
		for _, raw := range manifest["resources"].([]any) {
			resource := raw.(map[string]any)
			if resource["id"] == "workflow" {
				resource["bindings"].([]any)[0].(map[string]any)["capabilities"] = []any{}
			}
		}
	})
	indexPath := filepath.Join(retained, "catalog-index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index cataloglayout.CatalogSnapshotIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	for i := range index.Packs {
		pack := &index.Packs[i]
		if pack.ID != "external-assets" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(retained, "packs", pack.ID, "pack.json"))
		if err != nil {
			t.Fatal(err)
		}
		pack.ManifestSHA256 = catalogFixtureDigest(data)
		var closure strings.Builder
		for j := range pack.Files {
			file := &pack.Files[j]
			if file.Path == "pack.json" {
				file.SHA256 = pack.ManifestSHA256
			}
			closure.WriteString(file.Path + "\x00" + file.Mode + "\x00" + file.SHA256 + "\n")
		}
		pack.ClosureSHA256 = catalogFixtureDigest([]byte(closure.String()))
	}
	data, err = json.Marshal(index.Packs)
	if err != nil {
		t.Fatal(err)
	}
	index.CatalogSHA256 = catalogFixtureDigest(data)
	data, err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func issue897CatalogProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, fixture := range []testsupport.Fixture{testsupport.CapabilityRich("external-assets").WithSurfaces(testsupport.SurfaceClaude), testsupport.PortableAllSurfaces("independent").WithSurfaces(testsupport.SurfaceClaude)} {
		if err := fixture.WriteCatalog(root); err != nil {
			t.Fatal(err)
		}
		editCrossPackManifest(t, root, fixture.ID(), func(manifest map[string]any) {
			manifest["origins"] = []any{}
			for _, raw := range manifest["resources"].([]any) {
				delete(raw.(map[string]any), "origin")
			}
		})
	}
	workflow := filepath.Join(root, "packs/external-assets/skills/external-assets-workflow")
	writePluginFixtureFile(t, filepath.Join(workflow, "agents/openai.yaml"), "interface:\n  display_name: Reviewed workflow\n", 0644)
	writePluginFixtureFile(t, filepath.Join(workflow, "scripts/inert.sh"), "#!/bin/sh\ntouch \"$HOME/executed\"\n", 0755)
	return root
}

func issue897ApproveCLI(t *testing.T, binary, directory string, environment []string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, environment
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	command.Stdin = slave
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := master.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, output.String())
	}
}
