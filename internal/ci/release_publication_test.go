package ci_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseWorkflowUsesConventionalImmutableTagPublication(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readFile(t, filepath.Join(root, ".github", "workflows", "release.yml"))

	for _, required := range []string{
		"push:",
		"tags:",
		"scripts/build-release-artifacts.sh",
		"scripts/validate-release-artifacts.sh",
		"scripts/generate-homebrew-formula.sh",
		"gh release create",
		"gh release view",
		"gh release download",
		`[[ "$GITHUB_SHA" == "$(git rev-parse origin/main)" ]]`,
		"show origin/main:Formula/packy.rb",
		"SHA256SUMS",
		"brew install yersonargotev/tap/packy",
		`"packy version $RELEASE_TAG"`,
		"HOME: ${{ runner.temp }}/home",
		"XDG_CONFIG_HOME: ${{ runner.temp }}/xdg",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
	for _, retired := range []string{
		"workflow_dispatch:",
		"releasecandidate",
		"recovery",
		"sbom",
		"attestation",
		"id-token: write",
		"attestations: write",
		"--clobber",
		"brew install --formula bundle/release-metadata/packy.rb",
	} {
		if strings.Contains(strings.ToLower(workflow), strings.ToLower(retired)) {
			t.Errorf("release workflow retains retired behavior %q", retired)
		}
	}
	if got := strings.Count(workflow, "gh release create"); got != 1 {
		t.Errorf("release workflow creates %d GitHub releases; want exactly one", got)
	}
}

func TestReleaseArtifactsAreArchivesWithOneChecksumManifest(t *testing.T) {
	root := repositoryRoot(t)
	build := readFile(t, filepath.Join(root, "scripts", "build-release-artifacts.sh"))
	validate := readFile(t, filepath.Join(root, "scripts", "validate-release-artifacts.sh"))
	formula := readFile(t, filepath.Join(root, "scripts", "generate-homebrew-formula.sh"))

	for _, platform := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"} {
		if !strings.Contains(build, platform) {
			t.Errorf("release build omits %s", platform)
		}
	}
	for _, required := range []string{".tar.gz", "SHA256SUMS"} {
		if !strings.Contains(build, required) || !strings.Contains(validate, required) {
			t.Errorf("release build or validation is missing %q", required)
		}
	}
	for _, required := range []string{"tar -tzf", "sha256sum", "--version", "packy version"} {
		if !strings.Contains(validate, required) {
			t.Errorf("release validation is missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(build), "sbom") {
		t.Fatal("release build still produces the retired SBOM")
	}
	if !strings.Contains(formula, ".tar.gz") || !strings.Contains(formula, `bin.install "packy"`) {
		t.Fatal("Homebrew formula generation does not install the matching release archive")
	}
}

func TestCustomReleaseDomainIsRemoved(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		"internal/release",
		"internal/tools/releasecandidate",
		"internal/tools/releasesbom",
		"scripts/normalize-release-event.sh",
		"scripts/acquire-retained-release-candidate.sh",
		"scripts/verify-retained-release-candidate.sh",
		"scripts/verify-release-boundary.sh",
		"scripts/verify-release-evidence.sh",
		"scripts/verify-release-ref-state.sh",
		"scripts/test-release-scenarios.sh",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("retired custom release path still exists: %s", path)
		}
	}
}
