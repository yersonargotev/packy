package authorityphase

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestPromoteSeparatesPrepublicationFromMutationAuthority(t *testing.T) {
	repository := testRepository(t)
	executable := testExecutable(t)
	candidate := validCandidate(filepath.Join(t.TempDir(), "candidate"))
	ambient := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"GH_TOKEN=github-write-token",
		"AWS_SECRET_ACCESS_KEY=unrelated-secret",
	}
	var protocolRoot string
	runner := processRunnerFunc(func(_ context.Context, invocation processInvocation) error {
		if protocolRoot == "" {
			protocolRoot = filepath.Dir(invocation.RequestPath)
		}
		switch invocation.Mode {
		case PrepareModeArgument:
			if environmentValue(invocation.Environment, "GH_TOKEN") != "" || environmentValue(invocation.Environment, "AWS_SECRET_ACCESS_KEY") != "" {
				t.Fatalf("prepublication environment retained credentials: %q", invocation.Environment)
			}
			request, err := readPrepareRequest(invocation.RequestPath)
			if err != nil {
				return err
			}
			if request.Request.RepositoryRoot == repository || !pathWithin(protocolRoot, request.Request.RepositoryRoot) {
				t.Fatalf("prepublication repository = %q, original = %q", request.Request.RepositoryRoot, repository)
			}
			if remote := strings.TrimSpace(runTestCommand(t, request.Request.RepositoryRoot, "git", "remote", "get-url", "origin")); remote != "https://github.com/example/packy.git" {
				t.Fatalf("sanitized prepublication remote = %q", remote)
			}
			candidate.RepositoryRoot = filepath.Join(protocolRoot, candidateDirectoryName)
			if err := os.Mkdir(candidate.RepositoryRoot, 0o700); err != nil {
				return err
			}
			return writePrepareResponse(invocation.ResponsePath, request, prepareResponse{Status: prepareCandidate, Candidate: candidate})
		case PublishModeArgument:
			if environmentValue(invocation.Environment, "GH_TOKEN") != "github-write-token" {
				t.Fatalf("mutation environment omitted GitHub authority: %q", invocation.Environment)
			}
			if environmentValue(invocation.Environment, "AWS_SECRET_ACCESS_KEY") != "" {
				t.Fatalf("mutation environment retained unrelated authority: %q", invocation.Environment)
			}
			request, err := readPublishRequest(invocation.RequestPath)
			if err != nil {
				return err
			}
			if request.Candidate != candidate {
				t.Fatalf("mutation candidate = %#v, want %#v", request.Candidate, candidate)
			}
			return writePublishResponse(invocation.ResponsePath, request, publishResponse{Result: managedpackpromotion.Result{Status: managedpackpromotion.StatusProposal, Proposal: &managedpackpromotion.Proposal{
				Branch: candidate.Branch, Number: 17, URL: "https://github.com/example/packy/pull/17", HeadSHA: candidate.HeadSHA,
			}}})
		default:
			t.Fatalf("unexpected mode %q", invocation.Mode)
			return nil
		}
	})

	adapter := newWithRunner(executable, ambient, runner)
	result, err := adapter.Promote(context.Background(), managedpackpromotion.Request{
		RepositoryRoot: repository,
		Coordinate:     managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"},
	})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if result.Status != managedpackpromotion.StatusProposal || result.Proposal == nil || result.Proposal.Number != 17 {
		t.Fatalf("Promote() result = %#v", result)
	}
	if _, err := os.Stat(protocolRoot); !os.IsNotExist(err) {
		t.Fatalf("protocol root survived promotion: %v", err)
	}
}

