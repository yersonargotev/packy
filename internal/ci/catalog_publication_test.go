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
emit_release() {
  asset_count="$(find "$release" -type f ! -name target ! -name draft ! -name immutable ! -name listed | wc -l | tr -d ' ')"
  case "$asset_count" in
    0) assets='[]' ;;
    1) assets='[{}]' ;;
    2) assets='[{},{}]' ;;
    *) exit 2 ;;
  esac
  printf '{"tag_name":"catalog-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target_commitish":"%s","draft":%s,"immutable":%s,"assets":%s}\n' \
    "$(<"$release/target")" "$(<"$release/draft")" "$(<"$release/immutable")" "$assets"
}
case "$1 $2" in
  "api repos/example/catalog/releases/tags/catalog-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
    [[ -d "$release" ]] || exit 1
    [[ "$(<"$release/draft")" == false ]] || exit 1
    emit_release
    ;;
  "api --paginate")
    [[ "${FAKE_API_FAILURE:-false}" != true ]] || exit 2
    [[ -d "$release" ]] || exit 0
    if [[ "$(<"$release/draft")" == true && ! -e "$release/listed" ]]; then
      touch "$release/listed"
      exit 0
    fi
    emit_release
    ;;
  "release create")
    [[ ! -e "$release" ]] || exit 1
    mkdir "$release"
    saw_draft=false
    while (($#)); do
      if [[ "$1" == --target ]]; then printf '%s\n' "$2" > "$release/target"; fi
      if [[ "$1" == --draft ]]; then saw_draft=true; fi
      shift
    done
    [[ "$saw_draft" == true ]]
    printf 'true\n' > "$release/draft"
    printf 'false\n' > "$release/immutable"
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
  "release edit")
    [[ " $* " == *" --draft=false "* ]]
    printf 'false\n' > "$release/draft"
    printf 'true\n' > "$release/immutable"
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
	if err != nil || !strings.Contains(output, "published immutable Catalog Snapshot") {
		t.Fatalf("first publication: output=%q err=%v", output, err)
	}
	for name, want := range map[string]string{"draft": "false\n", "immutable": "true\n"} {
		got, readErr := os.ReadFile(filepath.Join(releaseRoot, "release", name))
		if readErr != nil || string(got) != want {
			t.Fatalf("published %s state: got=%q err=%v", name, got, readErr)
		}
	}
	output, err = run()
	if err != nil || !strings.Contains(output, "already published unchanged") {
		t.Fatalf("idempotent retry: output=%q err=%v", output, err)
	}
	if err := os.WriteFile(filepath.Join(releaseRoot, "release", "immutable"), []byte("false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err = run()
	if err == nil || !strings.Contains(output, "published Catalog Snapshot is mutable") {
		t.Fatalf("mutable publication: output=%q err=%v", output, err)
	}
	if err := os.RemoveAll(filepath.Join(releaseRoot, "release")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(releaseRoot, "release"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"target":    strings.Repeat("a", 40) + "\n",
		"draft":     "true\n",
		"immutable": "false\n",
	} {
		if err := os.WriteFile(filepath.Join(releaseRoot, "release", name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
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

	failureRoot := t.TempDir()
	command := exec.Command(script,
		"--repository", "example/catalog",
		"--commit", strings.Repeat("a", 40),
		"--dist", dist,
	)
	command.Env = testprocess.Env(t, "GH_BIN="+fakeGH, "FAKE_RELEASE_ROOT="+failureRoot, "FAKE_API_FAILURE=true")
	outputBytes, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(outputBytes), "could not determine Catalog Snapshot release state") {
		t.Fatalf("failed lookup: output=%q err=%v", outputBytes, err)
	}
	if _, statErr := os.Stat(filepath.Join(failureRoot, "release")); !os.IsNotExist(statErr) {
		t.Fatalf("failed lookup created release: err=%v", statErr)
	}
}
