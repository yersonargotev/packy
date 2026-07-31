package release_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
	"golang.org/x/sys/unix"
)

var supportedReleasePlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
}

func TestReleaseWorkflowClassifiesTagPushAndManualModes(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))

	for _, want := range []string{
		"push:",
		"- 'v0.*.*'",
		"workflow_dispatch:",
		"dry_run:",
		"type: boolean",
		"Normalize and seal release identity read-only",
		"Classify event and seal immutable candidate",
		"run: ./scripts/normalize-release-event.sh",
		"scripts/build-release-artifacts.sh",
		"sbom.spdx.json",
		"SHA256SUMS",
		"Retain the one built candidate",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q", want)
		}
	}
	if got := strings.Count(text, "scripts/build-release-artifacts.sh"); got != 1 {
		t.Fatalf("release candidate must be built exactly once; got %d build invocations", got)
	}

	dryRun := baseReleaseEventFixture()
	dryRun.eventName = "workflow_dispatch"
	dryRun.eventRef = "refs/heads/main"
	dryRun.eventRefType = "branch"
	dryRun.eventRefName = "main"
	dryRun.eventSHA = dryRun.mainCommit
	dryRun.inputTag = dryRun.tag
	dryRun.inputDryRun = "true"
	output, err := runReleaseEventFixture(t, text, dryRun)
	if err != nil {
		t.Fatalf("valid manual dry run should remain available: %v", err)
	}
	assertReleaseEventOutput(t, output, "dry-run", dryRun.tag, dryRun.tagCommit)

	recovery := dryRun
	recovery.inputDryRun = "false"
	recovery.releaseID = "R_release"
	recovery.releaseState = "draft"
	recovery.originalRunID = "12345"
	output, err = runReleaseEventFixture(t, text, recovery)
	if err != nil {
		t.Fatalf("valid exact-tag recovery should remain available: %v", err)
	}
	assertReleaseEventOutput(t, output, "recovery", recovery.tag, recovery.tagCommit)

	absentRecovery := dryRun
	absentRecovery.inputDryRun = "false"
	if _, err := runReleaseEventFixture(t, text, absentRecovery); err == nil {
		t.Fatal("manual recovery without an existing sealed same-tag release unexpectedly passed")
	}
}

func TestReleaseWorkflowRejectsUnauthorizedTriggersAndVersions(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))

	tests := []struct {
		name   string
		mutate func(*releaseEventFixture)
	}{
		{name: "branch push", mutate: func(f *releaseEventFixture) {
			f.eventRef, f.eventRefType, f.eventRefName = "refs/heads/main", "branch", "main"
		}},
		{name: "malformed tag", mutate: func(f *releaseEventFixture) {
			f.tag, f.eventRef, f.eventRefName = "v1.2.0", "refs/tags/v1.2.0", "v1.2.0"
		}},
		{name: "occupied release", mutate: func(f *releaseEventFixture) {
			f.releaseID = "R_release"
		}},
		{name: "non monotonic version", mutate: func(f *releaseEventFixture) {
			f.existingReleases = "v0.3.0"
		}},
		{name: "moved event tag", mutate: func(f *releaseEventFixture) {
			f.eventSHA = strings.Repeat("c", 40)
		}},
		{name: "ambiguous manual input", mutate: func(f *releaseEventFixture) {
			f.eventName = "workflow_dispatch"
			f.eventRef, f.eventRefType, f.eventRefName = "refs/heads/main", "branch", "main"
			f.eventSHA = f.mainCommit
			f.inputTag = f.tag
			f.inputDryRun = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := baseReleaseEventFixture()
			test.mutate(&fixture)
			if _, err := runReleaseEventFixture(t, text, fixture); err == nil {
				t.Fatal("unauthorized release event unexpectedly passed normalization")
			}
		})
	}
}

func TestReleaseWorkflowSealsCandidateAndRevalidatesPrivilegedBoundaries(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	recovery := baseReleaseEventFixture()
	recovery.eventName = "workflow_dispatch"
	recovery.eventRef, recovery.eventRefType, recovery.eventRefName = "refs/heads/main", "branch", "main"
	recovery.mainCommit = strings.Repeat("b", 40)
	recovery.eventSHA = recovery.mainCommit
	recovery.inputTag, recovery.inputDryRun = recovery.tag, "false"
	recovery.releaseID = "R_release"
	recovery.releaseState = "draft"
	recovery.originalRunID = "12345"
	if _, err := runReleaseEventFixture(t, text, recovery); err != nil {
		t.Fatalf("later main advancement should preserve an ancestral sealed candidate: %v", err)
	}
	recovery.ancestor = false
	if _, err := runReleaseEventFixture(t, text, recovery); err == nil {
		t.Fatal("candidate outside protected-main history unexpectedly reached recovery")
	}

	for _, job := range []string{"inspect-release", "attest", "publish-github", "homebrew"} {
		block := releaseWorkflowJob(t, text, job)
		for _, want := range []string{"verify-release-ref-state.sh", "--release-commit"} {
			if !strings.Contains(block, want) {
				t.Fatalf("%s lacks post-seal identity or ancestry proof %q", job, want)
			}
		}
		if strings.Contains(block, `"$main_commit" == "$tag_commit"`) {
			t.Fatalf("%s must not invalidate a sealed candidate merely because protected main advanced", job)
		}
	}
	normalizer := readReleaseEventNormalizer(t, repoRoot(t))
	if got := strings.Count(text, "resolve_ref_commit()"); got != 0 {
		t.Fatalf("workflow must not retain private inline ref observers; got %d", got)
	}
	if got := strings.Count(normalizer, "resolve_ref_commit()"); got != 1 {
		t.Fatalf("checked-in normalizer must own one ref observer; got %d", got)
	}
}