func TestPromotePropagatesCancellationAndRemovesAuthorityState(t *testing.T) {
	repository := testRepository(t)
	executable := testExecutable(t)
	ctx, cancel := context.WithCancel(context.Background())
	var protocolRoot string
	runner := processRunnerFunc(func(workerContext context.Context, invocation processInvocation) error {
		protocolRoot = filepath.Dir(invocation.RequestPath)
		cancel()
		<-workerContext.Done()
		return workerContext.Err()
	})
	adapter := newWithRunner(executable, testAmbient(t), runner)
	_, err := adapter.Promote(ctx, managedpackpromotion.Request{RepositoryRoot: repository, Coordinate: managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Promote() error = %v, want cancellation", err)
	}
	if _, err := os.Stat(protocolRoot); !os.IsNotExist(err) {
		t.Fatalf("protocol root survived cancellation: %v", err)
	}
}

func TestPromoteFailsClosedBeforeMutationWhenPrepublicationResponseIsTampered(t *testing.T) {
	repository := testRepository(t)
	executable := testExecutable(t)
	mutated := false
	runner := processRunnerFunc(func(_ context.Context, invocation processInvocation) error {
		if invocation.Mode == PublishModeArgument {
			mutated = true
			return nil
		}
		request, err := readPrepareRequest(invocation.RequestPath)
		if err != nil {
			return err
		}
		response := prepareResponse{Status: prepareResult, Result: managedpackpromotion.Result{Status: managedpackpromotion.StatusNoChange, Reason: "exact"}}
		if err := writePrepareResponse(invocation.ResponsePath, request, response); err != nil {
			return err
		}
		data, err := os.ReadFile(invocation.ResponsePath)
		if err != nil {
			return err
		}
		return os.WriteFile(invocation.ResponsePath, []byte(strings.Replace(string(data), "exact", "altered", 1)), 0o600)
	})
	adapter := newWithRunner(executable, testAmbient(t), runner)
	_, err := adapter.Promote(context.Background(), managedpackpromotion.Request{RepositoryRoot: repository, Coordinate: managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"}})
	if err == nil || !strings.Contains(err.Error(), "digest") || mutated {
		t.Fatalf("Promote() error = %v, mutation called = %t", err, mutated)
	}
}

func TestSanitizeRemoteRemovesEmbeddedURLAuthority(t *testing.T) {
	tests := map[string]string{
		"https://token@github.com/example/packy.git?credential=secret#fragment": "https://github.com/example/packy.git",
		"ssh://token@github.com/example/packy.git?credential=secret":            "ssh://git@github.com/example/packy.git",
		"git@github.com:example/packy.git":                                      "git@github.com:example/packy.git",
		"write-token@github.com:example/packy.git":                              "git@github.com:example/packy.git",
		"/tmp/local-packy.git":                                                  "/tmp/local-packy.git",
	}
	for input, want := range tests {
		got, err := sanitizeRemote(input)
		if err != nil || got != want {
			t.Fatalf("sanitizeRemote(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestPrepublicationEnvironmentDoesNotResolveGoThroughAmbientPATH(t *testing.T) {
	root := t.TempDir()
	ambientHome := filepath.Join(root, "ambient-home")
	if err := os.Mkdir(ambientHome, 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := prepublicationEnvironment(context.Background(), root, []string{
		"PATH=" + filepath.Join(root, "ambient-path"),
		"HOME=" + ambientHome,
		"GH_TOKEN=must-not-reach-go-env",
	})
	if err != nil {
		t.Fatalf("prepublicationEnvironment() error = %v", err)
	}
	if environmentValue(environment, "GH_TOKEN") != "" {
		t.Fatalf("prepublication environment retained GitHub authority: %q", environment)
	}
	for _, key := range []string{"GOROOT", "GOCACHE", "GOMODCACHE", "GOPATH"} {
		value := environmentValue(environment, key)
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			t.Fatalf("%s = %q, want absolute clean path", key, value)
		}
	}
	if childMode := environmentValue(environment, "GO_TELEMETRY_CHILD"); childMode != "2" {
		t.Fatalf("GO_TELEMETRY_CHILD = %q, want child-of-child mode", childMode)
	}
	if err := filepath.WalkDir(ambientHome, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == "upload.token" {
			t.Fatalf("Go telemetry sidecar token escaped into ambient HOME at %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect ambient HOME after Go environment resolution: %v", err)
	}
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func validCandidate(root string) managedpackpromotion.Candidate {
	return managedpackpromotion.Candidate{
		ID: "sealed", Summary: "sealed summary", Coordinate: managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"},
		Project: "example/project", RepositoryRoot: root, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40),
		ResultTreeSHA: strings.Repeat("c", 40), Branch: "promote/example-1.2.3",
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "promotepack")
	if err := os.WriteFile(path, []byte("executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testAmbient(t *testing.T) []string {
	t.Helper()
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
}

func testRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runTestCommand(t, root, "git", "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, root, "git", "add", "README.md")
	runTestCommand(t, root, "git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "fixture")
	sha := strings.TrimSpace(runTestCommand(t, root, "git", "rev-parse", "HEAD"))
	runTestCommand(t, root, "git", "update-ref", "refs/remotes/origin/main", sha)
	runTestCommand(t, root, "git", "remote", "add", "origin", "https://token@github.com/example/packy.git")
	return root
}

func runTestCommand(t *testing.T, directory string, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = testprocess.Env(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
