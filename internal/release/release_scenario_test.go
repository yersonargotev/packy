package release_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

const (
	releaseScenarioNormalizeStage releaseScenarioStage = "normalize-and-admit"
	releaseScenarioRefStage       releaseScenarioStage = "verify-ref-state"
)

type releaseScenarioStage string

type releaseScenario struct {
	Event releaseEventFixture
}

type releaseScenarioStageResult struct {
	Stage      releaseScenarioStage
	ExitStatus int
}

type releaseScenarioEffect struct {
	Tool string
	Args string
}

type releaseScenarioResult struct {
	ExitStatus   int
	Admission    *release.Admission
	VerifiedRefs *release.VerifiedRefState
	Diagnostics  []string
	Effects      []releaseScenarioEffect
	Stages       []releaseScenarioStageResult
	GitHubOutput string
	SandboxRoot  string
	WritableRoot map[string]string
	err          error
}

type releaseProcessBoundary struct {
	Name        string
	ReleaseMode string
}

type releaseProcessFixture struct {
	Name            string
	Boundary        string
	ReleaseMode     string
	StartAtBoundary bool
	Inject          func(*releaseProcessObservation)
}

type releaseProcessObservation struct {
	TagCommit  string
	MainCommit string
	Ancestor   bool
	Release    *release.Release
}

type releaseProcessResult struct {
	Admission    *release.Admission
	Effects      []string
	Observations []releaseScenarioEffect
	Diagnostics  []string
	err          error
}

var releaseCandidateBuild struct {
	sync.Once
	root string
	path string
	err  error
}

func TestMain(main *testing.M) {
	code := main.Run()
	if releaseCandidateBuild.root != "" {
		if err := os.RemoveAll(releaseCandidateBuild.root); err != nil && code == 0 {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestReleaseScenarioValidTagPush(t *testing.T) {
	result := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})

	if result.err != nil {
		t.Fatalf("valid release scenario failed: %v\n%s", result.err, strings.Join(result.Diagnostics, "\n"))
	}
	if result.ExitStatus != 0 {
		t.Fatalf("exit status = %d, want 0", result.ExitStatus)
	}
	commit := strings.Repeat("a", 40)
	wantAdmission := &release.Admission{
		Mode:                 release.AdmissionFresh,
		Tag:                  "v0.2.0",
		ReleaseCommit:        commit,
		CurrentMain:          commit,
		AttestationSourceRef: "refs/tags/v0.2.0",
		ReleaseState:         "absent",
	}
	wantVerified := &release.VerifiedRefState{
		Verified:      true,
		Tag:           "v0.2.0",
		ReleaseCommit: commit,
		CurrentMain:   commit,
	}
	if !reflect.DeepEqual(result.Admission, wantAdmission) {
		t.Fatalf("admission = %#v, want %#v", result.Admission, wantAdmission)
	}
	if !reflect.DeepEqual(result.VerifiedRefs, wantVerified) {
		t.Fatalf("verified refs = %#v, want %#v", result.VerifiedRefs, wantVerified)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	wantStages := []releaseScenarioStageResult{
		{Stage: releaseScenarioNormalizeStage, ExitStatus: 0},
		{Stage: releaseScenarioRefStage, ExitStatus: 0},
	}
	if !reflect.DeepEqual(result.Stages, wantStages) {
		t.Fatalf("stages = %#v, want %#v", result.Stages, wantStages)
	}
	if len(result.Effects) == 0 {
		t.Fatal("valid scenario recorded no attempted effects")
	}
	assertNoReleaseScenarioRemoteMutation(t, result.Effects)
}

func TestReleaseScenarioRejectsMalformedEventBeforeAdmission(t *testing.T) {
	event := baseReleaseEventFixture()
	event.eventRef = "refs/heads/main"
	event.eventRefType = "branch"
	event.eventRefName = "main"

	result := runReleaseScenario(t, releaseScenario{Event: event})

	if result.err == nil || result.ExitStatus == 0 {
		t.Fatalf("malformed event result = %#v, want a nonzero normalization failure", result)
	}
	if result.Admission != nil || result.VerifiedRefs != nil {
		t.Fatalf("malformed event sealed later identities: admission=%#v refs=%#v", result.Admission, result.VerifiedRefs)
	}
	if len(result.Stages) != 1 || result.Stages[0].Stage != releaseScenarioNormalizeStage {
		t.Fatalf("malformed stages = %#v, want only normalization", result.Stages)
	}
	if len(result.Effects) != 0 {
		t.Fatalf("malformed event attempted later effects: %#v", result.Effects)
	}
	if !containsDiagnostic(result.Diagnostics, "ambiguous tag-push event payload") {
		t.Fatalf("diagnostics = %#v, want ambiguous tag-push failure", result.Diagnostics)
	}
}

func TestReleaseScenarioUsesDisposableRoots(t *testing.T) {
	result := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})
	if result.err != nil {
		t.Fatal(result.err)
	}
	wantNames := []string{
		"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME",
		"GOCACHE", "GOMODCACHE", "GOPATH", "TMPDIR", "RUNNER_TEMP", "GITHUB_OUTPUT",
	}
	for _, name := range wantNames {
		path := result.WritableRoot[name]
		if path == "" || !pathWithinRoot(result.SandboxRoot, path) {
			t.Fatalf("%s = %q, want a path beneath %q", name, path, result.SandboxRoot)
		}
	}
}

func TestReleaseScenarioUsesFakeExternalBoundaries(t *testing.T) {
	result := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})
	if result.err != nil {
		t.Fatal(result.err)
	}
	seen := map[string]bool{}
	for _, effect := range result.Effects {
		seen[effect.Tool] = true
		switch effect.Tool {
		case "git", "gh", "releasecandidate":
		default:
			t.Fatalf("unexpected external boundary %#v", effect)
		}
	}
	for _, tool := range []string{"git", "gh", "releasecandidate"} {
		if !seen[tool] {
			t.Fatalf("scenario did not exercise fake %s boundary: %#v", tool, result.Effects)
		}
	}
}

func TestReleaseScenarioFakeGitHubRejectsGraphQLMutation(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReleaseScenarioFakes(t, fakeBin)
	command := exec.Command(filepath.Join(fakeBin, "gh"),
		"api", "graphql",
		"-f", "query=mutation { deleteRelease(input:{}) { clientMutationId } }",
	)
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"FIXTURE_EFFECT_LOG=" + filepath.Join(root, "effects.log"),
	}
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("GraphQL mutation unexpectedly crossed the fake boundary:\n%s", output)
	}
}

