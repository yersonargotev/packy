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
	Name     string
	Boundary string
	Inject   func(*releaseProcessObservation)
}

type releaseProcessObservation struct {
	TagCommit  string
	MainCommit string
	Ancestor   bool
	Release    *release.Release
}

type releaseProcessResult struct {
	Effects     []string
	Diagnostics []string
	err         error
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
		observed := releaseProcessObservation{
			TagCommit:  commit,
			MainCommit: commit,
			Ancestor:   true,
		}
		if boundary.ReleaseMode != "" {
			state := releaseState
			if boundary.ReleaseMode == "published" {
				state.Draft = false
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
			"--verifier", releaseCandidate,
			"--ref-output", refOutput,
		}
		if boundary.ReleaseMode != "" {
			arguments = append(arguments,
				"--candidate", candidatePath,
				"--provenance", provenancePath,
				"--state-output", filepath.Join(root, fmt.Sprintf("release-state-%d.json", index)),
				"--mode", boundary.ReleaseMode,
			)
		}
		command := exec.Command("/bin/bash", arguments...)
		command.Dir = repositoryRoot
		command.Env = boundaryEnv
		if output, err := command.CombinedOutput(); err != nil {
			result.err = fmt.Errorf("%s denied", boundary.Name)
			result.Diagnostics = appendDiagnostics(result.Diagnostics, output)
			result.Effects = readReleaseProcessEffects(t, effectLog, root)
			return result
		}
		effect := releaseProcessEffectCommand(fakeBin, boundary)
		effect.Env = boundaryEnv
		if output, err := effect.CombinedOutput(); err != nil {
			t.Fatalf("fake %s effect failed: %v: %s", boundary.Name, err, output)
		}
	}
	result.Effects = readReleaseProcessEffects(t, effectLog, root)
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
	metadata, err := json.Marshal(struct {
		CandidateID  string             `json:"candidate_id"`
		Provenance   release.Provenance `json:"provenance"`
		TargetCommit string             `json:"target_commit"`
	}{
		CandidateID: state.CandidateID, Provenance: state.Provenance, TargetCommit: state.TargetCommit,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := "notes\n\n<!-- packy-release-metadata\n" + string(metadata) + "\n-->\n"
	return writeReleaseProcessJSON(t, root, fmt.Sprintf("release-observation-%d.json", index), struct {
		TagName string              `json:"tagName"`
		IsDraft bool                `json:"isDraft"`
		Body    string              `json:"body"`
		Assets  []map[string]string `json:"assets"`
	}{
		TagName: state.Version, IsDraft: state.Draft, Body: body, Assets: assets,
	})
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
