package ci_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/githubfake"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestCatalogPublicationAndConsumption(t *testing.T) {
	catalogCLI, _ := buildCatalogCLI(t)
	root := repositoryRoot(t)
	baseline := t.TempDir()
	writePublicationCatalogPack(t, baseline, "alpha", "1.0.0", "old alpha\n")
	writePublicationCatalogPack(t, baseline, "stable", "2.0.0", "stable\n")
	candidate := t.TempDir()
	writePublicationCatalogPack(t, candidate, "alpha", "1.1.0", "new alpha\n")
	writePublicationCatalogPack(t, candidate, "beta", "1.0.0", "new beta\n")
	writePublicationCatalogPack(t, candidate, "stable", "2.0.0", "stable\n")

	bin := t.TempDir()
	buildEnvironment := testprocess.GoOfflineEnv(t)
	buildGoExecutable(t, root, buildEnvironment, filepath.Join(bin, "packy"), "./internal/cli/testdata/packytransport")

	validation := runProcess(t, testprocess.Env(t), root, catalogCLI,
		"catalog", "validate", "--project", candidate, "--baseline", baseline)
	for _, want := range []string{"validated Catalog Project packs=3", "alpha@1.1.0", "beta@1.0.0", "stable@2.0.0"} {
		if !strings.Contains(validation, want) {
			t.Fatalf("candidate validation omitted %q:\n%s", want, validation)
		}
	}

	baselineCommit := strings.Repeat("3", 40)
	candidateCommit := strings.Repeat("4", 40)
	baselineDist := filepath.Join(t.TempDir(), "dist")
	candidateDist := filepath.Join(t.TempDir(), "dist")
	for _, snapshot := range []struct {
		project, commit, dist string
	}{{baseline, baselineCommit, baselineDist}, {candidate, candidateCommit, candidateDist}} {
		output := runProcess(t, testprocess.Env(t), root, catalogCLI, "catalog", "build",
			"--project", snapshot.project,
			"--source-repository", "yersonargotev/packy-catalog",
			"--source-commit", snapshot.commit,
			"--out-dir", snapshot.dist)
		if !strings.Contains(output, "built Catalog Snapshot") {
			t.Fatalf("snapshot build output:\n%s", output)
		}
	}

	fakeGH, baselineReleaseRoot := catalogPublicationFixture(t)
	candidateReleaseRoot := t.TempDir()
	publishCatalogSnapshot(t, catalogCLI, fakeGH, baselineReleaseRoot, baselineCommit, baselineDist)
	publishCatalogSnapshot(t, catalogCLI, fakeGH, candidateReleaseRoot, candidateCommit, candidateDist)

	fixtures := t.TempDir()
	if err := githubfake.Publish(fixtures, "yersonargotev/packy-catalog", baselineCommit, filepath.Join(baselineReleaseRoot, "release")); err != nil {
		t.Fatal(err)
	}
	github := httptest.NewTLSServer(githubfake.Handler(fixtures))
	defer github.Close()
	addressFile := filepath.Join(t.TempDir(), "addr")
	if err := githubfake.AddressOf(github).Write(addressFile); err != nil {
		t.Fatal(err)
	}
	packyEnvironment := testprocess.Env(t, "PACKY_GITHUBFAKE_ADDR_FILE="+addressFile)
	packy := filepath.Join(bin, "packy")
	executableDigest := sha256Hex(readFileBytes(t, packy))
	if output := runProcess(t, packyEnvironment, t.TempDir(), packy, "init"); !strings.Contains(output, baselineCommit) {
		t.Fatalf("initialization omitted baseline snapshot %s:\n%s", baselineCommit, output)
	}

	if err := githubfake.Publish(fixtures, "yersonargotev/packy-catalog", candidateCommit, filepath.Join(candidateReleaseRoot, "release")); err != nil {
		t.Fatal(err)
	}
	if output := runProcess(t, packyEnvironment, t.TempDir(), packy, "catalog", "refresh"); !strings.Contains(output, candidateCommit) {
		t.Fatalf("refresh omitted candidate snapshot %s:\n%s", candidateCommit, output)
	}
	github.Close()
	for _, check := range []struct {
		want string
		args []string
	}{
		{"alpha   1.1.0", []string{"list"}},
		{"beta    1.0.0", []string{"list"}},
		{"stable  2.0.0", []string{"list"}},
		{"alpha", []string{"activate", "alpha", "--surface", "agents", "--dry-run"}},
		{"beta", []string{"activate", "beta", "--surface", "agents", "--dry-run"}},
	} {
		if output := runProcess(t, packyEnvironment, t.TempDir(), packy, check.args...); !strings.Contains(output, check.want) {
			t.Fatalf("offline Packy %v omitted %q:\n%s", check.args, check.want, output)
		}
	}
	if finalDigest := sha256Hex(readFileBytes(t, packy)); finalDigest != executableDigest {
		t.Fatalf("installed Packy executable changed across Catalog Publications: %s -> %s", executableDigest, finalDigest)
	}
}