func TestReleaseScenarioResultIsDeterministicAndMutationSensitive(t *testing.T) {
	first := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})
	second := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})
	if first.err != nil || second.err != nil {
		t.Fatalf("repeat failed: first=%v second=%v", first.err, second.err)
	}
	if !reflect.DeepEqual(stableReleaseScenarioResult(first), stableReleaseScenarioResult(second)) {
		t.Fatalf("repeated scenario changed result:\nfirst=%#v\nsecond=%#v", first, second)
	}

	mutatedEvent := baseReleaseEventFixture()
	mutatedCommit := strings.Repeat("b", 40)
	mutatedEvent.eventSHA = mutatedCommit
	mutatedEvent.tagCommit = mutatedCommit
	mutatedEvent.mainCommit = mutatedCommit
	mutated := runReleaseScenario(t, releaseScenario{Event: mutatedEvent})
	if mutated.err != nil {
		t.Fatalf("mutated valid scenario failed: %v", mutated.err)
	}
	if reflect.DeepEqual(first.Admission, mutated.Admission) ||
		reflect.DeepEqual(first.VerifiedRefs, mutated.VerifiedRefs) {
		t.Fatalf("sealed identities ignored commit mutation: first=%#v mutated=%#v", first, mutated)
	}
	if mutated.Admission.ReleaseCommit != mutatedCommit || mutated.VerifiedRefs.ReleaseCommit != mutatedCommit {
		t.Fatalf("mutated identities did not seal %s: admission=%#v refs=%#v", mutatedCommit, mutated.Admission, mutated.VerifiedRefs)
	}
}

func TestReleaseScenarioRejectsIdentityDriftAtEveryPrivilegedBoundary(t *testing.T) {
	commit := strings.Repeat("c", 40)
	moved := strings.Repeat("e", 40)
	boundaries := releaseProcessBoundaries()
	var fixtures []releaseProcessFixture
	for _, boundary := range boundaries {
		boundary := boundary
		fixtures = append(fixtures,
			releaseProcessFixture{
				Name: "tag movement", Boundary: boundary.Name,
				Inject: func(observed *releaseProcessObservation) {
					observed.TagCommit = moved
				},
			},
			releaseProcessFixture{
				Name: "loss of protected-main ancestry", Boundary: boundary.Name,
				Inject: func(observed *releaseProcessObservation) {
					observed.Ancestor = false
				},
			},
		)
		if boundary.ReleaseMode != "" {
			fixtures = append(fixtures,
				releaseProcessFixture{
					Name: "mismatched release target", Boundary: boundary.Name,
					Inject: func(observed *releaseProcessObservation) {
						observed.Release.TargetCommit = moved
					},
				},
				releaseProcessFixture{
					Name: "missing release", Boundary: boundary.Name,
					Inject: func(observed *releaseProcessObservation) {
						observed.Release = nil
					},
				},
				releaseProcessFixture{
					Name: "divergent sealed release", Boundary: boundary.Name,
					Inject: func(observed *releaseProcessObservation) {
						observed.Release.CandidateID = strings.Repeat("f", 64)
					},
				},
			)
		}
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Boundary+"/"+fixture.Name, func(t *testing.T) {
			result := runReleaseProcessScenario(t, fixture)
			if result.err == nil {
				t.Fatalf("%s at %s was accepted", fixture.Name, fixture.Boundary)
			}
			diagnostic := strings.Join(result.Diagnostics, "\n")
			for _, want := range []string{fixture.Boundary, "expected", commit, "observed"} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("diagnostic %q does not name %q", diagnostic, want)
				}
			}
			boundaryIndex := releaseProcessBoundaryIndex(t, fixture.Boundary)
			if len(result.Effects) != boundaryIndex {
				t.Fatalf("effects = %#v, want exactly the %d effects before %s", result.Effects, boundaryIndex, fixture.Boundary)
			}
			for index, effect := range result.Effects {
				if effect != boundaries[index].Name {
					t.Fatalf("effect %d = %q, want %q", index, effect, boundaries[index].Name)
				}
			}
		})
	}
}

func TestReleaseScenarioManualRecoveryConsumesOnlyOriginalRetainedEvidence(t *testing.T) {
	event := baseReleaseEventFixture()
	event.tag = "v0.1.2"
	event.tagCommit = strings.Repeat("c", 40)
	event.mainCommit = event.tagCommit
	event.eventName = "workflow_dispatch"
	event.eventRef, event.eventRefType, event.eventRefName = "refs/heads/main", "branch", "main"
	event.eventSHA, event.inputTag, event.inputDryRun = event.mainCommit, event.tag, "false"
	event.releaseID, event.releaseState, event.originalRunID = "R_release", "draft", "12345"

	result := runRetainedRecoveryVerification(t, event.tag, event.tagCommit, event.originalRunID, "", nil)
	if result.err != nil {
		t.Fatalf("retained recovery verification failed: %v\n%s", result.err, strings.Join(result.Diagnostics, "\n"))
	}
	if result.Admission == nil || result.Admission.Mode != release.AdmissionRecovery || result.Admission.OriginalRunID != event.originalRunID {
		t.Fatalf("integrated recovery admission = %#v, want original run %s", result.Admission, event.originalRunID)
	}
	for _, effect := range result.Effects[:len(result.Effects)-1] {
		if strings.Contains(effect, "build") || strings.Contains(effect, "attest") || strings.Contains(effect, "sign") ||
			strings.Contains(effect, "release create") || strings.Contains(effect, "release upload") {
			t.Fatalf("recovery verification attempted a forbidden effect: %q", effect)
		}
	}
	wantEffects := []string{
		"gh run download 12345 --repo yersonargotev/packy --name packy-release-v0.1.2 --dir " + filepath.Join(resultRoot(result), "dist"),
		"gh run download 12345 --repo yersonargotev/packy --name packy-release-metadata-v0.1.2 --dir " + filepath.Join(resultRoot(result), "metadata"),
	}
	if len(result.Effects) < 8 || !reflect.DeepEqual(result.Effects[1:3], wantEffects) ||
		!strings.HasPrefix(result.Effects[0], "releasecandidate admit ") ||
		!strings.HasPrefix(result.Effects[3], "releasecandidate verify-recovery ") ||
		!strings.HasPrefix(result.Effects[len(result.Effects)-1], "gh release edit ") {
		t.Fatalf("integrated effects = %#v, want admission, original-run downloads, domain verification, boundary observations, and continuation (downloads %#v)", result.Effects, wantEffects)
	}
}