func TestReleaseWorkflowAcquiresBoundaryAdapterOutsideRetainedRecoveryMetadata(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	for _, job := range []string{"attest", "publish-github", "homebrew"} {
		block := releaseWorkflowJob(t, text, job)
		for _, want := range []string{
			"Check out trusted release boundary adapter",
			"ref: ${{ github.sha }}",
			"path: release-boundary",
			"sparse-checkout: scripts",
			"persist-credentials: false",
			"release-boundary/scripts/verify-release-boundary.sh",
		} {
			if !strings.Contains(block, want) {
				t.Fatalf("%s must acquire the trusted boundary adapter independently of retained metadata: missing %q", job, want)
			}
		}
	}
	if strings.Contains(releaseWorkflowJob(t, text, "build"), "cp scripts/verify-release-boundary.sh release-metadata/") {
		t.Fatal("the boundary adapter must remain independent of retained candidate metadata")
	}
}

func TestReleaseWorkflowAdmissionPassesRawReleaseMetadataFacts(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	normalize := readReleaseEventNormalizer(t, repoRoot(t))
	if !strings.Contains(releaseWorkflowJob(t, text, "normalize"), "run: ./scripts/normalize-release-event.sh") {
		t.Fatal("normalize job must execute the checked-in event adapter")
	}
	for _, want := range []string{
		"release_schema_version",
		"release_candidate_id",
		"release_attestation_source_ref",
		`--slurpfile release_metadata "$RUNNER_TEMP/admission-metadata.json"`,
	} {
		if !strings.Contains(normalize, want) {
			t.Fatalf("normalization must pass raw release metadata fact %q", want)
		}
	}
	if strings.Contains(normalize, "release_sealed") {
		t.Fatal("workflow adapter must not derive or pass a trusted release_sealed verdict")
	}
	for _, forbidden := range []string{
		"--argjson release_schema_version",
		"--arg release_candidate_id",
		"--arg release_attestation_source_ref",
	} {
		if strings.Contains(normalize, forbidden) {
			t.Fatalf("workflow adapter must preserve raw release metadata types instead of rebuilding %q", forbidden)
		}
	}
}

func readReleaseEventNormalizer(t *testing.T, root string) string {
	t.Helper()
	normalizer, err := os.ReadFile(filepath.Join(root, "scripts", "normalize-release-event.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(normalizer)
}

func TestVerifyReleaseRefStateAdapterOwnsRemoteObservation(t *testing.T) {
	root := repoRoot(t)
	bin := t.TempDir()
	commit := strings.Repeat("a", 40)
	mainCommit := strings.Repeat("b", 40)
	gh := `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *git/ref/tags/*) printf 'commit\t%s\n' "$TAG_COMMIT" ;;
  *git/ref/heads/main*) printf 'commit\t%s\n' "$MAIN_COMMIT" ;;
  *compare/*) printf '%s\n' "$COMPARE_STATUS" ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 91 ;;
esac
`
	verifier := `#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == verify-ref-state && "$2" == --observation ]]
jq -e --arg commit "$EXPECTED_COMMIT" --arg main "$MAIN_COMMIT" '
  .expected_tag_commit==$commit and .remote_tag_commit==$commit and
  .release_commit==$commit and .current_main==$main and .release_in_main==true
' "$3" >/dev/null
jq -n --arg commit "$EXPECTED_COMMIT" --arg main "$MAIN_COMMIT" \
  '{verified:true,tag:"v0.2.0",release_commit:$commit,current_main:$main}'
`
	for name, contents := range map[string]string{"gh": gh, "releasecandidate": verifier} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "verified.json")
	cmd := exec.Command(filepath.Join(root, "scripts", "verify-release-ref-state.sh"),
		"--repository", "yersonargotev/packy", "--tag", "v0.2.0",
		"--release-commit", commit, "--verifier", filepath.Join(bin, "releasecandidate"),
		"--output", output)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "RUNNER_TEMP="+t.TempDir(),
		"TAG_COMMIT="+commit, "MAIN_COMMIT="+mainCommit, "EXPECTED_COMMIT="+commit,
		"COMPARE_STATUS=ahead")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify release refs: %v\n%s", err, combined)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"verified": true`) {
		t.Fatalf("adapter did not retain verifier output:\n%s", data)
	}

	cmd = exec.Command(filepath.Join(root, "scripts", "verify-release-ref-state.sh"),
		"--repository", "yersonargotev/packy", "--tag", "v0.2.0",
		"--release-commit", commit, "--verifier", filepath.Join(bin, "releasecandidate"),
		"--output", output)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "RUNNER_TEMP="+t.TempDir(),
		"TAG_COMMIT="+strings.Repeat("c", 40), "MAIN_COMMIT="+mainCommit,
		"EXPECTED_COMMIT="+commit, "COMPARE_STATUS=ahead")
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatal("adapter accepted a moved release tag")
	}
}