func testCatalogPublicationExecutable(t *testing.T, executable string) {
	dist := t.TempDir()
	archive := []byte("deterministic snapshot bytes\n")
	if err := os.WriteFile(filepath.Join(dist, "catalog-snapshot.tar.gz"), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte("b6095fc720133eb0a6381f02ee0afc9b33812946b391f3086b2e3f8a49f09e2c  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeGH, releaseRoot := catalogPublicationFixture(t)

	run := func() (string, error) {
		environment := testprocess.Env(t, "PATH="+filepath.Dir(fakeGH), "FAKE_RELEASE_ROOT="+releaseRoot, "FAKE_CATALOG_COMMIT="+strings.Repeat("a", 40))
		stdout, stderr, err := runCatalogCLI(t, environment, t.TempDir(), executable,
			"catalog", "publish", "--repository", "example/catalog", "--commit", strings.Repeat("a", 40), "--dist", dist)
		return string(stdout) + string(stderr), err
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

	t.Run("structured publication contract", func(t *testing.T) { testCatalogPublicationContract(t, executable) })

	failureRoot := t.TempDir()
	environment := testprocess.Env(t, "PATH="+filepath.Dir(fakeGH), "FAKE_RELEASE_ROOT="+failureRoot, "FAKE_CATALOG_COMMIT="+strings.Repeat("a", 40), "FAKE_API_FAILURE=true")
	stdout, stderr, err := runCatalogCLI(t, environment, t.TempDir(), executable,
		"catalog", "publish", "--repository", "example/catalog", "--commit", strings.Repeat("a", 40), "--dist", dist)
	outputBytes := append(stdout, stderr...)
	if err == nil || !strings.Contains(string(outputBytes), "could not determine Catalog Snapshot release state") {
		t.Fatalf("failed lookup: output=%q err=%v", outputBytes, err)
	}
	if _, statErr := os.Stat(filepath.Join(failureRoot, "release")); !os.IsNotExist(statErr) {
		t.Fatalf("failed lookup created release: err=%v", statErr)
	}
}

func buildGoExecutable(t *testing.T, root string, environment []string, output, packagePath string) {
	t.Helper()
	command := exec.Command("go", "build", "-o", output, packagePath)
	command.Dir = root
	command.Env = append([]string(nil), environment...)
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, combined)
	}
}

func runProcess(t *testing.T, environment []string, directory, executable string, args ...string) string {
	t.Helper()
	command := exec.Command(executable, args...)
	command.Dir = directory
	command.Env = append([]string(nil), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v: %v\n%s", executable, args, err, output)
	}
	return string(output)
}

func publishCatalogSnapshot(t *testing.T, executable, fakeGH, releaseRoot, commit, dist string) {
	t.Helper()
	environment := testprocess.Env(t, "PATH="+filepath.Dir(fakeGH), "FAKE_RELEASE_ROOT="+releaseRoot, "FAKE_CATALOG_COMMIT="+commit)
	stdout, stderr, err := runCatalogCLI(t, environment, t.TempDir(), executable,
		"catalog", "publish", "--repository", "example/catalog", "--commit", commit, "--dist", dist)
	output := append(stdout, stderr...)
	if err != nil || !strings.Contains(string(output), "published immutable Catalog Snapshot") {
		t.Fatalf("publish Catalog Snapshot %s: %v\n%s", commit, err, output)
	}
	for _, asset := range []string{"catalog-snapshot.tar.gz", "SHA256SUMS"} {
		published := readFileBytes(t, filepath.Join(releaseRoot, "release", asset))
		built := readFileBytes(t, filepath.Join(dist, asset))
		if string(published) != string(built) {
			t.Fatalf("published %s differs from validated candidate", asset)
		}
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writePublicationCatalogPack(t *testing.T, root, id, version, content string) {
	t.Helper()
	source := filepath.Join("skills", id)
	packRoot := filepath.Join(root, "packs", id)
	resource := filepath.Join(packRoot, source, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(resource), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resource, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{
  "schema_version": 3,
  "id": %q,
  "version": %q,
  "description": "Publication fixture",
  "selectable": true,
  "surfaces": ["agents"],
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
      "surface": "agents",
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
	manifestPath := filepath.Join(packRoot, "pack.json")
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
	for _, name := range []string{"bash", "basename", "cp", "mkdir", "touch"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(filepath.Dir(fakeGH), name)); err != nil {
			t.Fatal(err)
		}
	}
	return fakeGH, t.TempDir()
}

const fakeCatalogPublicationGH = `#!/usr/bin/env bash
set -euo pipefail
release="$FAKE_RELEASE_ROOT/release"
emit_release() {
  assets='['
  separator=''
  for asset in "$release"/*; do
    case "$(basename "$asset")" in target|draft|immutable|listed) continue ;; esac
    [[ -f "$asset" ]] || continue
    assets="$assets$separator{}"
    separator=','
  done
  assets="$assets]"
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
    [[ "${FAKE_DOWNLOAD_FAILURE:-false}" != true ]] || exit 2
    for asset in "$release"/*; do
      case "$(basename "$asset")" in target|draft|immutable|listed) continue ;; esac
      [[ ! -f "$asset" ]] || cp "$asset" "$destination/"
    done
    ;;
  "release upload")
    [[ "${FAKE_UPLOAD_FAILURE:-}" != "$(basename "$4")" ]] || exit 2
    cp "$4" "$release/$(basename "$4")"
    ;;
  "release edit")
    [[ " $* " == *" --draft=false "* ]]
    printf 'false\n' > "$release/draft"
    printf '%s\n' "${FAKE_PUBLISHED_IMMUTABLE:-true}" > "$release/immutable"
    ;;
  *) exit 2 ;;
esac
`