func TestReleaseScenarioRejectsUnavailableOrDivergentRetainedEvidenceBeforePrivilege(t *testing.T) {
	for _, test := range []struct {
		name            string
		downloadFailure string
		diagnostic      string
		effectCount     int
		mutate          func(string, string)
	}{
		{name: "missing candidate artifact", downloadFailure: "missing-candidate", diagnostic: "candidate artifact not found", effectCount: 2},
		{name: "expired metadata artifact", downloadFailure: "expired-metadata", diagnostic: "metadata artifact expired", effectCount: 3},
		{name: "divergent bytes", diagnostic: "retained candidate subject set or digest diverges", effectCount: 4, mutate: func(dist, _ string) {
			entries, err := os.ReadDir(dist)
			if err != nil || len(entries) == 0 {
				t.Fatalf("read retained fixture: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dist, entries[0].Name()), []byte("divergent retained bytes\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "divergent metadata", diagnostic: "retained publication plan diverges", effectCount: 4, mutate: func(_, metadata string) {
			path := filepath.Join(metadata, "publication-plan.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.Replace(string(data), `"source_run_id":"12345"`, `"source_run_id":"99999"`, 1))
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := runRetainedRecoveryVerification(t, "v0.1.2", strings.Repeat("c", 40), "12345", test.downloadFailure, test.mutate)
			if result.err == nil {
				t.Fatalf("%s unexpectedly passed", test.name)
			}
			if !containsDiagnostic(result.Diagnostics, test.diagnostic) || len(result.Effects) != test.effectCount {
				t.Fatalf("denial diagnostics/effects = %#v / %#v, want %q and %d read-only effects", result.Diagnostics, result.Effects, test.diagnostic, test.effectCount)
			}
			for _, effect := range result.Effects {
				if !strings.HasPrefix(effect, "gh run download ") && !strings.HasPrefix(effect, "releasecandidate admit ") && !strings.HasPrefix(effect, "releasecandidate verify-recovery ") {
					t.Fatalf("denial crossed a privileged boundary: %#v", result.Effects)
				}
			}
		})
	}
}

func TestReleaseScenarioDistinguishesFreshPublicationFromSafeResumeAndCannotRecreateDisappearedRelease(t *testing.T) {
	fresh := runReleaseScenario(t, releaseScenario{Event: baseReleaseEventFixture()})
	if fresh.err != nil || fresh.Admission.Mode != release.AdmissionFresh || fresh.Admission.OriginalRunID != "" {
		t.Fatalf("fresh admission = %#v, err=%v", fresh.Admission, fresh.err)
	}
	recovery := baseReleaseEventFixture()
	recovery.eventName = "workflow_dispatch"
	recovery.eventRef, recovery.eventRefType, recovery.eventRefName = "refs/heads/main", "branch", "main"
	recovery.eventSHA, recovery.inputTag, recovery.inputDryRun = recovery.mainCommit, recovery.tag, "false"
	recovery.releaseID, recovery.releaseState, recovery.originalRunID = "R_release", "draft", "12345"
	resumed := runReleaseScenario(t, releaseScenario{Event: recovery})
	if resumed.err != nil || resumed.Admission.Mode != release.AdmissionRecovery || resumed.Admission.OriginalRunID != "12345" {
		t.Fatalf("recovery admission = %#v, err=%v", resumed.Admission, resumed.err)
	}
	recovery.releaseID, recovery.releaseState, recovery.originalRunID = "", "", ""
	disappeared := runReleaseScenario(t, releaseScenario{Event: recovery})
	if disappeared.err == nil {
		t.Fatal("manual recovery recreated a disappeared sealed release")
	}
	assertNoReleaseScenarioRemoteMutation(t, disappeared.Effects)

	continuation := runRetainedRecoveryVerification(t, "v0.1.2", strings.Repeat("c", 40), "12345", "disappeared-release", nil)
	if continuation.err == nil || !containsDiagnostic(continuation.Diagnostics, "publication denied") ||
		!containsDiagnostic(continuation.Diagnostics, "expected") || !containsDiagnostic(continuation.Diagnostics, "observed") {
		t.Fatalf("post-admission disappearance = %#v, want fail-closed continuation", continuation)
	}
	for _, command := range continuation.Effects {
		for _, forbidden := range []string{"gh release create", "gh release upload", "gh release edit", "oidc ", "build", "attest", "sign", "git push"} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("post-admission disappearance attempted %q: %#v", forbidden, continuation.Effects)
			}
		}
	}
}

func runRetainedRecoveryVerification(t *testing.T, tag, commit, runID, downloadFailure string, mutate func(string, string)) releaseProcessResult {
	t.Helper()
	root := t.TempDir()
	dist, metadata := filepath.Join(root, "dist"), filepath.Join(root, "metadata")
	retainedDist, retainedMetadata := filepath.Join(root, "retained-dist"), filepath.Join(root, "retained-metadata")
	for _, path := range []string{retainedDist, retainedMetadata, filepath.Join(root, "bin"), filepath.Join(root, "home"), filepath.Join(root, "config"), filepath.Join(root, "cache"), filepath.Join(root, "state"), filepath.Join(root, "tmp")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	binary := []byte("original retained candidate bytes\n")
	observed := fixtureObservation()
	for index := range observed.Subjects {
		if strings.HasPrefix(observed.Subjects[index].Name, "packy_") {
			observed.Subjects[index].SHA256 = digest(binary)
		}
	}
	replaceSBOM(&observed, []byte(strings.ReplaceAll(string(observed.SBOM), strings.Repeat("b", 64), digest(binary))))
	candidate := mustCandidate(t, observed)
	for _, subject := range candidate.Subjects {
		var content []byte
		switch subject.Name {
		case release.SBOMName:
			content = observed.SBOM
		case release.ChecksumsName:
			content = observed.SHA256SUMS
		default:
			content = binary
		}
		if err := os.WriteFile(filepath.Join(retainedDist, subject.Name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeReleaseProcessJSON(t, retainedMetadata, "candidate.json", candidate)
	provenance := release.ProvenanceFor(candidate)
	writeReleaseProcessJSON(t, retainedMetadata, "provenance.json", provenance)
	plan := map[string]any{"schema_version": 1, "tag": tag, "target_commit": commit, "draft": true, "source_run_id": runID, "attestation_source_ref": "refs/tags/" + tag, "candidate_id": candidate.ID, "candidate_assets": candidate.Subjects, "attestation": "attestation.bundle.jsonl", "homebrew": map[string]any{"repository": "yersonargotev/homebrew-tap", "path": "Formula/packy.rb", "sha256": strings.Repeat("a", 64)}}
	writeReleaseProcessJSON(t, retainedMetadata, "publication-plan.json", plan)
	draft := map[string]any{"schema_version": 1, "candidate_id": candidate.ID, "provenance": provenance, "target_commit": commit, "source_run_id": runID, "attestation_source_ref": "refs/tags/" + tag, "publication_plan": plan}
	writeReleaseProcessJSON(t, retainedMetadata, "draft-base.json", draft)
	if mutate != nil {
		mutate(retainedDist, retainedMetadata)
	}
	effectLog := filepath.Join(root, "effects.log")
	fakeVerifier := filepath.Join(root, "bin", "releasecandidate")
	if err := os.WriteFile(fakeVerifier, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf 'releasecandidate %s\\n' \"$*\" >> \"$FIXTURE_EFFECT_LOG\"\nexec \"$FIXTURE_REAL_RELEASECANDIDATE\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	admissionPath := writeReleaseProcessJSON(t, root, "recovery-admission.json", release.AdmissionObservation{
		EventName: "workflow_dispatch", EventRef: release.PackyMainRef, RequestedMode: "recovery",
		Repository: release.PackyRepository, Tag: tag, TagCommit: commit, EventCommit: commit,
		CurrentMain: commit, LatestVersion: tag, TagInMain: true, ReleasePresent: true,
		ReleaseTag: tag, ReleaseState: "draft", ReleaseCommit: commit, ReleaseSchemaVersion: 1,
		ReleaseCandidateID: candidate.ID, ReleaseAttestationSourceRef: "refs/tags/" + tag,
		OriginalRunID: runID, CandidateLocator: "run-" + runID,
	})
	admit := exec.Command(fakeVerifier, "admit", "--observation", admissionPath)
	admit.Env = []string{"PATH=" + os.Getenv("PATH"), "FIXTURE_EFFECT_LOG=" + effectLog, "FIXTURE_REAL_RELEASECANDIDATE=" + releaseCandidateAdapter(t, repoRoot(t))}
	admissionJSON, err := admit.Output()
	if err != nil {
		return retainedRecoveryResult(t, root, effectLog, nil, err)
	}
	var admission release.Admission
	if err := json.Unmarshal(admissionJSON, &admission); err != nil {
		t.Fatal(err)
	}
	tag, commit, runID = admission.Tag, admission.ReleaseCommit, admission.OriginalRunID
	fakeGH := filepath.Join(root, "bin", "gh")
	if err := os.WriteFile(fakeGH, []byte(`#!/usr/bin/env bash
set -euo pipefail
printf 'gh %s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
if [[ "${1:-}" == run ]]; then
  [[ "$#" == 9 && "$2" == download && "$3" == "$FIXTURE_RUN_ID" &&
     "$4" == --repo && "$5" == yersonargotev/packy && "$6" == --name && "$8" == --dir ]] || exit 98
  case "$7" in
  "packy-release-$FIXTURE_TAG")
    [[ "$FIXTURE_DOWNLOAD_FAILURE" != missing-candidate ]] || { echo 'candidate artifact not found' >&2; exit 1; }
    cp -R "$FIXTURE_RETAINED_DIST/." "$9/"
    ;;
  "packy-release-metadata-$FIXTURE_TAG")
    [[ "$FIXTURE_DOWNLOAD_FAILURE" != expired-metadata ]] || { echo 'metadata artifact expired' >&2; exit 1; }
    cp -R "$FIXTURE_RETAINED_METADATA/." "$9/"
    ;;
  *) exit 98 ;;
esac; exit 0; fi
args="$*"
case "$args" in
  *"git/ref/heads/main"*) printf 'commit\t%s\n' "$FIXTURE_COMMIT" ;;
  *"git/ref/tags/"*) printf 'commit\t%s\n' "$FIXTURE_COMMIT" ;;
  *"compare/"*) printf 'ahead\n' ;;
  "api graphql "*) [[ "$FIXTURE_DOWNLOAD_FAILURE" == disappeared-release ]] || printf 'R_release\n' ;;
  "release view "*) jq . "$FIXTURE_RELEASE_JSON" ;;
  "release edit "*) exit 0 ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 98 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	acquire := exec.Command("/bin/bash", filepath.Join(repoRoot(t), "scripts", "acquire-retained-release-candidate.sh"), "--repository", "yersonargotev/packy", "--tag", tag, "--run-id", runID, "--dist", dist, "--metadata", metadata)
	baseEnv := []string{"PATH=" + filepath.Join(root, "bin") + ":" + os.Getenv("PATH"), "FIXTURE_EFFECT_LOG=" + effectLog, "FIXTURE_REAL_RELEASECANDIDATE=" + releaseCandidateAdapter(t, repoRoot(t)), "FIXTURE_RUN_ID=" + runID, "FIXTURE_TAG=" + tag, "FIXTURE_COMMIT=" + commit, "FIXTURE_DOWNLOAD_FAILURE=" + downloadFailure, "FIXTURE_RETAINED_DIST=" + retainedDist, "FIXTURE_RETAINED_METADATA=" + retainedMetadata}
	acquire.Env = baseEnv
	if output, err := acquire.CombinedOutput(); err != nil {
		result := retainedRecoveryResult(t, root, effectLog, output, err)
		result.Admission = &admission
		return result
	}
	command := exec.Command("/bin/bash", filepath.Join(repoRoot(t), "scripts", "verify-retained-release-candidate.sh"), "--tag", tag, "--commit", commit, "--run-id", runID, "--dist", dist, "--metadata", metadata, "--verifier", fakeVerifier)
	command.Dir = repoRoot(t)
	command.Env = append(baseEnv, "HOME="+filepath.Join(root, "home"), "XDG_CONFIG_HOME="+filepath.Join(root, "config"), "XDG_CACHE_HOME="+filepath.Join(root, "cache"), "XDG_STATE_HOME="+filepath.Join(root, "state"), "TMPDIR="+filepath.Join(root, "tmp"))
	output, err := command.CombinedOutput()
	if err != nil {
		result := retainedRecoveryResult(t, root, effectLog, output, err)
		result.Admission = &admission
		return result
	}
	releaseSubjects := append([]release.Subject(nil), candidate.Subjects...)
	releaseSubjects = append(releaseSubjects, release.Subject{Name: "attestation.bundle.jsonl", SHA256: digest([]byte("sealed attestation bundle\n"))})
	releaseState := exactRelease(candidate, true, releaseSubjects)
	expectedBody := filepath.Join(root, "expected-release-body.md")
	if err := os.WriteFile(expectedBody, []byte(releaseProcessBody(releaseState)), 0o600); err != nil {
		t.Fatal(err)
	}
	attestation := filepath.Join(root, "attestation.bundle.jsonl")
	if err := os.WriteFile(attestation, []byte("sealed attestation bundle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	releaseJSON := writeReleaseProcessObservation(t, root, 0, releaseState)
	boundaryEnv := append(baseEnv, "FIXTURE_RELEASE_JSON="+releaseJSON, "RUNNER_TEMP="+filepath.Join(root, "tmp"))
	boundary := exec.Command("/bin/bash", filepath.Join(repoRoot(t), "scripts", "verify-release-boundary.sh"), "--boundary", "publication", "--repository", release.PackyRepository, "--tag", tag, "--release-commit", commit, "--verifier", fakeVerifier, "--ref-output", filepath.Join(root, "refs.json"), "--candidate", filepath.Join(metadata, "candidate.json"), "--provenance", filepath.Join(metadata, "provenance.json"), "--state-output", filepath.Join(root, "state.json"), "--decision-output", filepath.Join(root, "decision.json"), "--expected-body", expectedBody, "--attestation", attestation, "--mode", "draft")
	boundary.Env = boundaryEnv
	if output, err := boundary.CombinedOutput(); err != nil {
		result := retainedRecoveryResult(t, root, effectLog, output, err)
		result.Admission = &admission
		return result
	}
	continuation := exec.Command(filepath.Join(root, "bin", "gh"), "release", "edit", tag, "--repo", release.PackyRepository, "--draft=false")
	continuation.Env = boundaryEnv
	output, err = continuation.CombinedOutput()
	result := retainedRecoveryResult(t, root, effectLog, output, err)
	result.Admission = &admission
	return result
}

func retainedRecoveryResult(t *testing.T, root, effectLog string, output []byte, err error) releaseProcessResult {
	t.Helper()
	result := releaseProcessResult{err: err, Diagnostics: appendDiagnostics(nil, output)}
	data, readErr := os.ReadFile(effectLog)
	if readErr == nil {
		result.Effects = strings.Split(strings.TrimSpace(string(data)), "\n")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	result.Diagnostics = append(result.Diagnostics, "scenario_root="+root)
	return result
}

func resultRoot(result releaseProcessResult) string {
	for _, diagnostic := range result.Diagnostics {
		if root, ok := strings.CutPrefix(diagnostic, "scenario_root="); ok {
			return root
		}
	}
	return ""
}

func TestReleaseScenarioAllowsProtectedMainToAdvanceAtEveryPrivilegedBoundary(t *testing.T) {
	result := runReleaseProcessScenario(t, releaseProcessFixture{
		Name: "later main advancement",
		Inject: func(observed *releaseProcessObservation) {
			observed.MainCommit = strings.Repeat("d", 40)
		},
	})
	if result.err != nil {
		t.Fatalf("later protected-main advancement failed: %v\n%s", result.err, strings.Join(result.Diagnostics, "\n"))
	}
	want := []string{
		"OIDC issuance",
		"draft creation",
		"asset upload",
		"publication",
		"Homebrew mutation",
	}
	if !reflect.DeepEqual(result.Effects, want) {
		t.Fatalf("effects = %#v, want %#v", result.Effects, want)
	}
}

func TestReleaseScenarioAllowsExactPublishedContinuationAtPublication(t *testing.T) {
	result := runReleaseProcessScenario(t, releaseProcessFixture{
		Name:        "published continuation",
		Boundary:    "publication",
		ReleaseMode: "published",
	})
	if result.err != nil {
		t.Fatalf("exact published continuation failed: %v\n%s", result.err, strings.Join(result.Diagnostics, "\n"))
	}
	want := []string{"OIDC issuance", "draft creation", "asset upload", "publication", "Homebrew mutation"}
	if !reflect.DeepEqual(result.Effects, want) {
		t.Fatalf("effects = %#v, want %#v", result.Effects, want)
	}
}

func TestReleaseScenarioReacquiresReleaseStateAfterEarlierEffects(t *testing.T) {
	moved := strings.Repeat("e", 40)
	result := runReleaseProcessScenario(t, releaseProcessFixture{
		Name:     "release target changed after asset upload",
		Boundary: "publication",
		Inject: func(observed *releaseProcessObservation) {
			observed.Release.TargetCommit = moved
		},
	})
	if result.err == nil {
		t.Fatal("publication used release state captured before the earlier asset effect")
	}
	if !containsDiagnostic(result.Diagnostics, "publication denied") ||
		!containsDiagnostic(result.Diagnostics, "observed") ||
		!containsDiagnostic(result.Diagnostics, moved) {
		t.Fatalf("diagnostics = %#v, want freshly observed publication drift", result.Diagnostics)
	}
	want := []string{"OIDC issuance", "draft creation", "asset upload"}
	if !reflect.DeepEqual(result.Effects, want) {
		t.Fatalf("effects = %#v, want only earlier effects %#v", result.Effects, want)
	}
}

func TestReleaseScenarioRejectsReleaseSetDriftAfterEarlierEffects(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*releaseProcessObservation)
	}{
		{
			name: "body drift",
			inject: func(observed *releaseProcessObservation) {
				observed.Release.CandidateID = strings.Repeat("f", 64)
			},
		},
		{
			name: "attestation bundle drift",
			inject: func(observed *releaseProcessObservation) {
				for index := range observed.Release.Assets {
					if observed.Release.Assets[index].Name == "attestation.bundle.jsonl" {
						observed.Release.Assets[index].SHA256 = strings.Repeat("f", 64)
					}
				}
			},
		},
		{
			name: "incomplete candidate assets",
			inject: func(observed *releaseProcessObservation) {
				observed.Release.Assets = observed.Release.Assets[1:]
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result := runReleaseProcessScenario(t, releaseProcessFixture{
				Name: test.name, Boundary: "publication", Inject: test.inject,
			})
			if result.err == nil {
				t.Fatalf("%s passed publication", test.name)
			}
			diagnostic := strings.Join(result.Diagnostics, "\n")
			for _, want := range []string{"publication denied", "expected", "observed"} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
				}
			}
			wantEffects := []string{"OIDC issuance", "draft creation", "asset upload"}
			if !reflect.DeepEqual(result.Effects, wantEffects) {
				t.Fatalf("effects = %#v, want only earlier effects %#v", result.Effects, wantEffects)
			}
		})
	}
}

func releaseProcessBoundaries() []releaseProcessBoundary {
	return []releaseProcessBoundary{
		{Name: "OIDC issuance"},
		{Name: "draft creation"},
		{Name: "asset upload", ReleaseMode: "draft"},
		{Name: "publication", ReleaseMode: "draft"},
		{Name: "Homebrew mutation", ReleaseMode: "published"},
	}
}

func releaseProcessBoundaryIndex(t *testing.T, name string) int {
	t.Helper()
	for index, boundary := range releaseProcessBoundaries() {
		if boundary.Name == name {
			return index
		}
	}
	t.Fatalf("unknown release process boundary %q", name)
	return -1
}

func runReleaseProcessScenario(t *testing.T, fixture releaseProcessFixture) releaseProcessResult {
	t.Helper()
	root := t.TempDir()
	repositoryRoot := repoRoot(t)
	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReleaseScenarioFakes(t, fakeBin)
	commit := strings.Repeat("c", 40)
	candidate := mustCandidate(t, fixtureObservation())
	releaseState := exactRelease(candidate, true, candidate.Subjects)
	candidatePath := writeReleaseProcessJSON(t, root, "candidate.json", candidate)
	provenancePath := writeReleaseProcessJSON(t, root, "provenance.json", release.ProvenanceFor(candidate))
	expectedBody := filepath.Join(root, "expected-release-body.md")
	if err := os.WriteFile(expectedBody, []byte(releaseProcessBody(releaseState)), 0o600); err != nil {
		t.Fatal(err)
	}
	attestation := filepath.Join(root, "attestation.bundle.jsonl")
	attestationBytes := []byte("sealed attestation bundle\n")
	if err := os.WriteFile(attestation, attestationBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	attestationSubject := release.Subject{Name: filepath.Base(attestation), SHA256: digest(attestationBytes)}
	releaseCandidate := releaseCandidateAdapter(t, repositoryRoot)
	effectLog := filepath.Join(root, "effects.log")
	runnerTemp := filepath.Join(root, "runner")
	if err := os.Mkdir(runnerTemp, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + fakeBin + ":" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
		"RUNNER_TEMP=" + runnerTemp,
		"TMPDIR=" + filepath.Join(root, "tmp"),
		"GITHUB_REPOSITORY=yersonargotev/packy",
		"FIXTURE_EFFECT_LOG=" + effectLog,
		"FIXTURE_REAL_RELEASECANDIDATE=" + releaseCandidate,
		"FIXTURE_TAGS=",
		"FIXTURE_RELEASES=",
		"FIXTURE_RELEASE_STATE=",
		"FIXTURE_RELEASE_BODY=",
		"FIXTURE_TAG=v0.1.2",
		"FIXTURE_ALLOW_MUTATION=true",
	}
	for _, directory := range []string{filepath.Join(root, "home"), filepath.Join(root, "config"), filepath.Join(root, "tmp")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	result := releaseProcessResult{}
	for index, boundary := range releaseProcessBoundaries() {
		if fixture.StartAtBoundary && boundary.Name != fixture.Boundary {
			continue
		}
		releaseMode := boundary.ReleaseMode
		if fixture.Boundary == boundary.Name && fixture.ReleaseMode != "" {
			releaseMode = fixture.ReleaseMode
		}
		observed := releaseProcessObservation{
			TagCommit:  commit,
			MainCommit: commit,
			Ancestor:   true,
		}
		if releaseMode != "" {
			state := releaseState
			if releaseMode == "published" {
				state.Draft = false
			}
			if boundary.Name == "publication" || boundary.Name == "Homebrew mutation" {
				state.Assets = append(state.Assets, attestationSubject)
			}
			observed.Release = &state
		}
		if fixture.Inject != nil && (fixture.Boundary == "" || fixture.Boundary == boundary.Name) {
			fixture.Inject(&observed)
		}
		boundaryEnv := append([]string(nil), env...)
		boundaryEnv = append(boundaryEnv,
			"FIXTURE_TAG_COMMIT="+observed.TagCommit,
			"FIXTURE_MAIN_COMMIT="+observed.MainCommit,
			"FIXTURE_ANCESTOR="+fmt.Sprint(observed.Ancestor),
		)
		if observed.Release == nil {
			boundaryEnv = append(boundaryEnv, "FIXTURE_RELEASE_ID=", "FIXTURE_RELEASE_JSON=")
		} else {
			releaseJSON := writeReleaseProcessObservation(t, root, index, *observed.Release)
			boundaryEnv = append(boundaryEnv, "FIXTURE_RELEASE_ID=R_release", "FIXTURE_RELEASE_JSON="+releaseJSON)
		}
		refOutput := filepath.Join(root, fmt.Sprintf("verified-ref-%d.json", index))
		arguments := []string{
			filepath.Join(repositoryRoot, "scripts", "verify-release-boundary.sh"),
			"--boundary", boundary.Name,
			"--repository", "yersonargotev/packy",
			"--tag", candidate.Version,
			"--release-commit", commit,
			"--verifier", filepath.Join(fakeBin, "releasecandidate"),
			"--ref-output", refOutput,
		}
		if releaseMode != "" {
			arguments = append(arguments,
				"--candidate", candidatePath,
				"--provenance", provenancePath,
				"--state-output", filepath.Join(root, fmt.Sprintf("release-state-%d.json", index)),
				"--decision-output", filepath.Join(root, fmt.Sprintf("release-decision-%d.json", index)),
				"--expected-body", expectedBody,
				"--attestation", attestation,
				"--mode", releaseMode,
			)
			if boundary.Name == "asset upload" {
				arguments = append(arguments, "--upload-asset", filepath.Base(attestation))
			}
		}
		command := exec.Command("/bin/bash", arguments...)
		command.Dir = repositoryRoot
		command.Env = boundaryEnv
		if output, err := command.CombinedOutput(); err != nil {
			result.err = fmt.Errorf("%s denied", boundary.Name)
			result.Diagnostics = appendDiagnostics(result.Diagnostics, output)
			result.Effects = readReleaseProcessEffects(t, effectLog, root)
			result.Observations = readReleaseScenarioEffects(t, effectLog, root)
			return result
		}
		effect := releaseProcessEffectCommand(fakeBin, boundary)
		effect.Env = boundaryEnv
		if output, err := effect.CombinedOutput(); err != nil {
			t.Fatalf("fake %s effect failed: %v: %s", boundary.Name, err, output)
		}
	}
	result.Effects = readReleaseProcessEffects(t, effectLog, root)
	result.Observations = readReleaseScenarioEffects(t, effectLog, root)
	return result
}

func releaseProcessEffectCommand(fakeBin string, boundary releaseProcessBoundary) *exec.Cmd {
	switch boundary.Name {
	case "OIDC issuance":
		return exec.Command(filepath.Join(fakeBin, "oidc"), "issue")
	case "draft creation":
		return exec.Command(filepath.Join(fakeBin, "gh"), "release", "create", "v0.1.2")
	case "asset upload":
		return exec.Command(filepath.Join(fakeBin, "gh"), "release", "upload", "v0.1.2", "asset")
	case "publication":
		return exec.Command(filepath.Join(fakeBin, "gh"), "release", "edit", "v0.1.2", "--draft=false")
	case "Homebrew mutation":
		return exec.Command(filepath.Join(fakeBin, "git"), "push", "origin", "HEAD:main")
	default:
		panic("unknown release process boundary " + boundary.Name)
	}
}

func readReleaseProcessEffects(t *testing.T, path, root string) []string {
	t.Helper()
	var effects []string
	for _, effect := range readReleaseScenarioEffects(t, path, root) {
		switch {
		case effect.Tool == "oidc":
			effects = append(effects, "OIDC issuance")
		case effect.Tool == "gh" && strings.HasPrefix(effect.Args, "release create "):
			effects = append(effects, "draft creation")
		case effect.Tool == "gh" && strings.HasPrefix(effect.Args, "release upload "):
			effects = append(effects, "asset upload")
		case effect.Tool == "gh" && strings.HasPrefix(effect.Args, "release edit "):
			effects = append(effects, "publication")
		case effect.Tool == "git" && strings.HasPrefix(effect.Args, "push "):
			effects = append(effects, "Homebrew mutation")
		}
	}
	return effects
}

func writeReleaseProcessJSON(t *testing.T, root, name string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeReleaseProcessObservation(t *testing.T, root string, index int, state release.Release) string {
	t.Helper()
	assets := make([]map[string]string, len(state.Assets))
	for index, asset := range state.Assets {
		assets[index] = map[string]string{"name": asset.Name, "digest": "sha256:" + asset.SHA256}
	}
	body := releaseProcessBody(state)
	return writeReleaseProcessJSON(t, root, fmt.Sprintf("release-observation-%d.json", index), struct {
		TagName string              `json:"tagName"`
		IsDraft bool                `json:"isDraft"`
		Body    string              `json:"body"`
		Assets  []map[string]string `json:"assets"`
	}{
		TagName: state.Version, IsDraft: state.Draft, Body: body, Assets: assets,
	})
}

func releaseProcessBody(state release.Release) string {
	metadata, err := json.Marshal(struct {
		CandidateID  string             `json:"candidate_id"`
		Provenance   release.Provenance `json:"provenance"`
		TargetCommit string             `json:"target_commit"`
	}{
		CandidateID: state.CandidateID, Provenance: state.Provenance, TargetCommit: state.TargetCommit,
	})
	if err != nil {
		panic(err)
	}
	return "notes\n\n<!-- packy-release-metadata\n" + string(metadata) + "\n-->\n"
}

func runReleaseScenario(t *testing.T, scenario releaseScenario) releaseScenarioResult {
	t.Helper()
	root := t.TempDir()
	repositoryRoot := repoRoot(t)
	result := releaseScenarioResult{
		ExitStatus:  -1,
		SandboxRoot: root,
		WritableRoot: map[string]string{
			"HOME":            filepath.Join(root, "home"),
			"XDG_CONFIG_HOME": filepath.Join(root, "config"),
			"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
			"XDG_STATE_HOME":  filepath.Join(root, "state"),
			"GOCACHE":         filepath.Join(root, "go-build"),
			"GOMODCACHE":      filepath.Join(root, "go-mod"),
			"GOPATH":          filepath.Join(root, "go-path"),
			"TMPDIR":          filepath.Join(root, "tmp"),
			"RUNNER_TEMP":     filepath.Join(root, "runner"),
			"GITHUB_OUTPUT":   filepath.Join(root, "output", "github"),
		},
	}
	for name, path := range result.WritableRoot {
		if name == "GITHUB_OUTPUT" {
			path = filepath.Dir(path)
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	effectLog := filepath.Join(root, "effects.log")
	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	releaseCandidate := releaseCandidateAdapter(t, repositoryRoot)
	writeReleaseScenarioFakes(t, fakeBin)

	event := scenario.Event
	releaseBody := fmt.Sprintf("notes\n\n<!-- packy-release-metadata\n{\"schema_version\":1,\"candidate_id\":\"candidate\",\"target_commit\":%q,\"source_run_id\":%q,\"attestation_source_ref\":%q,\"publication_plan\":{\"source_run_id\":%q,\"attestation_source_ref\":%q}}\n-->\n",
		event.tagCommit, event.originalRunID, "refs/tags/"+event.tag, event.originalRunID, "refs/tags/"+event.tag)
	env := []string{
		"PATH=" + fakeBin + ":" + os.Getenv("PATH"),
		"HOME=" + result.WritableRoot["HOME"],
		"XDG_CONFIG_HOME=" + result.WritableRoot["XDG_CONFIG_HOME"],
		"XDG_CACHE_HOME=" + result.WritableRoot["XDG_CACHE_HOME"],
		"XDG_STATE_HOME=" + result.WritableRoot["XDG_STATE_HOME"],
		"GOCACHE=" + result.WritableRoot["GOCACHE"],
		"GOMODCACHE=" + result.WritableRoot["GOMODCACHE"],
		"GOPATH=" + result.WritableRoot["GOPATH"],
		"TMPDIR=" + result.WritableRoot["TMPDIR"],
		"RUNNER_TEMP=" + result.WritableRoot["RUNNER_TEMP"],
		"GITHUB_REPOSITORY=yersonargotev/packy",
		"GITHUB_OUTPUT=" + result.WritableRoot["GITHUB_OUTPUT"],
		"EVENT_NAME=" + event.eventName,
		"EVENT_REF=" + event.eventRef,
		"EVENT_REF_TYPE=" + event.eventRefType,
		"EVENT_REF_NAME=" + event.eventRefName,
		"EVENT_SHA=" + event.eventSHA,
		"INPUT_TAG=" + event.inputTag,
		"INPUT_DRY_RUN=" + event.inputDryRun,
		"FIXTURE_REAL_RELEASECANDIDATE=" + releaseCandidate,
		"FIXTURE_EFFECT_LOG=" + effectLog,
		"FIXTURE_TAG_COMMIT=" + event.tagCommit,
		"FIXTURE_MAIN_COMMIT=" + event.mainCommit,
		"FIXTURE_ANCESTOR=" + fmt.Sprint(event.ancestor),
		"FIXTURE_TAGS=" + event.existingTags,
		"FIXTURE_RELEASES=" + event.existingReleases,
		"FIXTURE_RELEASE_ID=" + event.releaseID,
		"FIXTURE_RELEASE_STATE=" + event.releaseState,
		"FIXTURE_RELEASE_BODY=" + releaseBody,
		"FIXTURE_TAG=" + event.tag,
	}

	normalizer := exec.Command("/bin/bash", filepath.Join(repositoryRoot, "scripts", "normalize-release-event.sh"))
	normalizer.Dir = repositoryRoot
	normalizer.Env = env
	output, err := normalizer.CombinedOutput()
	normalizationStatus := processExitStatus(err)
	result.Stages = append(result.Stages, releaseScenarioStageResult{
		Stage:      releaseScenarioNormalizeStage,
		ExitStatus: normalizationStatus,
	})
	result.Diagnostics = appendDiagnostics(result.Diagnostics, output)
	result.ExitStatus = normalizationStatus
	if err != nil {
		result.err = err
		result.Effects = readReleaseScenarioEffects(t, effectLog, root)
		return result
	}
	result.Admission = new(release.Admission)
	readReleaseScenarioJSON(t, filepath.Join(result.WritableRoot["RUNNER_TEMP"], "admission.json"), result.Admission)
	githubOutput, readErr := os.ReadFile(result.WritableRoot["GITHUB_OUTPUT"])
	if readErr != nil {
		t.Fatal(readErr)
	}
	result.GitHubOutput = string(githubOutput)

	refOutput := filepath.Join(root, "output", "verified-ref-state.json")
	verifier := exec.Command("/bin/bash", filepath.Join(repositoryRoot, "scripts", "verify-release-ref-state.sh"),
		"--repository", "yersonargotev/packy",
		"--tag", result.Admission.Tag,
		"--release-commit", result.Admission.ReleaseCommit,
		"--verifier", filepath.Join(fakeBin, "releasecandidate"),
		"--output", refOutput,
	)
	verifier.Dir = repositoryRoot
	verifier.Env = env
	output, err = verifier.CombinedOutput()
	refStatus := processExitStatus(err)
	result.Stages = append(result.Stages, releaseScenarioStageResult{
		Stage:      releaseScenarioRefStage,
		ExitStatus: refStatus,
	})
	result.Diagnostics = appendDiagnostics(result.Diagnostics, output)
	result.ExitStatus = refStatus
	result.Effects = readReleaseScenarioEffects(t, effectLog, root)
	if err != nil {
		result.err = err
		return result
	}
	result.VerifiedRefs = new(release.VerifiedRefState)
	readReleaseScenarioJSON(t, refOutput, result.VerifiedRefs)
	return result
}

func releaseCandidateAdapter(t *testing.T, repositoryRoot string) string {
	t.Helper()
	releaseCandidateBuild.Do(func() {
		releaseCandidateBuild.root, releaseCandidateBuild.err = os.MkdirTemp("", "packy-releasecandidate-")
		if releaseCandidateBuild.err != nil {
			return
		}
		releaseCandidateBuild.path = filepath.Join(releaseCandidateBuild.root, "releasecandidate")
		roots := map[string]string{
			"HOME":            filepath.Join(releaseCandidateBuild.root, "home"),
			"XDG_CONFIG_HOME": filepath.Join(releaseCandidateBuild.root, "config"),
			"XDG_CACHE_HOME":  filepath.Join(releaseCandidateBuild.root, "cache"),
			"XDG_STATE_HOME":  filepath.Join(releaseCandidateBuild.root, "state"),
			"GOCACHE":         filepath.Join(releaseCandidateBuild.root, "go-build"),
			"GOMODCACHE":      filepath.Join(releaseCandidateBuild.root, "go-mod"),
			"GOPATH":          filepath.Join(releaseCandidateBuild.root, "go-path"),
			"TMPDIR":          filepath.Join(releaseCandidateBuild.root, "tmp"),
		}
		for _, path := range roots {
			if releaseCandidateBuild.err = os.MkdirAll(path, 0o755); releaseCandidateBuild.err != nil {
				return
			}
		}
		module := filepath.Join("golang.org", "x", "sys@v0.32.0")
		sourceModule := filepath.Join(goEnv(t, "GOMODCACHE"), module)
		if releaseCandidateBuild.err = os.CopyFS(
			filepath.Join(roots["GOMODCACHE"], module),
			os.DirFS(sourceModule),
		); releaseCandidateBuild.err != nil {
			releaseCandidateBuild.err = fmt.Errorf("copy cached %s into scenario sandbox: %w", module, releaseCandidateBuild.err)
			return
		}
		sourceDownload := filepath.Join(goEnv(t, "GOMODCACHE"), "cache", "download", "golang.org", "x", "sys", "@v")
		if releaseCandidateBuild.err = os.CopyFS(
			filepath.Join(roots["GOMODCACHE"], "cache", "download", "golang.org", "x", "sys", "@v"),
			os.DirFS(sourceDownload),
		); releaseCandidateBuild.err != nil {
			releaseCandidateBuild.err = fmt.Errorf("copy cached %s download metadata into scenario sandbox: %w", module, releaseCandidateBuild.err)
			return
		}
		command := exec.Command("go", "build", "-o", releaseCandidateBuild.path, "./internal/tools/releasecandidate")
		command.Dir = repositoryRoot
		command.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + roots["HOME"],
			"XDG_CONFIG_HOME=" + roots["XDG_CONFIG_HOME"],
			"XDG_CACHE_HOME=" + roots["XDG_CACHE_HOME"],
			"XDG_STATE_HOME=" + roots["XDG_STATE_HOME"],
			"GOCACHE=" + roots["GOCACHE"],
			"GOMODCACHE=" + roots["GOMODCACHE"],
			"GOPATH=" + roots["GOPATH"],
			"TMPDIR=" + roots["TMPDIR"],
			"GOENV=off",
			"GOTOOLCHAIN=local",
			"GOPROXY=off",
			"GOSUMDB=off",
			"CGO_ENABLED=0",
		}
		if output, err := command.CombinedOutput(); err != nil {
			releaseCandidateBuild.err = fmt.Errorf("build releasecandidate: %w: %s", err, output)
		}
	})
	if releaseCandidateBuild.err != nil {
		t.Fatal(releaseCandidateBuild.err)
	}
	return releaseCandidateBuild.path
}

func writeReleaseScenarioFakes(t *testing.T, fakeBin string) {
	t.Helper()
	files := map[string]string{
		"go": `#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -ge 3 && "$1" == run && "$2" == ./internal/tools/releasecandidate ]] || {
  echo "unexpected go invocation: $*" >&2
  exit 96
}
shift 2
printf 'releasecandidate\t%s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
exec "$FIXTURE_REAL_RELEASECANDIDATE" "$@"
`,
		"git": `#!/usr/bin/env bash
set -euo pipefail
printf 'git\t%s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
case "${1:-}" in
  fetch|checkout) exit 0 ;;
  push) [[ "${FIXTURE_ALLOW_MUTATION:-}" == true ]] ;;
  rev-parse) printf '%s\n' "$FIXTURE_TAG_COMMIT" ;;
  merge-base) [[ "$FIXTURE_ANCESTOR" == true ]] ;;
  tag) [[ -z "$FIXTURE_TAGS" ]] || printf '%s\n' "$FIXTURE_TAGS" ;;
  *) echo "unexpected git invocation: $*" >&2; exit 97 ;;
esac
`,
		"gh": `#!/usr/bin/env bash
set -euo pipefail
printf 'gh\t%s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
args="$*"
if [[ "${1:-}" == api && "${2:-}" == graphql ]]; then
  expected_query='query($owner:String!,$repository:String!,$tag:String!){repository(owner:$owner,name:$repository){release(tagName:$tag){id}}}'
  [[ "$#" -eq 12 &&
     "$3" == -f && "$4" == "query=$expected_query" &&
     "$5" == -f && "$6" == "owner=yersonargotev" &&
     "$7" == -f && "$8" == "repository=packy" &&
     "$9" == -f && "${10}" == "tag=$FIXTURE_TAG" &&
     "${11}" == --jq && "${12}" == '.data.repository.release.id // ""' ]] || {
    echo "unexpected GraphQL invocation: $*" >&2
    exit 98
  }
  [[ "$expected_query" != mutation* ]] || {
    echo "GraphQL mutation is forbidden" >&2
    exit 98
  }
  [[ -z "$FIXTURE_RELEASE_ID" ]] || printf '%s\n' "$FIXTURE_RELEASE_ID"
  exit 0
fi
case "$args" in
  release\ create\ *|release\ upload\ *|release\ edit\ *)
    [[ "${FIXTURE_ALLOW_MUTATION:-}" == true ]] || {
      echo "GitHub mutation is forbidden" >&2
      exit 98
    }
    exit 0
    ;;
  *"git/ref/heads/main"*) printf 'commit\t%s\n' "$FIXTURE_MAIN_COMMIT" ;;
  *"git/ref/tags/"*) printf 'commit\t%s\n' "$FIXTURE_TAG_COMMIT" ;;
  *"compare/"*)
    if [[ "$FIXTURE_ANCESTOR" == true ]]; then printf 'ahead\n'; else printf 'diverged\n'; fi
    ;;
  *"releases?per_page=100&page=1"*) [[ -z "$FIXTURE_RELEASES" ]] || printf '%s\n' "$FIXTURE_RELEASES" ;;
  *"releases?per_page=100"*) [[ -z "$FIXTURE_RELEASES" ]] || printf '%s\n' "$FIXTURE_RELEASES" ;;
  *"releases?per_page=100&page="*) ;;
  *)
    if [[ "$args" == release\ view* ]]; then
      if [[ -n "${FIXTURE_RELEASE_JSON:-}" ]]; then
        jq . "$FIXTURE_RELEASE_JSON"
        exit 0
      fi
      is_draft=false
      [[ "$FIXTURE_RELEASE_STATE" == draft ]] && is_draft=true
      jq -n --arg tag "$FIXTURE_TAG" --arg body "$FIXTURE_RELEASE_BODY" \
        --argjson isDraft "$is_draft" '{tagName:$tag,isDraft:$isDraft,body:$body}'
      exit 0
    fi
    echo "unexpected gh invocation: $*" >&2
    exit 98
    ;;
