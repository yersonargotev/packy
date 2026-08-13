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

func TestReleaseGateRequiresFreshDecisionEvidence(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readFile(t, filepath.Join(root, "workflows", "packy-release.md"))
	maintainer := readFile(t, filepath.Join(root, "docs", "release.md"))
	skill := readFile(t, filepath.Join(root, ".agents", "skills", "release-packy", "SKILL.md"))
	notes := readFile(t, filepath.Join(root, "docs", "release-notes", "next.md"))
	workflowText := strings.Join(strings.Fields(workflow), " ")
	maintainerText := strings.Join(strings.Fields(maintainer), " ")

	for _, required := range []string{
		"latest published stable GitHub Release",
		"next patch",
		"every user-visible change",
		"exactly one `{{TAG}}` placeholder",
		"repository authorities",
		"release-note summary or diff evidence",
		"`release` and `homebrew` environment approvals",
		"same workflow run, version, and commit",
	} {
		if !strings.Contains(workflowText, required) {
			t.Errorf("release workflow contract is missing %q", required)
		}
	}
	for _, required := range []string{
		"latest published stable GitHub Release",
		"next patch",
		"every user-visible change",
		"exactly one `{{TAG}}` placeholder",
		"repository authorities",
	} {
		if !strings.Contains(maintainerText, required) {
			t.Errorf("maintainer release contract is missing %q", required)
		}
	}

	for _, required := range []string{
		"Read the complete",
		"Run the complete workflow contract",
		"repository-change exception",
	} {
		if !strings.Contains(skill, required) {
			t.Errorf("release skill gate is missing %q", required)
		}
	}
	for _, duplicatedPhase := range []string{"## 1. Establish", "## 2. Prove", "## 3. Approve", "## 4. Publish once", "## 5. Verify and close"} {
		if strings.Contains(skill, duplicatedPhase) {
			t.Errorf("release skill duplicates workflow phase %q", duplicatedPhase)
		}
	}

	if got := strings.Count(notes, "{{TAG}}"); got != 1 {
		t.Errorf("next release notes contain %d tag placeholders; want exactly one", got)
	}
	if !strings.Contains(notes, "## Changes since the previous release") {
		t.Error("next release notes do not identify the candidate delta")
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
