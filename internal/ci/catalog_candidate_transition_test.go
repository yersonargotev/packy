package ci_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func testCatalogCandidateTransitionsExecutable(t *testing.T, executable string) {
	hostBin := t.TempDir()
	for _, name := range []string{"codex", "claude", "engram", "brew"} {
		script := "#!/bin/sh\nprintf executed > '" + filepath.Join(hostBin, "executed") + "'\nexit 99\n"
		if err := os.WriteFile(filepath.Join(hostBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	environment := testprocess.Env(t, "PATH="+hostBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	directory := t.TempDir()
	baseline, baselineSHA := buildTransitionCandidate(t, executable, environment, directory, "1.0.0", strings.Repeat("a", 40))
	candidate, candidateSHA := buildTransitionCandidate(t, executable, environment, directory, "2.0.0", strings.Repeat("b", 40))
	conflicting, _ := buildTransitionCandidate(t, executable, environment, directory, "3.0.0", strings.Repeat("a", 40))
	ambientRoots := []string{directory, hostBin}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if key == "HOME" || strings.HasPrefix(key, "XDG_") {
			ambientRoots = append(ambientRoots, value)
		}
	}
	ambientBefore := catalogTreeState(t, ambientRoots)
	for _, scope := range []string{"project", "global"} {
		t.Run(scope, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			current := baseline
			args := func(operation ...string) []string {
				return append([]string{"catalog", "candidate", "--snapshot", current, "--workspace", workspace, "--"}, operation...)
			}
			run := func(operation ...string) ([]byte, []byte, error) {
				return runCatalogCLI(t, environment, directory, executable, args(operation...)...)
			}
			apply := func(operation ...string) {
				t.Helper()
				out, err := runCandidatePTY(t, environment, directory, executable, "y\ny\ny", args(operation...)...)
				if err != nil {
					t.Fatalf("%v: %v\n%s", operation, err, out)
				}
			}
			rejectUnchanged := func(operation ...string) {
				t.Helper()
				before := catalogTreeState(t, []string{workspace})
				out, diagnostic, err := run(operation...)
				if err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
					t.Fatalf("rejected operation changed state: %v %v %s %s", operation, err, out, diagnostic)
				}
			}
			out, diagnostic, err := run("list", "--json")
			if err != nil || !bytes.Contains(diagnostic, []byte("not an official Catalog Publication")) {
				t.Fatalf("prepare: %v %s %s", err, out, diagnostic)
			}
			install, remove := "install", "uninstall"
			projectionRoot := filepath.Join(workspace, "project")
			if scope == "global" {
				install, remove, projectionRoot = "activate", "deactivate", filepath.Join(workspace, "home")
			}
			codex := filepath.Join(projectionRoot, ".agents", "skills")
			claude := filepath.Join(projectionRoot, ".claude", "skills")
			unrelated := filepath.Join(projectionRoot, "unrelated.txt")
			if err := os.WriteFile(unrelated, []byte("preserve unrelated\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			for _, surface := range []string{"codex", "claude"} {
				operation := []string{install, "transition", "--surface", surface, "--resource", "skill:guide"}
				rejectUnchanged(append(append([]string{}, operation...), "--json")...)
				before := catalogTreeState(t, []string{workspace})
				if out, err := runCandidatePTY(t, environment, directory, executable, "n", args(operation...)...); err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
					t.Fatalf("declined install changed state: %v %s", err, out)
				}
				apply(operation...)
			}
			if _, err := os.Stat(filepath.Join(codex, "helper", "SKILL.md")); err != nil {
				t.Fatalf("Codex effective dependency: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(claude, "helper")); !os.IsNotExist(err) {
				t.Fatalf("Claude inherited Codex-only dependency: %v", err)
			}
			receiptPath := filepath.Join(workspace, "project", "packy.lock.json")
			if scope == "global" {
				receiptPath = filepath.Join(workspace, "home", ".packy", "packs.json")
			}
			before := catalogTreeState(t, []string{workspace})
			current = candidate
			out, diagnostic, err = run("transition", "--expect-archive-sha256", strings.Repeat("0", 64), "--json")
			validateCatalogCLISchema(t, "catalog-candidate-transition", out)
			if err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
				t.Fatalf("unexpected transition identity changed state: %v %s %s", err, out, diagnostic)
			}
			current = conflicting
			rejectUnchanged("transition", "--expect-archive-sha256", baselineSHA, "--json")
			current = candidate
			installedBefore := catalogTreeState(t, []string{filepath.Join(workspace, "home"), filepath.Join(workspace, "config"), filepath.Join(workspace, "project")})
			out, diagnostic, err = run("transition", "--expect-archive-sha256", baselineSHA, "--json")
			validateCatalogCLISchema(t, "catalog-candidate-transition", out)
			if err != nil || !bytes.Contains(out, []byte(candidateSHA)) || !reflect.DeepEqual(installedBefore, catalogTreeState(t, []string{filepath.Join(workspace, "home"), filepath.Join(workspace, "config"), filepath.Join(workspace, "project")})) {
				t.Fatalf("transition changed installed baseline: %v %s %s", err, out, diagnostic)
			}
			rejectUnchanged("transition", "--expect-archive-sha256", baselineSHA, "--json")
			for _, step := range []struct{ surface, other string }{{"codex", claude}, {"claude", codex}} {
				otherBefore := candidateSkillState(t, step.other)
				otherSurface := "claude"
				if step.surface == "claude" {
					otherSurface = "codex"
				}
				otherReceipt := candidateReceipt(t, receiptPath, otherSurface)
				operation := []string{"update", "transition", "--surface", step.surface}
				if scope == "project" {
					operation = append(operation, "--project")
				}
				rejectUnchanged(append(append([]string{}, operation...), "--json")...)
				apply(operation...)
				targetRoot := codex
				if step.surface == "claude" {
					targetRoot = claude
				}
				if got := readIssue798File(t, filepath.Join(targetRoot, "guide", "references", "detail.md")); string(got) != step.surface+" 2.0.0\n" {
					t.Fatalf("%s update did not materialize candidate: %s", step.surface, got)
				}
				var receipt struct {
					Pack struct {
						Version  string `json:"version"`
						Snapshot string `json:"catalog_snapshot"`
					} `json:"pack"`
				}
				if err := json.Unmarshal(candidateReceipt(t, receiptPath, step.surface), &receipt); err != nil {
					t.Fatal(err)
				}
				if receipt.Pack.Version != "2.0.0" || receipt.Pack.Snapshot != strings.Repeat("b", 40) {
					t.Fatalf("targeted update receipt: %#v", receipt)
				}
				if !bytes.Equal(otherReceipt, candidateReceipt(t, receiptPath, otherSurface)) {
					t.Fatal("targeted update changed other surface receipt")
				}
				if !reflect.DeepEqual(otherBefore, candidateSkillState(t, step.other)) {
					t.Fatalf("%s update changed other surface bytes/modes", step.surface)
				}
			}
			rejectUnchanged(install, "transition", "--surface", "opencode", "--resource", "skill:guide", "--json")
			// Both owned scopes retain standard helper drift rejection. A global link
			// exposes retained-source drift; project copies expose projection drift.
			helper := filepath.Join(codex, "helper", "references", "detail.md")
			original := readIssue798File(t, helper)
			if err := os.WriteFile(helper, []byte("helper drift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			rejectUnchanged(remove, "transition", "--surface", "codex", "--json")
			if err := os.WriteFile(helper, original, 0o644); err != nil {
				t.Fatal(err)
			}
			// JSON never grants removal consent.
			operation := []string{remove, "transition", "--surface", "codex", "--json"}
			rejectUnchanged(operation...)
			retainedClaude := candidateSkillState(t, claude)
			retainedReceipt := candidateReceipt(t, receiptPath, "claude")
			apply(remove, "transition", "--surface", "codex")
			if !bytes.Equal(retainedReceipt, candidateReceipt(t, receiptPath, "claude")) {
				t.Fatal("Codex removal changed Claude receipt")
			}
			if !reflect.DeepEqual(retainedClaude, candidateSkillState(t, claude)) {
				t.Fatal("Codex removal changed Claude bytes/modes")
			}
			for _, name := range []string{"guide", "helper"} {
				if _, err := os.Lstat(filepath.Join(codex, name)); !os.IsNotExist(err) {
					t.Fatalf("retired Codex %s remains: %v", name, err)
				}
			}
			apply(remove, "transition", "--surface", "claude")
			if _, err := os.Lstat(filepath.Join(claude, "guide")); !os.IsNotExist(err) {
				t.Fatalf("retired Claude guide remains: %v", err)
			}
			apply(install, "transition", "--surface", "claude", "--resource", "skill:guide")
			if _, err := os.Lstat(filepath.Join(claude, "helper")); !os.IsNotExist(err) {
				t.Fatalf("Claude reinstallation inherited Codex-only dependency: %v", err)
			}
			apply(remove, "transition", "--surface", "claude")
			if _, err := os.Lstat(filepath.Join(claude, "guide")); !os.IsNotExist(err) {
				t.Fatalf("retired Claude guide remains after reinstallation: %v", err)
			}
			if got := readIssue798File(t, unrelated); string(got) != "preserve unrelated\n" {
				t.Fatalf("unrelated bytes changed: %s", got)
			}
			info, err := os.Stat(unrelated)
			if err != nil || info.Mode().Perm() != 0o640 {
				t.Fatalf("unrelated mode changed: %v %v", info, err)
			}
			for _, name := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md"} {
				if _, err := os.Stat(filepath.Join(workspace, "project", name)); !os.IsNotExist(err) {
					t.Fatalf("retired project contract remains: %s %v", name, err)
				}
			}
			rejectUnchanged("activate", "transition", "--surface", "codex", "--project", "--json")
			rejectUnchanged("deactivate", "transition", "--surface", "codex", "--project", "--json")
			rejectUnchanged("catalog", "refresh")
		})
	}
	if !reflect.DeepEqual(ambientBefore, catalogTreeState(t, ambientRoots)) {
		t.Fatal("candidate transitions changed ambient HOME/XDG/project")
	}
	testCandidateTransitionRejections(t, executable, environment, directory, baseline, candidate, baselineSHA)
}

func candidateReceipt(t *testing.T, path, surface string) json.RawMessage {
	t.Helper()
	var document struct {
		Receipts []json.RawMessage `json:"receipts"`
	}
	if err := json.Unmarshal(readIssue798File(t, path), &document); err != nil {
		t.Fatal(err)
	}
	for _, value := range document.Receipts {
		var receipt struct {
			Surface string `json:"surface"`
		}
		if err := json.Unmarshal(value, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Surface == surface {
			return value
		}
	}
	t.Fatalf("missing %s receipt", surface)
	return nil
}

func candidateSkillState(t *testing.T, root string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range catalogTreeState(t, []string{target}) {
			state[key] = value
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			state[path] = link
		}
	}
	return state
}

func buildTransitionCandidate(t *testing.T, executable string, environment []string, directory, version, commit string) (string, string) {
	t.Helper()
	project := t.TempDir()
	fixture := testsupport.SkillVariantDependencies("transition").WithVersion(version)
	if err := fixture.WriteCatalog(project); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(project, "packs", "transition", "pack.json")
	var manifest map[string]any
	if err := json.Unmarshal(readIssue798File(t, manifestPath), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["origins"] = []any{}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		delete(resource, "origin")
		for _, value := range variantArray(resource) {
			delete(value.(map[string]any), "origin")
		}
		if resource["kind"] == "skill" && resource["id"] == "guide" {
			// Claude has its own divergent reviewed body, with no helper dependency.
			source := filepath.Join(project, "packs", "transition", "skills", "claude")
			common := filepath.Join(project, "packs", "transition", "skills", "common")
			err := filepath.WalkDir(common, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(common, path)
				if err != nil {
					return err
				}
				target := filepath.Join(source, relative)
				if entry.IsDir() {
					return os.MkdirAll(target, 0o755)
				}
				data := bytes.ReplaceAll(readIssue798File(t, path), []byte("reviewed common"), []byte("reviewed claude"))
				return os.WriteFile(target, data, 0o644)
			})
			if err != nil {
				t.Fatal(err)
			}
			resource["variants"] = append(variantArray(resource), map[string]any{"surface": "claude", "source": "skills/claude", "requires": []string{}})
			// Variant arrays are canonical surface order.
			variants := resource["variants"].([]any)
			resource["variants"] = []any{variants[1], variants[0]}
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packs", "transition", "skills", "codex", "references", "detail.md"), []byte("codex "+version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packs", "transition", "skills", "claude", "references", "detail.md"), []byte("claude "+version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(t.TempDir(), "assets")
	out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, "catalog", "build", "--project", project, "--source-repository", "yersonargotev/packy-catalog", "--source-commit", commit, "--out-dir", assets, "--json")
	if err != nil {
		t.Fatalf("build transition candidate: %v %s %s", err, out, diagnostic)
	}
	var report struct {
		ArchiveSHA256 string `json:"archive_sha256"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	return assets, report.ArchiveSHA256
}

func variantArray(resource map[string]any) []any {
	values, _ := resource["variants"].([]any)
	return values
}

func testCandidateTransitionRejections(t *testing.T, executable string, environment []string, directory, baseline, candidate, baselineSHA string) {
	for _, scenario := range []string{"unowned", "missing HEAD", "empty HEAD", "missing objects", "symlink escape", "redirected global root", "retained source drift", "retained index drift", "occupied transition destination", "incompatible snapshot", "invalid snapshot", "stale project Apply", "stale global Apply", "stale project index", "stale global index", "stale project ownership", "stale global ownership", "output failure"} {
		t.Run(scenario, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			prefix := func(assets string) []string {
				return []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}
			}
			if out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, append(prefix(baseline), "install", "transition", "--surface", "codex", "--dry-run", "--json")...); err != nil {
				t.Fatalf("prepare: %v %s %s", err, out, diagnostic)
			}
			targetAssets := candidate
			switch scenario {
			case "unowned":
				if err := os.Remove(filepath.Join(workspace, "local-candidate.json")); err != nil {
					t.Fatal(err)
				}
			case "missing HEAD":
				if err := os.Remove(filepath.Join(workspace, "project", ".git", "HEAD")); err != nil {
					t.Fatal(err)
				}
			case "empty HEAD":
				if err := os.WriteFile(filepath.Join(workspace, "project", ".git", "HEAD"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			case "missing objects":
				if err := os.RemoveAll(filepath.Join(workspace, "project", ".git", "objects")); err != nil {
					t.Fatal(err)
				}
			case "symlink escape":
				if err := os.Symlink(directory, filepath.Join(workspace, "home", "escape")); err != nil {
					t.Fatal(err)
				}
			case "redirected global root":
				if err := os.Symlink(filepath.Join(workspace, "snapshots", strings.Repeat("a", 40)), filepath.Join(workspace, "home", ".agents")); err != nil {
					t.Fatal(err)
				}
			case "retained source drift":
				if err := os.WriteFile(filepath.Join(workspace, "snapshots", strings.Repeat("a", 40), "packs", "transition", "skills", "common", "SKILL.md"), []byte("drift\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "retained index drift":
				path := filepath.Join(workspace, "snapshots", strings.Repeat("a", 40), "catalog-index.json")
				data := append(readIssue798File(t, path), '\n')
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			case "occupied transition destination":
				if err := os.Mkdir(filepath.Join(workspace, "snapshots", strings.Repeat("b", 40)), 0o700); err != nil {
					t.Fatal(err)
				}
			case "incompatible snapshot", "invalid snapshot":
				files := catalogBuildArchive(t, readIssue798File(t, filepath.Join(candidate, "catalog-snapshot.tar.gz")))
				if scenario == "incompatible snapshot" {
					files["catalog-index.json"] = bytes.Replace(files["catalog-index.json"], []byte(`"schema_version": 3`), []byte(`"schema_version": 999`), 1)
				} else {
					files["packs/transition/pack.json"] = []byte("{")
				}
				targetAssets = writeCandidateArchive(t, files)
			case "output failure":
				before := catalogTreeState(t, []string{workspace})
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				output, err := os.Open(os.DevNull)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				command := exec.CommandContext(ctx, executable, append(prefix(candidate), "transition", "--expect-archive-sha256", baselineSHA, "--json")...)
				command.Env, command.Dir, command.Stdout = environment, directory, output
				var diagnostic bytes.Buffer
				command.Stderr = &diagnostic
				if err := command.Run(); err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
					t.Fatalf("failed transition output changed state: %v %s", err, diagnostic.String())
				}
				return
			case "stale project Apply", "stale global Apply", "stale project index", "stale global index", "stale project ownership", "stale global ownership":
				testCandidateStaleApply(t, executable, environment, directory, workspace, prefix(baseline), scenario)
				return
			}
			before := catalogTreeState(t, []string{workspace, directory})
			out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, append(prefix(targetAssets), "transition", "--expect-archive-sha256", baselineSHA, "--json")...)
			validateCatalogCLISchema(t, "catalog-candidate-transition", out)
			if err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace, directory})) {
				t.Fatalf("failed transition changed prior state: %v %s %s", err, out, diagnostic)
			}
		})
	}
}

// Consent is withheld until the real child has emitted its exact preview. An
// workspace or retained-source edit then makes Apply fail before mutation.
func testCandidateStaleApply(t *testing.T, executable string, environment []string, directory, workspace string, prefix []string, scenario string) {
	t.Helper()
	global, indexDrift := strings.Contains(scenario, "global"), strings.HasSuffix(scenario, "index")
	ownershipDrift := strings.HasSuffix(scenario, "ownership")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	operation := "install"
	if global {
		operation = "activate"
	}
	command := exec.CommandContext(ctx, executable, append(prefix, operation, "transition", "--surface", "codex", "--resource", "skill:guide")...)
	command.Env, command.Dir = environment, directory
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	command.Stdin = slave
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	one := make([]byte, 1)
	for !strings.Contains(output.String(), "[y/N]") {
		n, readErr := stdout.Read(one)
		output.Write(one[:n])
		if readErr != nil {
			command.Wait()
			t.Fatalf("no consent prompt: %v %s %s", readErr, output.String(), diagnostic.String())
		}
	}
	unrelated := filepath.Join(workspace, "project", "packy.json")
	if global && !indexDrift && !ownershipDrift {
		unrelated = filepath.Join(workspace, "home", ".agents", "skills", "foreign", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
			cancel()
			command.Wait()
			t.Fatal(err)
		}
	}
	edited := []byte("---\nname: guide\ndescription: Foreign conflict.\n---\nForeign\n")
	if indexDrift {
		unrelated = filepath.Join(workspace, "snapshots", strings.Repeat("a", 40), "catalog-index.json")
		edited = append(readIssue798File(t, unrelated), '\n')
	}
	var editErr error
	if ownershipDrift {
		editErr = os.Remove(filepath.Join(workspace, "local-candidate.json"))
	} else {
		editErr = os.WriteFile(unrelated, edited, 0o644)
	}
	if editErr != nil {
		cancel()
		command.Wait()
		t.Fatal(editErr)
	}
	before := catalogTreeState(t, []string{workspace})
	if _, err := master.Write([]byte("y\n")); err != nil {
		cancel()
		command.Wait()
		t.Fatal(err)
	}
	if _, err := io.Copy(&output, stdout); err != nil {
		cancel()
		command.Wait()
		t.Fatal(err)
	}
	err = command.Wait()
	if err == nil || !reflect.DeepEqual(before, catalogTreeState(t, []string{workspace})) {
		t.Fatalf("stale Apply changed edited state: %v %s %s", err, output.String(), diagnostic.String())
	}
}