esac
`,
		"releasecandidate": `#!/usr/bin/env bash
set -euo pipefail
printf 'releasecandidate\t%s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
exec "$FIXTURE_REAL_RELEASECANDIDATE" "$@"
`,
		"oidc": `#!/usr/bin/env bash
set -euo pipefail
printf 'oidc\t%s\n' "$*" >> "$FIXTURE_EFFECT_LOG"
[[ "${FIXTURE_ALLOW_MUTATION:-}" == true ]]
`,
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(files[name]), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func readReleaseScenarioEffects(t *testing.T, path, root string) []releaseScenarioEffect {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var effects []releaseScenarioEffect
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		tool, args, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("malformed scenario effect %q", line)
		}
		effects = append(effects, releaseScenarioEffect{
			Tool: tool,
			Args: normalizeReleaseScenarioArgs(args, root),
		})
	}
	return effects
}

func normalizeReleaseScenarioArgs(args, root string) string {
	fields := strings.Fields(args)
	for index := range fields {
		if index > 0 && fields[index-1] == "--observation" {
			fields[index] = "$OBSERVATION"
			continue
		}
		fields[index] = strings.ReplaceAll(fields[index], root, "$SCENARIO_ROOT")
	}
	return strings.Join(fields, " ")
}

func readReleaseScenarioJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func processExitStatus(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func appendDiagnostics(current []string, output []byte) []string {
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			current = append(current, line)
		}
	}
	return current
}

func containsDiagnostic(diagnostics []string, want string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, want) {
			return true
		}
	}
	return false
}

func assertNoReleaseScenarioRemoteMutation(t *testing.T, effects []releaseScenarioEffect) {
	t.Helper()
	for _, effect := range effects {
		command := effect.Tool + " " + effect.Args
		for _, forbidden := range []string{
			"git push",
			"gh release create",
			"gh release edit",
			"gh release upload",
			"gh release delete",
			"gh api --method POST",
			"gh api --method PATCH",
			"gh api --method PUT",
			"gh api --method DELETE",
		} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("valid pre-publication scenario attempted remote mutation %q", command)
			}
		}
	}
}

func stableReleaseScenarioResult(result releaseScenarioResult) releaseScenarioResult {
	result.SandboxRoot = ""
	result.WritableRoot = nil
	result.err = nil
	return result
}

func pathWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
