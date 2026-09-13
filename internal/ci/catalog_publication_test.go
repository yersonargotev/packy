package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestCatalogSnapshotPublicationIsImmutableAndIdempotent(t *testing.T) {
	root := repositoryRoot(t)
	dist := t.TempDir()
	archive := []byte("deterministic snapshot bytes\n")
	if err := os.WriteFile(filepath.Join(dist, "catalog-snapshot.tar.gz"), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte("b6095fc720133eb0a6381f02ee0afc9b33812946b391f3086b2e3f8a49f09e2c  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	releaseRoot := t.TempDir()
	fakeGH := filepath.Join(t.TempDir(), "gh")
	fake := `#!/usr/bin/env bash
set -euo pipefail
release="$FAKE_RELEASE_ROOT/release"
case "$1 $2" in
  "api repos/example/catalog/releases/tags/catalog-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
    [[ -d "$release" ]] || exit 1
    printf '{"tag_name":"catalog-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target_commitish":"%s","draft":false}\n' "$(<"$release/target")"
    ;;
  "release create")
    [[ ! -e "$release" ]] || exit 1
    mkdir "$release"
    cp "$4" "$release/catalog-snapshot.tar.gz"
    cp "$5" "$release/SHA256SUMS"
    while (($#)); do
      if [[ "$1" == --target ]]; then printf '%s\n' "$2" > "$release/target"; break; fi
      shift
    done
    ;;
  "release download")
    while (($#)); do
      if [[ "$1" == --dir ]]; then destination="$2"; break; fi
      shift
    done
    [[ ! -f "$release/catalog-snapshot.tar.gz" ]] || cp "$release/catalog-snapshot.tar.gz" "$destination/"
    [[ ! -f "$release/SHA256SUMS" ]] || cp "$release/SHA256SUMS" "$destination/"
    ;;
  "release upload")
    cp "$4" "$release/$(basename "$4")"
    ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(fakeGH, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(root, "scripts", "publish-catalog-snapshot.sh")
	run := func() (string, error) {
		command := exec.Command(script,
			"--repository", "example/catalog",
			"--commit", strings.Repeat("a", 40),
			"--dist", dist,
		)
		command.Env = testprocess.Env(t, "GH_BIN="+fakeGH, "FAKE_RELEASE_ROOT="+releaseRoot)
		output, err := command.CombinedOutput()
		return string(output), err
	}

	output, err := run()
	if err != nil || !strings.Contains(output, "published Catalog Snapshot") {
		t.Fatalf("first publication: output=%q err=%v", output, err)
	}
	output, err = run()
	if err != nil || !strings.Contains(output, "already published unchanged") {
		t.Fatalf("idempotent retry: output=%q err=%v", output, err)
	}
	if err := os.Remove(filepath.Join(releaseRoot, "release", "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
	output, err = run()
	if err != nil || !strings.Contains(output, "completed interrupted") {
		t.Fatalf("interrupted retry: output=%q err=%v", output, err)
	}
	if err := os.WriteFile(filepath.Join(releaseRoot, "release", "catalog-snapshot.tar.gz"), []byte("replaced bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err = run()
	if err == nil || !strings.Contains(output, "published Catalog Snapshot bytes differ") {
		t.Fatalf("conflicting retry: output=%q err=%v", output, err)
	}
}