func TestReleaseWorkflowRecoveryUsesOnlyOriginalRetainedCandidate(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	build := releaseWorkflowJob(t, text, "build")
	validate := releaseWorkflowJob(t, text, "validate-release-evidence")
	attest := releaseWorkflowJob(t, text, "attest")
	publish := releaseWorkflowJob(t, text, "publish-github")

	for _, block := range []string{validate, attest, publish} {
		for _, want := range []string{
			"needs.normalize.outputs.original_run_id",
			"github-token: ${{ github.token }}",
		} {
			if !strings.Contains(block, want) {
				t.Fatalf("recovery consumer must retrieve original-run artifacts with %q", want)
			}
		}
	}
	for _, want := range []string{"needs.normalize.outputs.original_run_id", "GH_TOKEN: ${{ github.token }}", "--run-id"} {
		if !strings.Contains(build, want) {
			t.Fatalf("shared recovery acquisition must contain %q", want)
		}
	}
	for _, block := range []string{validate, attest, publish} {
		if !strings.Contains(block, "run-id:") {
			t.Fatal("downstream recovery consumer must select the original artifact run")
		}
	}
	for _, want := range []string{
		"if: needs.normalize.outputs.mode != 'recovery'",
		"Build four binaries, deterministic SBOM, and SHA256SUMS once",
		"Retrieve original retained recovery evidence",
		"recovery-boundary/scripts/acquire-retained-release-candidate.sh",
		"Verify retained recovery bytes against the original candidate",
		"Check out trusted retained recovery adapter",
		"ref: ${{ github.sha }}",
		"path: recovery-boundary",
		"sparse-checkout: |",
		"internal",
		"scripts",
		"persist-credentials: false",
		"recovery-boundary/scripts/verify-retained-release-candidate.sh",
		`--run-id "${{ needs.normalize.outputs.original_run_id }}"`,
		`--verifier "$RUNNER_TEMP/releasecandidate"`,
	} {
		if !strings.Contains(build, want) {
			t.Fatalf("build job must keep recovery away from rebuilding with %q", want)
		}
	}
	if strings.Contains(build, "cp scripts/verify-retained-release-candidate.sh release-metadata/") {
		t.Fatal("trusted retained recovery adapter must not come from retained metadata")
	}
	for _, forbidden := range []string{"scripts/build-release-artifacts.sh", "run-claude-smoke.sh", "releasecandidate create"} {
		recoverySteps := releaseWorkflowStep(t, parseReleaseWorkflow(t, text), "Retrieve original publication metadata for recovery").Text
		if strings.Contains(recoverySteps, forbidden) {
			t.Fatalf("recovery metadata retrieval unexpectedly contains %q", forbidden)
		}
	}
	if !strings.Contains(attest, "if: github.repository == 'yersonargotev/packy' && needs.normalize.outputs.mode == 'fresh'") {
		t.Fatal("the entire privileged attestation job must be fresh-only")
	}
	if !strings.Contains(publish, "needs.attest.result == 'skipped'") || !strings.Contains(publish, "needs.attest.result == 'success'") || !strings.Contains(publish, "always()") {
		t.Fatal("publication must require fresh attestation success or a skipped privileged job for recovery")
	}
	for _, want := range []string{
		"MODE: ${{ needs.normalize.outputs.mode }}",
		`[[ "$MODE" == fresh ]]`,
		"sealed recovery release disappeared before publication",
	} {
		if !strings.Contains(publish, want) {
			t.Fatalf("recovery publication must fail closed when its sealed release disappears with %q", want)
		}
	}
}

