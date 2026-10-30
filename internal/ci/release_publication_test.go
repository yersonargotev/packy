package ci_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestReleasePublicationRequiresFourPlatformCatalogContracts(t *testing.T) {
	type job struct {
		Needs    yaml.Node `yaml:"needs"`
		Uses     string    `yaml:"uses"`
		Strategy struct {
			Matrix struct {
				Include []struct {
					Platform string `yaml:"platform"`
					Runner   string `yaml:"runner"`
				} `yaml:"include"`
			} `yaml:"matrix"`
		} `yaml:"strategy"`
	}
	readWorkflow := func(name string) map[string]job {
		t.Helper()
		var workflow struct {
			Jobs map[string]job `yaml:"jobs"`
		}
		if err := yaml.Unmarshal([]byte(readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", name))), &workflow); err != nil {
			t.Fatal(err)
		}
		return workflow.Jobs
	}
	release := readWorkflow("release.yml")
	var publicationDependencies []string
	publication := release["publish"]
	if err := publication.Needs.Decode(&publicationDependencies); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(publicationDependencies, "prepare") || !slices.Contains(publicationDependencies, "contracts") || release["contracts"].Uses != "./.github/workflows/release-contracts.yml" {
		t.Fatal("immutable publication must depend on the complete release contract workflow")
	}
	contracts := readWorkflow("release-contracts.yml")
	want := map[string]string{"darwin_amd64": "macos-15-intel", "darwin_arm64": "macos-15", "linux_amd64": "ubuntu-24.04", "linux_arm64": "ubuntu-24.04-arm"}
	for _, target := range contracts["contracts"].Strategy.Matrix.Include {
		if want[target.Platform] != target.Runner {
			t.Errorf("unsupported or incompatible contract target: %s on %s", target.Platform, target.Runner)
		}
		delete(want, target.Platform)
	}
	if len(want) != 0 {
		t.Fatalf("release contracts omit targets: %v", want)
	}
	if readWorkflow("ci.yml")["release-contracts"].Uses != "./.github/workflows/release-contracts.yml" {
		t.Fatal("ordinary CI must exercise the same staged-binary release gate")
	}
}

func TestReleaseWorkflowUsesConventionalImmutableTagPublication(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readFile(t, filepath.Join(root, ".github", "workflows", "release.yml"))

	for _, required := range []string{
		"push:",
		"tags:",
		"scripts/build-release-artifacts.sh",
		"Source commit: `%s`",
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

func TestNextReleaseNotesProvideOneVersionPlaceholder(t *testing.T) {
	notes := readFile(t, filepath.Join(repositoryRoot(t), "docs", "release-notes", "next.md"))
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
	for _, required := range []string{"tar -tzf", "sha256sum", "--version", "packy version", "source_commit", "version --json"} {
		if !strings.Contains(validate, required) {
			t.Errorf("release validation is missing %q", required)
		}
	}
	for _, required := range []string{"git -C", "rev-parse HEAD", "archive \"$source_commit\"", "internal/version.SourceCommit=${source_commit}", "clean source checkout", "GOWORK=off GOFLAGS= GOENV=off"} {
		if !strings.Contains(build, required) {
			t.Errorf("release build omits source identity guarantee %q", required)
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

func TestReleaseBundleDownloadsUseCurrentRunAndReportFailures(t *testing.T) {
	type step struct {
		Name string            `yaml:"name"`
		Run  string            `yaml:"run"`
		Env  map[string]string `yaml:"env"`
	}
	tests := []struct {
		workflow, job, name, bundle, destination string
	}{
		{"release.yml", "publish", "Download validated release bundle", "packy-release-${{ needs.prepare.outputs.tag }}", "."},
		{"release.yml", "homebrew", "Download validated release bundle", "packy-release-${{ needs.prepare.outputs.tag }}", "bundle"},
		{"release.yml", "package-smoke", "Download validated release bundle", "packy-release-${{ needs.prepare.outputs.tag }}", "bundle"},
		{"release-contracts.yml", "contracts", "Download the staged artifacts", "${{ inputs.bundle }}", "."},
	}
	for _, tc := range tests {
		t.Run(tc.workflow+"/"+tc.job, func(t *testing.T) {
			var workflow struct {
				Jobs map[string]struct {
					Steps []step `yaml:"steps"`
				} `yaml:"jobs"`
			}
			content := readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", tc.workflow))
			if err := yaml.Unmarshal([]byte(content), &workflow); err != nil {
				t.Fatal(err)
			}
			var downloads []step
			for _, candidate := range workflow.Jobs[tc.job].Steps {
				if candidate.Name == tc.name {
					downloads = append(downloads, candidate)
				}
			}
			if len(downloads) != 1 {
				t.Fatalf("found %d bundle download steps, want one", len(downloads))
			}
			download := downloads[0]
			if download.Env["GH_TOKEN"] != "${{ github.token }}" || download.Env["RELEASE_BUNDLE"] != tc.bundle {
				t.Fatalf("unexpected download environment: %v", download.Env)
			}
			dir := t.TempDir()
			fake := filepath.Join(dir, "gh")
			if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGUMENTS_FILE\"\nif [ \"$FAIL_DOWNLOAD\" = 1 ]; then\n  echo 'underlying download error' >&2\n  exit 37\nfi\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			argsFile := filepath.Join(dir, "args")
			for _, fail := range []string{"0", "1"} {
				command := exec.Command("bash", "-e", "-c", download.Run)
				command.Env = testprocess.Env(t, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "GH_TOKEN=token", "GITHUB_RUN_ID=123456", "GITHUB_REPOSITORY=owner/repo", "RELEASE_BUNDLE=packy-release-v1.2.3", "ARGUMENTS_FILE="+argsFile, "FAIL_DOWNLOAD="+fail)
				output, err := command.CombinedOutput()
				if fail == "0" && err != nil {
					t.Fatalf("download failed: %v: %s", err, output)
				}
				if fail == "1" {
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 37 || !strings.Contains(string(output), "underlying download error") {
						t.Fatalf("download failure was not propagated: %v: %s", err, output)
					}
				}
				args := readFile(t, argsFile)
				want := "run\ndownload\n123456\n--repo\nowner/repo\n--name\npacky-release-v1.2.3\n--dir\n" + tc.destination + "\n"
				if args != want {
					t.Errorf("download arguments = %q, want %q", args, want)
				}
			}
		})
	}
}
