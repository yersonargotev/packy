package ci_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

func testCatalogCandidateExecutable(t *testing.T, executable string) {
	environment := testprocess.Env(t)
	directory := t.TempDir()
	project := t.TempDir()
	for _, fixture := range []testsupport.Fixture{testsupport.PortableAllSurfaces("alpha"), testsupport.SkillVariants("beta")} {
		id := fixture.ID()
		if err := fixture.WriteCatalog(project); err != nil {
			t.Fatal(err)
		}
		// Locally authored generic content needs no upstream transport.
		path := filepath.Join(project, "packs", id, "pack.json")
		var manifest map[string]any
		if err := json.Unmarshal(readIssue798File(t, path), &manifest); err != nil {
			t.Fatal(err)
		}
		manifest["origins"] = []any{}
		for _, resource := range manifest["resources"].([]any) {
			delete(resource.(map[string]any), "origin")
		}
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if id == "beta" {
			inertScript := filepath.Join(project, "packs", id, "skills", "common", "scripts", "run.sh")
			if err := os.WriteFile(inertScript, []byte("#!/bin/sh\ntouch '"+filepath.Join(project, "candidate-executed")+"'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	assets := filepath.Join(t.TempDir(), "dist")
	stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "build", "--project", project, "--source-repository", "yersonargotev/packy-catalog", "--source-commit", strings.Repeat("a", 40), "--out-dir", assets, "--json")
	if err != nil {
		t.Fatalf("build local candidate: %v %s %s", err, stdout, stderr)
	}
	ambientRoots := []string{directory, project}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if key == "HOME" || key == "XDG_CONFIG_HOME" {
			ambientRoots = append(ambientRoots, value)
		}
	}
	ambientBefore := catalogTreeState(t, ambientRoots)
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			prefix := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}
			run := func(args ...string) ([]byte, []byte, error) {
				return runCatalogCLI(t, environment, directory, executable, append(append([]string{}, prefix...), args...)...)
			}
			out, diagnostic, err := run("list", "--json")
			if err != nil || !json.Valid(out) || !bytes.Contains(out, []byte(`"id":"beta"`)) || !bytes.Contains(diagnostic, []byte("not an official Catalog Publication")) {
				t.Fatalf("candidate provenance/list: %v %s %s", err, out, diagnostic)
			}
			ownedProject := filepath.Join(workspace, "project")
			for _, name := range []string{"AGENTS.md", "CLAUDE.md", "unrelated.txt"} {
				if err := os.WriteFile(filepath.Join(ownedProject, name), []byte("Personal project guidance\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			beforePreview := catalogTreeState(t, []string{workspace})
			install := []string{"install", "alpha", "--surface", surface, "--json"}
			out, diagnostic, err = run(append(append([]string{}, install...), "--dry-run")...)
			if err != nil || !json.Valid(out) || !bytes.Contains(out, []byte(`"catalog_snapshot":"`+strings.Repeat("a", 40)+`"`)) {
				t.Fatalf("real candidate preview: %v %s %s", err, out, diagnostic)
			}
			if after := catalogTreeState(t, []string{workspace}); !reflect.DeepEqual(beforePreview, after) {
				t.Fatal("preview mutated existing workspace")
			}
			out, diagnostic, err = run(install...)
			if err == nil || !bytes.Contains(diagnostic, []byte("interactive")) || !reflect.DeepEqual(beforePreview, catalogTreeState(t, []string{workspace})) {
				t.Fatalf("JSON counted as approval: %v %s %s", err, out, diagnostic)
			}
			// Preserve beta's complete composite document while retiring alpha.
			betaInstall := append(append([]string{}, prefix...), "install", "beta", "--surface", surface)
			if out, err := runCandidatePTY(t, environment, directory, executable, "y", betaInstall...); err != nil {
				t.Fatalf("PTY beta install: %v %s", err, out)
			}
			retainedDocuments := map[string][]byte{}
			for _, name := range []string{"AGENTS.md", "CLAUDE.md", "PACKY-NOTICES.md", "unrelated.txt"} {
				path := filepath.Join(ownedProject, name)
				if data, err := os.ReadFile(path); err == nil {
					retainedDocuments[path] = data
				}
			}
			alphaInstall := append(append([]string{}, prefix...), install...)
			if out, err := runCandidatePTY(t, environment, directory, executable, "n", alphaInstall...); err == nil {
				t.Fatalf("declined install succeeded: %s", out)
			}
			if out, err := runCandidatePTY(t, environment, directory, executable, "y", alphaInstall...); err != nil {
				t.Fatalf("PTY alpha install: %v %s", err, out)
			}
			out, diagnostic, err = run("verify", "--json")
			if err != nil || !bytes.Contains(out, []byte(`"result":"passed"`)) {
				t.Fatalf("verify installed candidate: %v %s %s", err, out, diagnostic)
			}
			beforeFailure := catalogTreeState(t, []string{workspace})
			changed := catalogBuildArchive(t, readIssue798File(t, filepath.Join(assets, "catalog-snapshot.tar.gz")))
			changed["catalog-index.json"] = bytes.Replace(changed["catalog-index.json"], []byte(strings.Repeat("a", 40)), []byte(strings.Repeat("b", 40)), 1)
			differentCandidate := writeCandidateArchive(t, changed)
			out, diagnostic, err = runCatalogCLI(t, environment, directory, executable, "catalog", "candidate", "--snapshot", differentCandidate, "--workspace", workspace, "--", "list", "--json")
			if err == nil || !reflect.DeepEqual(beforeFailure, catalogTreeState(t, []string{workspace})) {
				t.Fatalf("different candidate replaced prior usable state: %v %s %s", err, out, diagnostic)
			}
			for _, args := range [][]string{{"activate", "alpha", "--surface", surface, "--project"}, {"catalog", "refresh"}, {"install", "missing", "--surface", surface, "--json"}} {
				out, diagnostic, err = run(args...)
				if err == nil || !reflect.DeepEqual(beforeFailure, catalogTreeState(t, []string{workspace})) {
					t.Fatalf("failed operation changed workspace: %v %s %s", err, out, diagnostic)
				}
			}
			out, diagnostic, err = run("uninstall", "alpha", "--surface", surface, "--json")
			if err == nil || !reflect.DeepEqual(beforeFailure, catalogTreeState(t, []string{workspace})) {
				t.Fatalf("JSON uninstall counted as consent: %v %s %s", err, out, diagnostic)
			}
			uninstall := append(append([]string{}, prefix...), "uninstall", "alpha", "--surface", surface, "--json")
			if out, err := runCandidatePTY(t, environment, directory, executable, "y", uninstall...); err != nil {
				t.Fatalf("PTY alpha uninstall: %v %s", err, out)
			}
			for path, want := range retainedDocuments {
				// Marked-file lifecycle retains its existing separator newlines.
				if got := readIssue798File(t, path); strings.TrimRight(string(got), "\n") != strings.TrimRight(string(want), "\n") {
					t.Fatalf("retirement did not restore complete retained document %s: %s", path, got)
				}
			}
			out, diagnostic, err = run("verify", "--json")
			if err != nil || !bytes.Contains(out, []byte(`"result":"passed"`)) || bytes.Contains(out, []byte(`"id":"alpha"`)) {
				t.Fatalf("verify retained candidate: %v %s %s", err, out, diagnostic)
			}
			if out, err := runCandidatePTY(t, environment, directory, executable, "y", append(append([]string{}, prefix...), "uninstall", "beta", "--surface", surface)...); err != nil {
				t.Fatalf("PTY beta uninstall: %v %s", err, out)
			}
			for _, name := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md"} {
				if _, err := os.Stat(filepath.Join(ownedProject, name)); !os.IsNotExist(err) {
					t.Fatalf("retired project contract remains: %s %v", name, err)
				}
			}
			for _, name := range []string{"AGENTS.md", "CLAUDE.md", "unrelated.txt"} {
				if got := string(readIssue798File(t, filepath.Join(ownedProject, name))); strings.TrimRight(got, "\n") != "Personal project guidance" {
					t.Fatalf("final unrelated document %s = %q", name, got)
				}
			}
			skillRoot := ".agents"
			if surface == "claude" {
				skillRoot = ".claude"
			}
			if surface == "opencode" {
				skillRoot = ".opencode"
			}
			retiredSkill := filepath.Join(ownedProject, skillRoot, "skills", "guide")
			if _, err := os.Stat(retiredSkill); !os.IsNotExist(err) {
				t.Fatalf("retired skill remains: %v", err)
			}
			if err := os.MkdirAll(retiredSkill, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(retiredSkill, "SKILL.md"), []byte("foreign skill\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			beforeConflict := catalogTreeState(t, []string{workspace})
			out, diagnostic, err = run("install", "beta", "--surface", surface, "--dry-run", "--json")
			if err == nil || !bytes.Contains(out, []byte("foreign_target")) || !reflect.DeepEqual(beforeConflict, catalogTreeState(t, []string{workspace})) {
				t.Fatalf("ownership conflict changed unrelated files: %v %s %s", err, out, diagnostic)
			}
		})
	}
	t.Run("damaged Git boundary preserves enclosing project", func(t *testing.T) {
		parent := t.TempDir()
		if out, diagnostic, err := runCatalogCLI(t, environment, directory, "git", "init", "-q", parent); err != nil {
			t.Fatalf("initialize enclosing disposable project: %v %s %s", err, out, diagnostic)
		}
		if err := os.WriteFile(filepath.Join(parent, "personal.txt"), []byte("preserve enclosing project\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		workspace := filepath.Join(parent, "workspace")
		prefix := []string{"catalog", "candidate", "--snapshot", assets, "--workspace", workspace, "--"}
		if out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, append(append([]string{}, prefix...), "list", "--json")...); err != nil {
			t.Fatalf("prepare nested workspace: %v %s %s", err, out, diagnostic)
		}
		if err := os.Remove(filepath.Join(workspace, "project", ".git", "HEAD")); err != nil {
			t.Fatal(err)
		}
		before := catalogTreeState(t, []string{parent})
		out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, append(append([]string{}, prefix...), "install", "alpha", "--surface", "codex", "--json")...)
		if err == nil || !bytes.Contains(diagnostic, []byte("candidate workspace Git boundary")) || !reflect.DeepEqual(before, catalogTreeState(t, []string{parent})) {
			t.Fatalf("damaged inner Git boundary escaped isolation: %v %s %s", err, out, diagnostic)
		}
	})
	t.Run("candidate rejection", func(t *testing.T) {
		files := catalogBuildArchive(t, readIssue798File(t, filepath.Join(assets, "catalog-snapshot.tar.gz")))
		for _, tc := range []struct {
			name string
			edit func(map[string][]byte)
		}{
			{"incompatible index", func(files map[string][]byte) {
				files["catalog-index.json"] = bytes.Replace(files["catalog-index.json"], []byte(`"schema_version": 3`), []byte(`"schema_version": 999`), 1)
			}},
			{"invalid unselected pack", func(files map[string][]byte) { files["packs/beta/pack.json"] = []byte("{") }},
			{"unexpected file", func(files map[string][]byte) { files["packs/alpha/orphan"] = []byte("orphan") }},
			{"traversal", func(files map[string][]byte) { files["../escape"] = []byte("unsafe") }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				changed := map[string][]byte{}
				for path, data := range files {
					changed[path] = append([]byte{}, data...)
				}
				tc.edit(changed)
				invalid := writeCandidateArchive(t, changed)
				workspace := filepath.Join(t.TempDir(), "workspace")
				out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, "catalog", "candidate", "--snapshot", invalid, "--workspace", workspace, "--", "show", "alpha", "--json")
				if err == nil {
					t.Fatalf("invalid candidate accepted: %s %s", out, diagnostic)
				}
				if _, err := os.Stat(workspace); !os.IsNotExist(err) {
					t.Fatalf("failed candidate left workspace: %v", err)
				}
			})
		}
	})
	if after := catalogTreeState(t, ambientRoots); !reflect.DeepEqual(ambientBefore, after) {
		t.Fatalf("candidate operations changed ambient home/project: before=%v after=%v", ambientBefore, after)
	}
	// Local candidate preparation never enables ordinary official consumption.
	out, diagnostic, err := runCatalogCLI(t, environment, directory, executable, "list", "--json")
	if err == nil || !bytes.Contains(diagnostic, []byte("packy init")) {
		t.Fatalf("local candidate selected official source: %v %s %s", err, out, diagnostic)
	}
}

func runCandidatePTY(t *testing.T, environment []string, directory, executable, answer string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Env, command.Dir = environment, directory
	// Only stdin needs terminal identity. Pipes keep JSON and diagnostics separate.
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
		return nil, err
	}
	_, writeErr := master.Write([]byte(answer + "\n"))
	if writeErr != nil {
		cancel()
	}
	err = command.Wait()
	if writeErr != nil {
		t.Fatalf("write PTY approval: %v", writeErr)
	}
	return output.Bytes(), err
}

func writeCandidateArchive(t *testing.T, files map[string][]byte) string {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	assets := t.TempDir()
	digest := sha256.Sum256(archive.Bytes())
	for name, data := range map[string][]byte{"catalog-snapshot.tar.gz": archive.Bytes(), "SHA256SUMS": []byte(fmt.Sprintf("%s  catalog-snapshot.tar.gz\n", hex.EncodeToString(digest[:])))} {
		if err := os.WriteFile(filepath.Join(assets, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return assets
}