func TestReleaseWorkflowSeparatesCurrentMainGovernanceFromTagBuildIdentity(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	normalize := releaseWorkflowJob(t, text, "normalize")
	governance := releaseWorkflowJob(t, text, "governance-drift")
	build := releaseWorkflowJob(t, text, "build")
	for _, want := range []string{"main_commit:", "source_ref:", "release_state:"} {
		if !strings.Contains(normalize, want) {
			t.Fatalf("normalize must separately seal release and governance identity with %q", want)
		}
	}
	for _, want := range []string{
		`ref: ${{ needs.normalize.outputs.main_commit }}`,
		`--commit "${{ needs.normalize.outputs.main_commit }}"`,
	} {
		if !strings.Contains(governance, want) {
			t.Fatalf("governance must run against freshly observed current main with %q", want)
		}
	}
	if !strings.Contains(build, `ref: ${{ needs.normalize.outputs.commit }}`) {
		t.Fatal("release build must remain pinned to the sealed tag commit")
	}
	for _, want := range []string{
		`--arg attestation_source_ref "${{ needs.normalize.outputs.source_ref }}"`,
		`--source-ref "${{ needs.normalize.outputs.source_ref }}"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tag-triggered attestation identity must be sealed and verified with %q", want)
		}
	}
}

func TestReleaseWorkflowIssuesAndVerifiesSealedAttestationBundle(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	workflow := parseReleaseWorkflow(t, text)

	seal := releaseWorkflowStepIndex(t, workflow, "Create immutable candidate and provenance metadata", []string{
		"releasecandidate create",
		"--ref refs/heads/main",
		"--permission attestations=write",
		"--permission contents=write",
		"--permission id-token=write",
	})
	plan := releaseWorkflowStepIndex(t, workflow, "Seal exact publication plan and draft base", []string{
		"formula_sha",
		`repository:"yersonargotev/homebrew-tap"`,
		`path:"Formula/packy.rb"`,
		"sha256:$formula_sha",
	})
	attest := releaseWorkflowStepIndex(t, workflow, "Attest exact retained candidate", []string{
		"actions/attest-build-provenance@977bb373ede98d70efdf65b84cb5f73e068dcc2a",
		"subject-path: 'dist/*'",
	})
	refBeforeAttest := releaseWorkflowStepIndex(t, workflow, "Revalidate refs immediately before OIDC issuance", []string{
		"needs.inspect-release.outputs.has_bundle != 'true'",
		"verify-release-boundary.sh",
		"verify-release-ref-state.sh",
		"--boundary 'OIDC issuance'",
	})
	verify := releaseWorkflowStepIndex(t, workflow, "Verify bundle offline against exact workflow and subjects", []string{
		"gh attestation trusted-root",
		"--bundle \"$bundle\"",
		"--signer-workflow \"$GITHUB_REPOSITORY/.github/workflows/release.yml\"",
		`--source-ref "${{ needs.normalize.outputs.source_ref }}"`,
		"--source-digest \"${{ needs.build.outputs.commit }}\"",
		"--signer-digest \"${{ needs.build.outputs.commit }}\"",
		"--custom-trusted-root",
	})
	envelope := releaseWorkflowStepIndex(t, workflow, "Bind attestation and destination plan into the immutable release set", []string{
		"draft-base.json",
		"attestation.bundle.jsonl",
		"bundle_base64",
		"jq -cnS --slurpfile base",
		"release_set_id",
		"release-body.md",
	})
	publish := releaseWorkflowStepIndex(t, workflow, "Create or verify draft, upload only missing assets, and publish once", []string{
		"gh api graphql",
		"release(tagName:$tag){id}",
		`if [[ -z "$release_id" ]]`,
		"gh release create",
		"--draft",
		"verify-state",
		"--mode draft",
		"gh release upload",
		"assert_server_hashes",
		"gh release edit",
		"--draft=false",
	})

	assertReleaseWorkflowStepBefore(t, seal, plan, "candidate identity must precede the complete destination plan")
	assertReleaseWorkflowStepBefore(t, plan, attest, "the complete destination plan must be sealed before OIDC provenance is issued")
	assertReleaseWorkflowStepBefore(t, refBeforeAttest, attest, "refs must be revalidated immediately before OIDC provenance is issued")
	assertReleaseWorkflowStepBefore(t, attest, verify, "the generated bundle must be verified before publication")
	assertReleaseWorkflowStepBefore(t, verify, envelope, "the verified bundle must be bound into the final release set")
	assertReleaseWorkflowStepBefore(t, envelope, publish, "the complete release set must reach the exact draft before one-time publication")
	for _, forbidden := range []string{"--clobber", "gh release delete", "git tag -f", "git tag -d", "git push origin refs/tags"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("immutable publication workflow must not contain %q", forbidden)
		}
	}
	if strings.Contains(text, "if ! gh release view") || strings.Contains(text, "targetCommitish") {
		t.Fatal("a failed release lookup is ambiguous; absence must be proved by a successful API query")
	}
	publishStep := releaseWorkflowStep(t, workflow, "Create or verify draft, upload only missing assets, and publish once").Text
	firstRefCheck := strings.Index(publishStep, "verify_boundary 'draft creation'")
	create := strings.Index(publishStep, "gh release create")
	finalRefCheck := strings.LastIndex(publishStep, "verify_boundary publication")
	publishEdit := strings.Index(publishStep, "gh release edit")
	if firstRefCheck < 0 || create < 0 || finalRefCheck < 0 || publishEdit < 0 || !(firstRefCheck < create && create < finalRefCheck && finalRefCheck < publishEdit) {
		t.Fatal("tag and protected main must be revalidated immediately before draft creation and publication")
	}
	if got := strings.Count(publishStep, "verify_boundary 'asset upload'"); got < 2 {
		t.Fatalf("every asset upload phase needs the shared boundary adapter; got %d", got)
	}
	for _, want := range []string{
		"verify-release-boundary.sh",
		`--candidate "$candidate"`,
		`--provenance "$provenance"`,
		`--state-output "$RUNNER_TEMP/boundary-state.json"`,
		`--decision-output "$RUNNER_TEMP/boundary-decision.json"`,
		`--expected-body "$expected_body"`,
		`--attestation "$bundle"`,
		`--upload-asset "$name"`,
		`--upload-asset "$(basename "$bundle")"`,
	} {
		if !strings.Contains(publishStep, want) {
			t.Fatalf("publication boundaries must use sealed release-state verification %q", want)
		}
	}
}

func TestReleaseWorkflowKeepsPublishedReleaseImmutable(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	publish := releaseWorkflowStep(
		t,
		parseReleaseWorkflow(t, text),
		"Create or verify draft, upload only missing assets, and publish once",
	).Text

	published := strings.Index(publish, `if jq -e '.isDraft==false'`)
	upload := strings.Index(publish, "gh release upload")
	if published < 0 || upload < 0 {
		t.Fatal("publication step lacks the published-state or draft-upload branch")
	}
	publishedExit := strings.Index(publish[published:], "exit 0")
	if publishedExit < 0 || published+publishedExit >= upload {
		t.Fatal("an exact published release must exit through read-only verification before any upload path")
	}
	if strings.Count(publish, "gh release edit") != 1 ||
		!strings.Contains(publish, `gh release edit "$RELEASE_TAG"`) ||
		!strings.Contains(publish, "--draft=false") {
		t.Fatal("the sole release edit must be the one-way transition from an exact draft to published")
	}
	for _, forbidden := range []string{"--notes-file \"$RUNNER_TEMP", "--clobber", "gh release delete", "git tag -f", "git push --force"} {
		if strings.Contains(publish, forbidden) {
			t.Fatalf("published recovery contains forbidden mutation path %q", forbidden)
		}
	}
}

func TestReleaseWorkflowKeepsDryRunAndDestinationAuthoritySeparate(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	dryRun := releaseWorkflowJob(t, text, "dry-run")
	inspect := releaseWorkflowJob(t, text, "inspect-release")
	attest := releaseWorkflowJob(t, text, "attest")
	github := releaseWorkflowJob(t, text, "publish-github")
	homebrew := releaseWorkflowJob(t, text, "homebrew")
	for _, job := range []string{"build", "claude-smoke", "validate-release-evidence", "dry-run", "inspect-release"} {
		if strings.Contains(releaseWorkflowJob(t, text, job), "environment:") {
			t.Fatalf("read-only release job %s must not wait on a protected publication environment", job)
		}
	}
	if !strings.Contains(attest, "environment: release") || !strings.Contains(github, "environment: release") {
		t.Fatal("attestation and GitHub publication must use the protected release environment")
	}
	if !strings.Contains(homebrew, "environment: homebrew") {
		t.Fatal("Homebrew publication must use the protected homebrew environment")
	}

	for _, want := range []string{
		"if: needs.normalize.outputs.mode == 'dry-run'",
		"RELEASE_EFFECTS: ${{ needs.inspect-release.outputs.effects }}",
		"Exact effects a real run would perform from the observed state",
		"Dry-run stopped before OIDC issuance or any GitHub Release/Homebrew mutation",
	} {
		if !strings.Contains(dryRun, want) {
			t.Fatalf("dry-run job should contain %q", want)
		}
	}
	for _, forbidden := range []string{"id-token: write", "attest-build-provenance", "gh release create", "gh release upload", "git push origin"} {
		if strings.Contains(dryRun, forbidden) {
			t.Fatalf("dry-run job must stop before mutation authority %q", forbidden)
		}
	}
	for _, want := range []string{"Download built candidate for read-only attestation checks", "git clone --quiet --depth 1 --branch main", "plan-homebrew-effects.sh", "homebrew:$homebrew[0]", "gh attestation verify", "--custom-trusted-root", "canonical-body.md", "effects=$effects"} {
		if !strings.Contains(inspect, want) {
			t.Fatalf("read-only inspection should verify an available existing bundle with %q", want)
		}
	}
	if strings.Contains(inspect, "id-token: write") || strings.Contains(inspect, "attest-build-provenance") {
		t.Fatal("read-only existing-bundle verification must not request or issue OIDC")
	}
	for _, want := range []string{"contents: read", "id-token: write", "attestations: write"} {
		if !strings.Contains(attest, want) {
			t.Fatalf("attestation job should have narrow permission %q", want)
		}
	}
	if strings.Contains(attest, "HOMEBREW_TAP_TOKEN") || strings.Contains(attest, "contents: write") {
		t.Fatal("attestation authority must not receive release or Homebrew write authority")
	}
	if !strings.Contains(github, "contents: write") || strings.Contains(github, "id-token: write") || strings.Contains(github, "HOMEBREW_TAP_TOKEN") {
		t.Fatal("GitHub publication must have only its contents write boundary")
	}
	for _, want := range []string{"contents: read", "HOMEBREW_TAP_TOKEN", "repository: yersonargotev/homebrew-tap", "persist-credentials: true"} {
		if !strings.Contains(homebrew, want) {
			t.Fatalf("Homebrew job should contain isolated tap boundary %q", want)
		}
	}
	if strings.Contains(homebrew, "id-token: write") || strings.Contains(homebrew, "contents: write") {
		t.Fatal("Homebrew job must not receive GitHub release or attestation authority")
	}
	if got := strings.Count(homebrew, "HOMEBREW_TAP_TOKEN"); got != 1 {
		t.Fatalf("Homebrew token must appear only in the post-readback tap checkout; got %d references", got)
	}
}

func TestHomebrewEffectPlanIsExactForObservedTapFixtures(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "plan-homebrew-effects.sh")
	formula := filepath.Join(t.TempDir(), "packy.rb")
	if err := os.WriteFile(formula, []byte("class Packy < Formula\nend\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	formulaSHA := sha256.Sum256([]byte("class Packy < Formula\nend\n"))

	makeTap := func(t *testing.T, observedFormula string, legacy bool) string {
		t.Helper()
		tap := t.TempDir()
		if err := os.Mkdir(filepath.Join(tap, "Formula"), 0o700); err != nil {
			t.Fatal(err)
		}
		if observedFormula != "" {
			if err := os.WriteFile(filepath.Join(tap, "Formula", "packy.rb"), []byte(observedFormula), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if legacy {
			if err := os.WriteFile(filepath.Join(tap, "Formula", "matty.rb"), []byte("legacy\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "-qm", "fixture"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = tap
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		return tap
	}

	for _, tc := range []struct {
		name            string
		observedFormula string
		legacy          bool
		action          string
		write           bool
		deletions       int
	}{
		{name: "exact no-op", observedFormula: "class Packy < Formula\nend\n", action: "no-op"},
		{name: "stale formula and legacy deletion", observedFormula: "stale\n", legacy: true, action: "commit-and-push", write: true, deletions: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tap := makeTap(t, tc.observedFormula, tc.legacy)
			cmd := exec.Command("bash", script, "--tap-dir", tap, "--formula", formula, "--repository", "yersonargotev/homebrew-tap", "--ref", "refs/heads/main")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
			output, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			var plan struct {
				Action  string `json:"action"`
				Formula struct {
					SHA256 string `json:"sha256"`
					Write  bool   `json:"write"`
				} `json:"formula"`
				DeletePaths []string `json:"delete_paths"`
			}
			if err := json.Unmarshal(output, &plan); err != nil {
				t.Fatal(err)
			}
			if plan.Action != tc.action || plan.Formula.Write != tc.write || len(plan.DeletePaths) != tc.deletions || plan.Formula.SHA256 != hex.EncodeToString(formulaSHA[:]) {
				t.Fatalf("unexpected plan: %s", output)
			}
		})
	}
}

func TestReleaseEvidenceVerifierRequiresExactCandidateParity(t *testing.T) {
	root := repoRoot(t)
	tag := "v0.99.0"
	commit := gitOutput(t, root, "rev-parse", "HEAD")
	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, asset := range releaseAssets(tag) {
		path := filepath.Join(dist, asset)
		if err := os.WriteFile(path, []byte("candidate "+asset), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sbomSource := filepath.Join(t.TempDir(), release.SBOMName)
	generateSBOM := exec.Command("go", "run", "./internal/tools/releasesbom", "--version", tag, "--created", gitOutput(t, root, "show", "-s", "--format=%cI", commit), "--dist", dist, "--out", sbomSource)
	generateSBOM.Dir = root
	generateSBOM.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	if output, err := generateSBOM.CombinedOutput(); err != nil {
		t.Fatalf("generate SBOM: %v\n%s", err, output)
	}
	sbom, err := os.ReadFile(sbomSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, release.SBOMName), sbom, 0o600); err != nil {
		t.Fatal(err)
	}
	var checksumLines []string
	for _, name := range append(releaseAssets(tag), release.SBOMName) {
		checksumLines = append(checksumLines, sha256File(t, filepath.Join(dist, name))+"  "+name)
	}
	sort.Strings(checksumLines)
	if err := os.WriteFile(filepath.Join(dist, release.ChecksumsName), []byte(strings.Join(checksumLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(t.TempDir(), "metadata")
	formula := filepath.Join(metadata, "packy.rb")
	generate := exec.Command("bash", filepath.Join(root, "scripts", "generate-homebrew-formula.sh"), "--version", tag, "--checksums", filepath.Join(dist, release.ChecksumsName), "--out", formula)
	generate.Dir = root
	generate.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate formula: %v\n%s", err, output)
	}

	evidenceRoot := filepath.Join(t.TempDir(), "evidence")
	if err := os.MkdirAll(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	canonicalLog := filepath.Join(t.TempDir(), "canonical-validator.log")
	fakeGo := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CANONICAL_LOG"
if [ "${1:-}" = run ] && [ "${2:-}" = ./internal/tools/releasesbom ]; then
  out=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --out ]; then out="$2"; break; fi
    shift
  done
  cp "$FAKE_SBOM" "$out"
  exit 0
fi
[ "${FAIL_CANONICAL:-0}" != 1 ]
`
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}

	notes := filepath.Join(metadata, "release-notes.md")
	run := func(failCanonical bool) ([]byte, error) {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "verify-release-evidence.sh"),
			"--tag", tag, "--commit", commit, "--dist", dist, "--evidence-root", evidenceRoot,
			"--formula", formula, "--notes-template", filepath.Join(root, "docs", "release-notes", "next.md"), "--notes-output", notes,
			"--repository", "yersonargotev/packy", "--workflow", ".github/workflows/release.yml",
			"--workflow-digest", strings.Repeat("c", 64), "--run-id", "release-run")
		cmd.Dir = root
		fail := "0"
		if failCanonical {
			fail = "1"
		}
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "CANONICAL_LOG="+canonicalLog, "FAKE_SBOM="+filepath.Join(dist, release.SBOMName), "FAIL_CANONICAL="+fail)
		return cmd.CombinedOutput()
	}
	if output, err := run(false); err != nil {
		t.Fatalf("verify complete candidate: %v\n%s", err, output)
	}
	if rendered, err := os.ReadFile(notes); err != nil || !strings.Contains(string(rendered), "# "+tag) || strings.Contains(string(rendered), "{{TAG}}") {
		t.Fatalf("rendered notes are not tag-bound: %v\n%s", err, rendered)
	}
	invocation, err := os.ReadFile(canonicalLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run ./internal/tools/claudesmoke verify-release", "run ./internal/tools/claudesmoke verify-addy-release", "--evidence-root " + evidenceRoot, "--packy-version " + tag, "--packy-sha " + commit, "--repository yersonargotev/packy", "--workflow .github/workflows/release.yml", "--workflow-digest " + strings.Repeat("c", 64), "--run-id release-run"} {
		if !strings.Contains(string(invocation), want) {
			t.Fatalf("release verifier did not delegate %q to the canonical owner:\n%s", want, invocation)
		}
	}
	unexpectedDir := filepath.Join(dist, "unexpected-directory")
	if err := os.Mkdir(unexpectedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected directory: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedDir); err != nil {
		t.Fatal(err)
	}
	unexpectedLink := filepath.Join(dist, "unexpected-link")
	if err := os.Symlink(filepath.Join(dist, releaseAssets(tag)[0]), unexpectedLink); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected symlink: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedLink); err != nil {
		t.Fatal(err)
	}
	unexpectedFIFO := filepath.Join(dist, "unexpected-fifo")
	if err := unix.Mkfifo(unexpectedFIFO, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected FIFO: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedFIFO); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dist, releaseAssets(tag)[0])
	victimBytes, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dist, releaseAssets(tag)[1]), victim); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "not a regular non-symlink") {
		t.Fatalf("release verifier accepted expected-name symlink: %v\n%s", err, output)
	}
	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, victimBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := run(true); err == nil {
		t.Fatalf("release verifier ignored canonical evidence rejection:\n%s", output)
	}
}

