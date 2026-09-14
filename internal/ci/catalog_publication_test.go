package ci_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestIssue798ReviewedContentCandidatePublishesCompleteSnapshot(t *testing.T) {
	baseline := t.TempDir()
	writePublicationCatalogPack(t, baseline, "alpha", "1.0.0", "old alpha\n")
	writePublicationCatalogPack(t, baseline, "stable", "2.0.0", "stable\n")
	candidate := t.TempDir()
	writePublicationCatalogPack(t, candidate, "alpha", "1.1.0", "new alpha\n")
	writePublicationCatalogPack(t, candidate, "beta", "1.0.0", "new beta\n")
	writePublicationCatalogPack(t, candidate, "stable", "2.0.0", "stable\n")

	validation, err := managedpack.ValidateCatalogProject(context.Background(), candidate, baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	identities := make([]string, 0, len(validation.Packs))
	for _, pack := range validation.Packs {
		identities = append(identities, pack.Manifest.ID+"@"+pack.Manifest.Version)
	}
	if want := []string{"alpha@1.1.0", "beta@1.0.0", "stable@2.0.0"}; !slices.Equal(identities, want) {
		t.Fatalf("validated candidate Packs = %v, want %v", identities, want)
	}

	commit := strings.Repeat("3", 40)
	dist := filepath.Join(t.TempDir(), "dist")
	result, err := managedpack.BuildCatalogSnapshot(context.Background(), candidate, validation, managedpack.CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog",
		Commit:     commit,
		Builder:    "yersonargotev/packy@" + strings.Repeat("4", 40),
	}, dist)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Index.Packs) != 3 {
		t.Fatalf("complete Catalog Snapshot Packs = %d, want 3", len(result.Index.Packs))
	}

	fakeGH, releaseRoot := catalogPublicationFixture(t)
	command := exec.Command(filepath.Join(repositoryRoot(t), "scripts", "publish-catalog-snapshot.sh"),
		"--repository", "example/catalog", "--commit", commit, "--dist", dist)
	command.Env = testprocess.Env(t, "GH_BIN="+fakeGH, "FAKE_RELEASE_ROOT="+releaseRoot, "FAKE_CATALOG_COMMIT="+commit)
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "published immutable Catalog Snapshot") {
		t.Fatalf("publish reviewed content candidate: %v\n%s", err, output)
	}
	for _, asset := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		published, readErr := os.ReadFile(filepath.Join(releaseRoot, "release", asset))
		if readErr != nil {
			t.Fatal(readErr)
		}
		built, readErr := os.ReadFile(filepath.Join(dist, asset))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !slices.Equal(published, built) {
			t.Fatalf("published %s differs from validated candidate", asset)
		}
	}
}

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

	fakeGH, releaseRoot := catalogPublicationFixture(t)

	script := filepath.Join(root, "scripts", "publish-catalog-snapshot.sh")
	run := func() (string, error) {
		command := exec.Command(script,
			"--repository", "example/catalog",
			"--commit", strings.Repeat("a", 40),
			"--dist", dist,
		)
		command.Env = testprocess.Env(t, "GH_BIN="+fakeGH, "FAKE_RELEASE_ROOT="+releaseRoot, "FAKE_CATALOG_COMMIT="+strings.Repeat("a", 40))
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
	command.Env = testprocess.Env(t, "GH_BIN="+fakeGH, "FAKE_RELEASE_ROOT="+failureRoot, "FAKE_CATALOG_COMMIT="+strings.Repeat("a", 40), "FAKE_API_FAILURE=true")
	outputBytes, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(outputBytes), "could not determine Catalog Snapshot release state") {
		t.Fatalf("failed lookup: output=%q err=%v", outputBytes, err)
	}
	if _, statErr := os.Stat(filepath.Join(failureRoot, "release")); !os.IsNotExist(statErr) {
		t.Fatalf("failed lookup created release: err=%v", statErr)
	}
}

func writePublicationCatalogPack(t *testing.T, root, id, version, content string) {
	t.Helper()
	source := filepath.Join("skills", id)
	resource := filepath.Join(root, "bundle", source, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(resource), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resource, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "schema_version": 1,
  "id": %q,
  "version": %q,
  "description": "Publication fixture",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [{
    "kind": "skill",
    "id": %q,
    "source": %q,
    "description": "Publication fixture skill",
    "requires": [],
    "conflicts": [],
    "bindings": [{
      "surface": "codex",
      "projection": "skill",
      "name": %q,
      "invocation": %q,
      "mode": "native",
      "sharing": "exclusive",
      "capabilities": []
    }],
    "surface_exclusions": []
  }]
}
`, id, version, id, source, id, "$"+id)
	manifestPath := filepath.Join(root, "bundle", "packs", id, "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalogPublicationFixture(t *testing.T) (string, string) {
	t.Helper()
	fakeGH := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(fakeGH, []byte(fakeCatalogPublicationGH), 0o755); err != nil {
		t.Fatal(err)
	}
	return fakeGH, t.TempDir()
}

const fakeCatalogPublicationGH = `#!/usr/bin/env bash
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
  printf '{"tag_name":"catalog-%s","target_commitish":"%s","draft":%s,"immutable":%s,"assets":%s}\n' \
    "$FAKE_CATALOG_COMMIT" "$(<"$release/target")" "$(<"$release/draft")" "$(<"$release/immutable")" "$assets"
}
case "$1 $2" in
  "api repos/example/catalog/releases/tags/catalog-"*)
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
