package ci_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func testCatalogOriginReuse(t *testing.T, executable string) {
	var downloads atomic.Int32
	commit, proxy, ca := catalogGitOrigin(t, &downloads)
	candidate, baseline, directory := t.TempDir(), t.TempDir(), t.TempDir()
	writeCatalogOriginPack(t, candidate, commit)
	writeCatalogOriginPack(t, baseline, commit)
	env := testprocess.Env(t, "HTTPS_PROXY="+proxy, "SSL_CERT_FILE="+ca)
	// Keep every real invocation in a disposable Git worktree as well as a home.
	if _, stderr, err := runCatalogCLI(t, env, directory, "git", "init", "-q"); err != nil {
		t.Fatalf("git init: %v %s", err, stderr)
	}
	validate := func(want bool) []byte {
		t.Helper()
		out, stderr, err := runCatalogCLI(t, env, directory, executable, "catalog", "validate", "--project", candidate, "--baseline", baseline, "--json")
		code := ""
		if !want {
			code = "validation_failed"
		}
		assertCatalogResult(t, out, stderr, err, want, code)
		return out
	}
	build := func() ([]byte, []byte) {
		t.Helper()
		dist := filepath.Join(t.TempDir(), "dist")
		out, stderr, err := runCatalogCLI(t, env, directory, executable, "catalog", "build", "--project", candidate, "--baseline", baseline, "--source-repository", "example/catalog", "--source-commit", strings.Repeat("a", 40), "--out-dir", dist, "--json")
		if err != nil {
			t.Fatalf("build: %v stdout=%s stderr=%s", err, out, stderr)
		}
		validateCatalogCLISchema(t, "catalog-build", out)
		return readIssue798File(t, filepath.Join(dist, "catalog-snapshot.tar.gz")), readIssue798File(t, filepath.Join(dist, "SHA256SUMS"))
	}
	coldValidation := validate(true)
	warmValidation := validate(true)
	warmArchive, warmChecksum := build()
	if !bytes.Equal(coldValidation, warmValidation) {
		t.Fatal("warm reuse changed validation report")
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("validate + validate + build downloads=%d, want 1", got)
	}
	// Removing the documented cache creates a genuinely cold build comparison.
	cacheBase := ""
	for _, entry := range env {
		if runtime.GOOS == "darwin" && strings.HasPrefix(entry, "HOME=") {
			cacheBase = filepath.Join(strings.TrimPrefix(entry, "HOME="), "Library", "Caches")
		}
		if runtime.GOOS != "darwin" && strings.HasPrefix(entry, "XDG_CACHE_HOME=") {
			cacheBase = strings.TrimPrefix(entry, "XDG_CACHE_HOME=")
		}
	}
	if cacheBase == "" {
		t.Fatal("missing isolated cache root")
	}
	if err := os.RemoveAll(filepath.Join(cacheBase, "packy", "catalog-origins-v1")); err != nil {
		t.Fatal(err)
	}
	coldArchive, coldChecksum := build()
	if !bytes.Equal(coldArchive, warmArchive) || !bytes.Equal(coldChecksum, warmChecksum) {
		t.Fatal("cold/warm snapshot bytes or checksums differ")
	}
	if got := downloads.Load(); got != 2 {
		t.Fatalf("cold build downloads=%d, want 2 total", got)
	}
	// A warm origin never waives candidate bytes or baseline version validation.
	skill := filepath.Join(candidate, "packs", "alpha", "skills", "alpha", "SKILL.md")
	if err := os.WriteFile(skill, []byte("changed candidate\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out := validate(false); !bytes.Contains(out, []byte("exact-copy")) {
		t.Fatalf("missing exact-copy rejection: %s", out)
	}
	if err := os.WriteFile(skill, []byte("upstream skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mutateCatalogManifest(t, candidate, func(m map[string]any) { m["description"] = "Changed without a version bump" })
	validate(false)
	if downloads.Load() != 2 {
		t.Fatal("warm negative validation downloaded again")
	}
	t.Log("controlled Git upload-pack transfers: cold validate=1; warm validate+build=0; independent cold build=1; identical archives/checksums; warm exact-copy and baseline violations rejected")
}