func TestReleaseWorkflowVerifiesPublishedGitHubBytesBeforeHomebrew(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	homebrew := releaseWorkflowJob(t, text, "homebrew")
	for _, want := range []string{
		"needs: [normalize, build, validate-release-evidence, publish-github]",
		"needs.publish-github.outputs.published == 'true'",
		"Independently read back exact published GitHub assets",
		"verify-release-boundary.sh",
		"verify-release-ref-state.sh",
		"cmp attestation/release-body.md",
		"attestation.bundle.jsonl",
		"cmp \"$RUNNER_TEMP/expected-assets\" \"$RUNNER_TEMP/actual-assets\"",
		"sha256sum --check SHA256SUMS",
		"publication_plan.homebrew.sha256",
		"--boundary 'Homebrew mutation'",
		"--state-output \"$RUNNER_TEMP/boundary-state.json\"",
		"--decision-output \"$RUNNER_TEMP/boundary-decision.json\"",
		"--expected-body ../attestation/release-body.md",
		"--attestation ../attestation/attestation.bundle.jsonl --mode published",
		"tap remote formula does not match the sealed destination plan",
		"git push --dry-run origin HEAD:main",
		"git push origin HEAD:main",
	} {
		if !strings.Contains(homebrew, want) {
			t.Fatalf("Homebrew publication should contain %q", want)
		}
	}
	readBack := strings.Index(homebrew, "Independently read back exact published GitHub assets")
	checkout := strings.Index(homebrew, "Check out Homebrew tap with only its scoped credential")
	push := strings.Index(homebrew, "git push origin HEAD:main")
	if readBack < 0 || checkout < 0 || push < 0 || !(readBack < checkout && checkout < push) {
		t.Fatal("published GitHub bytes must be independently verified before tap checkout and push")
	}
	for _, forbidden := range []string{"formula_renames.json", "FormulaRenames", "yersonargotev/matty", "Formula/matty.rb =>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("release workflow must not contain legacy distribution identity %q", forbidden)
		}
	}
}

