package ci_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestCatalogBuildExecutable(t *testing.T) {
	executable, builderCommit := buildCatalogCLI(t)
	environment := testprocess.Env(t)
	directory := t.TempDir()
	project := t.TempDir()
	writePublicationCatalogPack(t, project, "alpha", "1.0.0", "inert skill\n")
	writePublicationCatalogPack(t, project, "beta", "2.0.0", "other skill\n")

	roots := []string{project, directory}
	for _, entry := range environment {
		if key, value, ok := strings.Cut(entry, "="); ok && (key == "HOME" || key == "XDG_CONFIG_HOME" || key == "TMPDIR") {
			roots = append(roots, value)
		}
	}
	beforeBuild := catalogTreeState(t, roots)
	sourceCommit := strings.Repeat("a", 40)
	var firstArchive []byte
	for _, format := range []string{"json", "human"} {
		out := filepath.Join(t.TempDir(), "dist")
		args := []string{"catalog", "build", "--project", project, "--source-repository", "example/catalog", "--source-commit", sourceCommit, "--out-dir", out}
		if format == "json" {
			args = append(args, "--json")
		}
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, args...)
		if err != nil {
			t.Fatalf("build: %v stdout=%s stderr=%s", err, stdout, stderr)
		}
		archive := readIssue798File(t, filepath.Join(out, "catalog-snapshot.tar.gz"))
		digest := sha256.Sum256(archive)
		archiveSHA := hex.EncodeToString(digest[:])
		if got := string(readIssue798File(t, filepath.Join(out, "SHA256SUMS"))); got != archiveSHA+"  catalog-snapshot.tar.gz\n" {
			t.Fatalf("checksum = %q", got)
		}
		files := catalogBuildArchive(t, archive)
		names := map[string]bool{}
		for name := range files {
			names[name] = true
		}
		want := map[string]bool{"catalog-index.json": true, "packs/alpha/pack.json": true, "packs/alpha/skills/alpha/SKILL.md": true, "packs/beta/pack.json": true, "packs/beta/skills/beta/SKILL.md": true}
		if !reflect.DeepEqual(names, want) {
			t.Fatalf("archive entries = %v", names)
		}
		var index cataloglayout.CatalogSnapshotIndex
		if err := json.Unmarshal(files["catalog-index.json"], &index); err != nil {
			t.Fatal(err)
		}
		if index.SchemaVersion != 3 || index.Builder != "yersonargotev/packy@"+builderCommit || index.Source.Repository != "example/catalog" || index.Source.Commit != sourceCommit || len(index.Packs) != 2 || string(files["packs/alpha/skills/alpha/SKILL.md"]) != "inert skill\n" {
			t.Fatalf("index/content = %#v", index)
		}
		if firstArchive != nil && !bytes.Equal(firstArchive, archive) {
			t.Fatal("repeated build changed archive bytes")
		}
		firstArchive = archive
		if format == "json" {
			validateCatalogCLISchema(t, "catalog-build", stdout)
			var result struct {
				Result        string                             `json:"result"`
				Index         cataloglayout.CatalogSnapshotIndex `json:"index"`
				ArchiveSHA256 string                             `json:"archive_sha256"`
				ArchivePath   string                             `json:"archive_path"`
				ChecksumPath  string                             `json:"checksum_path"`
			}
			if err := json.Unmarshal(stdout, &result); err != nil {
				t.Fatal(err)
			}
			if result.Result != "built" || !reflect.DeepEqual(result.Index, index) || result.ArchiveSHA256 != archiveSHA || result.ArchivePath != filepath.Join(out, "catalog-snapshot.tar.gz") || result.ChecksumPath != filepath.Join(out, "SHA256SUMS") {
				t.Fatalf("report = %s", stdout)
			}
		} else if len(stdout) == 0 {
			t.Fatal("missing human result")
		}
		entries, err := os.ReadDir(out)
		if err != nil || len(entries) != 2 {
			t.Fatalf("assets = %v %v", entries, err)
		}
	}

	if after := catalogTreeState(t, roots); !reflect.DeepEqual(beforeBuild, after) {
		t.Fatalf("build mutated candidate or personal state: before=%v after=%v", beforeBuild, after)
	}
	t.Run("failures leave no partial assets", func(t *testing.T) {
		for _, tc := range []struct {
			name, code string
			flags      []string
			mutate     func(string, string)
		}{
			{name: "builder override", code: "invalid_arguments", flags: []string{"--builder", "example/other@" + strings.Repeat("b", 40)}},
			{name: "positional argument", code: "invalid_arguments", flags: []string{"unexpected"}},
			{name: "missing source", code: "build_failed", flags: []string{"--source-commit="}},
			{name: "invalid repository", code: "build_failed", flags: []string{"--source-repository=example/catalog/extra"}},
			{name: "invalid pack", code: "validation_failed", mutate: func(candidate, out string) { writePublicationCatalogPack(t, candidate, "zulu", "invalid", "bad\n") }},
			{name: "undeclared file", code: "validation_failed", mutate: func(candidate, out string) {
				if err := os.WriteFile(filepath.Join(candidate, "packs", "alpha", "orphan"), []byte("orphan"), 0o644); err != nil {
					t.Fatal(err)
				}
			}},
			{name: "symlink", code: "validation_failed", mutate: func(candidate, out string) {
				path := filepath.Join(candidate, "packs", "alpha", "skills", "alpha", "SKILL.md")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(project, "packs", "alpha", "skills", "alpha", "SKILL.md"), path); err != nil {
					t.Fatal(err)
				}
			}},
			{name: "traversal", code: "validation_failed", mutate: func(candidate, out string) {
				mutateCatalogManifest(t, candidate, func(m map[string]any) { m["resources"].([]any)[0].(map[string]any)["source"] = "../outside" })
			}},
			{name: "existing output", code: "build_failed", mutate: func(candidate, out string) {
				if err := os.Mkdir(out, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, "retained"), []byte("keep"), 0o644); err != nil {
					t.Fatal(err)
				}
			}},
			{name: "output parent is a file", code: "build_failed", mutate: func(candidate, out string) {
				parent := filepath.Dir(out)
				if err := os.Remove(parent); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(parent, []byte("keep"), 0o644); err != nil {
					t.Fatal(err)
				}
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				candidate := t.TempDir()
				writePublicationCatalogPack(t, candidate, "alpha", "1.0.0", "valid\n")
				outputRoot := t.TempDir()
				out := filepath.Join(outputRoot, "parent", "dist")
				if err := os.Mkdir(filepath.Dir(out), 0o755); err != nil {
					t.Fatal(err)
				}
				if tc.mutate != nil {
					tc.mutate(candidate, out)
				}
				before := catalogTreeState(t, []string{outputRoot})
				args := []string{"catalog", "build", "--json", "--project", candidate, "--source-repository", "example/catalog", "--source-commit", sourceCommit, "--out-dir", out}
				args = append(args, tc.flags...)
				stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, args...)
				assertCatalogBuildFailure(t, stdout, stderr, err, tc.code)
				humanArgs := append([]string{"catalog", "build"}, args[3:]...)
				humanOut, humanErr, humanRunErr := runCatalogCLI(t, environment, directory, executable, humanArgs...)
				if humanRunErr == nil || len(humanOut) != 0 || len(humanErr) == 0 {
					t.Fatalf("human failure: %v stdout=%s stderr=%s", humanRunErr, humanOut, humanErr)
				}
				if after := catalogTreeState(t, []string{outputRoot}); !reflect.DeepEqual(before, after) {
					t.Fatalf("failure changed output: before=%v after=%v", before, after)
				}
			})
		}
	})

	for _, args := range [][]string{
		{"--builder", "example/other", "--json"},
		{"--help=invalid", "--json"},
		{"--json=invalid", "--json"},
	} {
		t.Run("JSON after "+args[0], func(t *testing.T) {
			stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", "build"}, args...)...)
			assertCatalogBuildFailure(t, stdout, stderr, err, "invalid_arguments")
		})
	}
	t.Run("build help", func(t *testing.T) {
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "build", "--help")
		if err != nil || len(stdout) == 0 || len(stderr) != 0 {
			t.Fatalf("help: %v stdout=%s stderr=%s", err, stdout, stderr)
		}
	})

	t.Run("baseline version violation", func(t *testing.T) {
		baseline := t.TempDir()
		writePublicationCatalogPack(t, baseline, "alpha", "1.0.0", "original\n")
		candidate := t.TempDir()
		writePublicationCatalogPack(t, candidate, "alpha", "1.0.0", "changed\n")
		out := filepath.Join(t.TempDir(), "dist")
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "build", "--json", "--project", candidate, "--baseline", baseline, "--source-repository", "example/catalog", "--source-commit", sourceCommit, "--out-dir", out)
		assertCatalogBuildFailure(t, stdout, stderr, err, "validation_failed")
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("partial output exists: %v", err)
		}
	})
	t.Run("report output failure is nonzero", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "dist")
		readOnly, err := os.Open(filepath.Join(project, "packs", "alpha", "pack.json"))
		if err != nil {
			t.Fatal(err)
		}
		defer readOnly.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "catalog", "build", "--json", "--project", project, "--source-repository", "example/catalog", "--source-commit", sourceCommit, "--out-dir", out)
		command.Env = environment
		command.Dir = directory
		command.Stdout = readOnly
		if err := command.Run(); err == nil {
			t.Fatal("report write failure returned success")
		}
	})

}

func catalogBuildArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	files := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected entry: %#v", header)
		}
		if _, exists := files[header.Name]; exists {
			t.Fatalf("duplicate %s", header.Name)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = content
	}
	return files
}

func assertCatalogBuildFailure(t *testing.T, stdout, stderr []byte, runErr error, code string) {
	t.Helper()
	validateCatalogCLISchema(t, "catalog-build", stdout)
	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatal(err)
	}
	if runErr == nil || result.Result != "failed" || result.Error == nil || result.Error.Code != code || result.Error.Message == "" {
		t.Fatalf("expected %s failure: %v stdout=%s stderr=%s", code, runErr, stdout, stderr)
	}
}

func TestUnstampedCatalogBuilderCannotClaimAnIdentity(t *testing.T) {
	root := repositoryRoot(t)
	executable := filepath.Join(t.TempDir(), "packy")
	buildIssue798Executable(t, root, testprocess.GoOfflineEnv(t), executable, "./cmd/packy")
	project := t.TempDir()
	writePublicationCatalogPack(t, project, "alpha", "1.0.0", "valid\n")
	out := filepath.Join(t.TempDir(), "dist")
	stdout, stderr, err := runCatalogCLI(t, testprocess.Env(t), t.TempDir(), executable, "catalog", "build", "--json", "--project", project, "--source-repository", "example/catalog", "--source-commit", strings.Repeat("a", 40), "--out-dir", out)
	assertCatalogBuildFailure(t, stdout, stderr, err, "build_failed")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("unstamped build created output: %v", err)
	}
}