func TestReleaseWorkflowPublishesAuditableFinalSummary(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	summary := releaseWorkflowJob(t, text, "release-summary")
	for _, want := range []string{
		"needs: [normalize, build, validate-release-evidence, inspect-release, publish-github, homebrew]",
		"Link exact published evidence",
		"needs.normalize.outputs.tag",
		"needs.normalize.outputs.commit",
		"needs.homebrew.outputs.tap_commit",
		"GitHub Release:",
		"Attestation evidence:",
		"Homebrew tap commit:",
		"published bytes, checksums, attestation bundle, release envelope, and tap formula",
		"GITHUB_STEP_SUMMARY",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("final release summary should contain %q", want)
		}
	}
}

func TestBuildReleaseArtifactsCreatesChecksummedSupportedPlatforms(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles release artifacts")
	}

	root := repoRoot(t)
	outDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-release-artifacts.sh"), "--version", "v0.1.7", "--out-dir", outDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "GOCACHE="+t.TempDir(), "GOMODCACHE="+goEnv(t, "GOMODCACHE"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release artifacts: %v\n%s", err, output)
	}

	wantAssets := releaseAssets("v0.1.7")
	wantEntries := append(append([]string{}, wantAssets...), release.ChecksumsName, release.SBOMName)
	sort.Strings(wantEntries)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotEntries []string
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("release output contains unexpected directory %s", entry.Name())
		}
		gotEntries = append(gotEntries, entry.Name())
	}
	sort.Strings(gotEntries)
	if strings.Join(gotEntries, "\n") != strings.Join(wantEntries, "\n") {
		t.Fatalf("v0.1.7 release directory mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantEntries, "\n"), strings.Join(gotEntries, "\n"))
	}

	checksums := readChecksums(t, filepath.Join(outDir, release.ChecksumsName))
	for _, asset := range wantAssets {
		gotChecksum, ok := checksums[asset]
		if !ok {
			t.Fatalf("SHA256SUMS missing checksum for %s", asset)
		}
		if gotChecksum != sha256File(t, filepath.Join(outDir, asset)) {
			t.Fatalf("checksum for %s does not match artifact bytes", asset)
		}
	}
	if got := checksums[release.SBOMName]; got != sha256File(t, filepath.Join(outDir, release.SBOMName)) {
		t.Fatalf("SHA256SUMS does not bind the SBOM: %q", got)
	}
	if len(checksums) != len(wantAssets)+1 {
		t.Fatalf("SHA256SUMS should contain exactly binaries and SBOM; got %d entries", len(checksums))
	}
}

func TestBuildReleaseArtifactsValidatesReleaseVersionBeforeBuilding(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-release-artifacts.sh")

	t.Run("accepts v0 x y release tags", func(t *testing.T) {
		for _, version := range []string{"v0.1.0", "v0.2.3", "v0.99.0"} {
			t.Run(version, func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("build release artifacts should accept %s: %v\n%s", version, err, output)
				}
				if _, err := os.Stat(logPath); err != nil {
					t.Fatalf("expected accepted version %s to reach go build: %v", version, err)
				}
				log, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("read go build log: %v", err)
				}
				wantLdflag := "-X github.com/yersonargotev/packy/internal/version.Value=" + version
				if !strings.Contains(string(log), wantLdflag) {
					t.Fatalf("release build should inject version with ldflags %q\nlog:\n%s", wantLdflag, log)
				}
			})
		}
	})

	t.Run("rejects non-v0 and malformed versions before building", func(t *testing.T) {
		for _, version := range []string{"v1.0.0", "v0.2", "v0.1.0-rc.1", "0.1.0", "main", ""} {
			t.Run(fmt.Sprintf("%q", version), func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())

				output, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("build release artifacts should reject version %q\n%s", version, output)
				}
				if !strings.Contains(string(output), "Release version must be a v0.x.y tag") {
					t.Fatalf("rejection should explain v0.x.y requirement, got:\n%s", output)
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatalf("invalid version %q should fail before go build; stat error: %v", version, err)
				}
			})
		}
	})
}

func TestGenerateHomebrewFormulaUsesChecksummedReleaseArtifacts(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := writeChecksumManifest(t, validFormulaChecksumLines("v0.99.0"))
	outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
		"--repo", "yersonargotev/packy",
		"--homepage", "https://github.com/yersonargotev/packy",
		"--desc", "AI coding workflow installer",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate formula: %v\n%s", err, output)
	}

	formula, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(formula)
	for _, want := range []string{
		"class Packy < Formula",
		`desc "AI coding workflow installer"`,
		`homepage "https://github.com/yersonargotev/packy"`,
		`version "0.99.0"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_darwin_amd64", using: :nounzip`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_darwin_arm64", using: :nounzip`,
		`sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_linux_amd64", using: :nounzip`,
		`sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_linux_arm64", using: :nounzip`,
		`sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`,
		`downloaded_binary = Dir["packy_*"].first`,
		`odie "downloaded packy binary not found" if downloaded_binary.nil?`,
		`bin.install downloaded_binary => "packy"`,
		`system "#{bin}/packy", "--version"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formula should contain %q\nformula:\n%s", want, text)
		}
	}
	if got := strings.Count(text, "using: :nounzip"); got != len(supportedReleasePlatforms) {
		t.Fatalf("formula should mark every raw executable URL as using: :nounzip; got %d occurrences in:\n%s", got, text)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumEntryIsMissing(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := writeChecksumManifest(t, validFormulaChecksumLines("v0.99.0")[:3])
	outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("generate formula should fail when a checksum entry is missing\n%s", output)
	}
	if !strings.Contains(string(output), "missing checksum entry for packy_v0.99.0_linux_arm64") {
		t.Fatalf("failure should name the missing artifact, got:\n%s", output)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("formula should not be written with incomplete checksums; stat error: %v", err)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumManifestIsNotExact(t *testing.T) {
	root := repoRoot(t)
	baseChecksums := validFormulaChecksumLines("v0.99.0")

	tests := []struct {
		name      string
		extraLine string
		wantError string
	}{
		{
			name:      "rejects unexpected release artifact",
			extraLine: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee  packy_v0.99.0_linux_386",
			wantError: "unexpected checksum entry for packy_v0.99.0_linux_386",
		},
		{
			name:      "rejects duplicate expected artifact",
			extraLine: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  packy_v0.99.0_darwin_amd64",
			wantError: "duplicate checksum entry for packy_v0.99.0_darwin_amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksums := append(append([]string{}, baseChecksums...), tt.extraLine)
			checksumsPath := writeChecksumManifest(t, checksums)
			outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

			cmd := exec.Command(
				"bash",
				filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
				"--version", "v0.99.0",
				"--checksums", checksumsPath,
				"--out", outputPath,
			)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("generate formula should fail when the checksum manifest is not exact\n%s", output)
			}
			if !strings.Contains(string(output), tt.wantError) {
				t.Fatalf("failure should explain the manifest mismatch, got:\n%s", output)
			}
			if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
				t.Fatalf("formula should not be written with invalid checksum manifest; stat error: %v", err)
			}
		})
	}
}

func readReleaseWorkflow(t *testing.T, root string) string {
	t.Helper()
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(workflow)
}

type releaseEventFixture struct {
	eventName        string
	eventRef         string
	eventRefType     string
	eventRefName     string
	eventSHA         string
	inputTag         string
	inputDryRun      string
	tag              string
	tagCommit        string
	mainCommit       string
	ancestor         bool
	existingTags     string
	existingReleases string
	releaseID        string
	releaseState     string
	originalRunID    string
}

func baseReleaseEventFixture() releaseEventFixture {
	commit := strings.Repeat("a", 40)
	return releaseEventFixture{
		eventName:        "push",
		eventRef:         "refs/tags/v0.2.0",
		eventRefType:     "tag",
		eventRefName:     "v0.2.0",
		eventSHA:         commit,
		tag:              "v0.2.0",
		tagCommit:        commit,
		mainCommit:       commit,
		ancestor:         true,
		existingTags:     "v0.1.0\nv0.2.0",
		existingReleases: "v0.1.0",
	}
}

func runReleaseEventFixture(t *testing.T, workflow string, fixture releaseEventFixture) (string, error) {
	t.Helper()
	if !strings.Contains(workflow, "run: ./scripts/normalize-release-event.sh") {
		t.Fatal("release workflow does not invoke the checked-in event normalizer")
	}
	result := runReleaseScenario(t, releaseScenario{Event: fixture})
	if result.err != nil {
		return strings.Join(result.Diagnostics, "\n"), result.err
	}
	return result.GitHubOutput, nil
}

func assertReleaseEventOutput(t *testing.T, output, mode, tag, commit string) {
	t.Helper()
	for _, want := range []string{"mode=" + mode, "tag=" + tag, "commit=" + commit} {
		if !strings.Contains(output, want+"\n") {
			t.Fatalf("release event output missing %q:\n%s", want, output)
		}
	}
}

func releaseWorkflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	marker := "\n  " + name + ":\n"
	start := strings.Index(workflow, marker)
	if start < 0 {
		t.Fatalf("release workflow missing job %q", name)
	}
	start++
	end := len(workflow)
	for offset, line := range strings.Split(workflow[start:], "\n") {
		if offset > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			candidate := strings.Index(workflow[start:], "\n"+line+"\n")
			if candidate >= 0 {
				end = start + candidate
				break
			}
		}
	}
	return workflow[start:end]
}

type workflowStep struct {
	Name  string
	Index int
	Text  string
}

func parseReleaseWorkflow(t *testing.T, workflow string) []workflowStep {
	t.Helper()
	const stepMarker = "\n      - name: "
	var steps []workflowStep
	searchFrom := 0
	for {
		markerIndex := strings.Index(workflow[searchFrom:], stepMarker)
		if markerIndex < 0 {
			break
		}
		stepStart := searchFrom + markerIndex + 1
		nameStart := stepStart + len("      - name: ")
		nameEnd := strings.IndexByte(workflow[nameStart:], '\n')
		if nameEnd < 0 {
			t.Fatalf("release workflow step at byte %d is missing a newline after its name", stepStart)
		}
		nameEnd += nameStart
		nextMarker := strings.Index(workflow[nameEnd:], stepMarker)
		stepEnd := len(workflow)
		if nextMarker >= 0 {
			stepEnd = nameEnd + nextMarker
		}
		steps = append(steps, workflowStep{
			Name:  strings.TrimSpace(workflow[nameStart:nameEnd]),
			Index: stepStart,
			Text:  workflow[stepStart:stepEnd],
		})
		searchFrom = stepEnd
	}
	if len(steps) == 0 {
		t.Fatal("release workflow contains no job steps")
	}
	return steps
}

func releaseWorkflowStepIndex(t *testing.T, workflow []workflowStep, name string, requiredFragments []string) int {
	t.Helper()
	step := releaseWorkflowStep(t, workflow, name)
	for _, fragment := range requiredFragments {
		if !strings.Contains(step.Text, fragment) {
			t.Fatalf("release workflow step %q should contain %q\nstep:\n%s", name, fragment, step.Text)
		}
	}
	return step.Index
}

func releaseWorkflowStep(t *testing.T, workflow []workflowStep, name string) workflowStep {
	t.Helper()
	for _, step := range workflow {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("release workflow missing step %q", name)
	return workflowStep{}
}

func assertReleaseWorkflowStepBefore(t *testing.T, earlier, later int, reason string) {
	t.Helper()
	if earlier < 0 || later < 0 {
		t.Fatalf("cannot compare missing workflow steps: earlier=%d later=%d", earlier, later)
	}
	if earlier >= later {
		t.Fatalf("release workflow ordering violation: %s", reason)
	}
}

func validFormulaChecksumLines(version string) []string {
	return []string{
		fmt.Sprintf("%s  packy_%s_darwin_amd64", strings.Repeat("a", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_darwin_arm64", strings.Repeat("b", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_linux_amd64", strings.Repeat("c", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_linux_arm64", strings.Repeat("d", sha256.Size*2), version),
		fmt.Sprintf("%s  %s", strings.Repeat("e", sha256.Size*2), release.SBOMName),
	}
}

func writeChecksumManifest(t *testing.T, lines []string) string {
	t.Helper()
	checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
	checksums := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	return checksumsPath
}

func fakeGoBuild(t *testing.T) (string, string) {
	t.Helper()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-build.log")
	goPath := filepath.Join(dir, "go")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> %q
if [[ "${1:-}" == "run" ]]; then
  exec %q "$@"
fi
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    out="${2:-}"
    break
  fi
  shift
done
if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf 'fake binary for %%s\n' "$(basename "$out")" > "$out"
fi
`, logPath, realGo)
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logPath
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}

func releaseAssets(version string) []string {
	assets := make([]string, 0, len(supportedReleasePlatforms))
	for _, platform := range supportedReleasePlatforms {
		parts := strings.Split(platform, "/")
		assets = append(assets, fmt.Sprintf("packy_%s_%s_%s", version, parts[0], parts[1]))
	}
	sort.Strings(assets)
	return assets
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env %s: %v", key, err)
	}
	return strings.TrimSpace(string(output))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("checksum line should be '<sha256>  <asset>', got %q", scanner.Text())
		}
		if len(fields[0]) != sha256.Size*2 {
			t.Fatalf("checksum for %s should be SHA-256 hex, got %q", fields[1], fields[0])
		}
		if strings.Contains(fields[1], string(os.PathSeparator)) {
			t.Fatalf("checksum entry should use asset filename only, got %q", fields[1])
		}
		if _, ok := checksums[fields[1]]; ok {
			t.Fatalf("duplicate checksum entry for %s", fields[1])
		}
		checksums[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return checksums
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
