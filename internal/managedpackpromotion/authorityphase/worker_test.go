package authorityphase

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestRunPrepublicationStagesOnlyTheSealedCandidate(t *testing.T) {
	protocolRoot := t.TempDir()
	_, candidate := candidateRepository(t)
	sanitizedRepository := filepath.Join(protocolRoot, "repository")
	if err := os.Mkdir(sanitizedRepository, 0o700); err != nil {
		t.Fatal(err)
	}
	request, err := newPrepareRequest(managedpackpromotion.Request{RepositoryRoot: sanitizedRepository, Coordinate: candidate.Coordinate})
	if err != nil {
		t.Fatal(err)
	}
	requestPath, responsePath := filepath.Join(protocolRoot, "request.json"), filepath.Join(protocolRoot, "response.json")
	if err := writePrepareRequest(requestPath, request); err != nil {
		t.Fatal(err)
	}
	module := &workerPromoter{promote: func(ctx context.Context, got managedpackpromotion.Request, publisher managedpackpromotion.Publisher) (managedpackpromotion.Result, error) {
		if got != request.Request {
			t.Fatalf("request = %#v, want %#v", got, request.Request)
		}
		publication, err := publisher.Publish(ctx, candidate)
		if err != nil {
			return managedpackpromotion.Result{}, err
		}
		return managedpackpromotion.Result{Status: managedpackpromotion.StatusNoChange, Reason: publication.NoChangeReason}, nil
	}}
	var stdout, stderr bytes.Buffer
	exit := RunPrepublication(context.Background(), []string{requestPath, responsePath}, &stdout, &stderr, func(publisher managedpackpromotion.Publisher) Promoter {
		module.publisher = publisher
		return module
	})
	if exit != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
	response, err := readPrepareResponse(responsePath, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != prepareCandidate || response.Candidate.RepositoryRoot != filepath.Join(protocolRoot, candidateDirectoryName) {
		t.Fatalf("response = %#v", response)
	}
	got := response.Candidate
	if got.ID != candidate.ID || got.HeadSHA != candidate.HeadSHA || got.ResultTreeSHA != candidate.ResultTreeSHA || got.BaseSHA != candidate.BaseSHA {
		t.Fatalf("staged candidate = %#v, want identity %#v", got, candidate)
	}
	if remote := strings.TrimSpace(runTestCommand(t, got.RepositoryRoot, "git", "remote", "get-url", "origin")); remote != "https://github.com/example/packy.git" {
		t.Fatalf("staged remote = %q", remote)
	}
	if tree := strings.TrimSpace(runTestCommand(t, got.RepositoryRoot, "git", "show", "-s", "--format=%T", got.HeadSHA)); tree != got.ResultTreeSHA {
		t.Fatalf("staged tree = %q, want %q", tree, got.ResultTreeSHA)
	}
}

func TestRunPublicationReturnsTypedRejectionFromCandidateOnly(t *testing.T) {
	protocolRoot := t.TempDir()
	candidate := validCandidate(filepath.Join(protocolRoot, candidateDirectoryName))
	if err := os.Mkdir(candidate.RepositoryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	request, err := newPublishRequest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	requestPath, responsePath := filepath.Join(protocolRoot, "request.json"), filepath.Join(protocolRoot, "response.json")
	if err := writePublishRequest(requestPath, request); err != nil {
		t.Fatal(err)
	}
	publisher := publisherFunc(func(_ context.Context, got managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
		if got != candidate {
			t.Fatalf("candidate = %#v, want %#v", got, candidate)
		}
		return managedpackpromotion.Publication{}, managedpackpromotion.Reject(managedpackpromotion.GateFreshness, "origin/main moved")
	})
	var stdout, stderr bytes.Buffer
	exit := RunPublication(context.Background(), []string{requestPath, responsePath}, &stdout, &stderr, publisher)
	if exit != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
	response, err := readPublishResponse(responsePath, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Status != managedpackpromotion.StatusRejected || response.Result.Rejection == nil || response.Result.Rejection.Gate != managedpackpromotion.GateFreshness {
		t.Fatalf("result = %#v", response.Result)
	}
}

func TestWorkersRejectPathsThatCrossTheProtocolRoot(t *testing.T) {
	root := t.TempDir()
	request, err := newPrepareRequest(managedpackpromotion.Request{RepositoryRoot: root, Coordinate: managedpackpromotion.Coordinate{PackID: "example", Version: "1.2.3"}})
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(root, "request.json")
	if err := writePrepareRequest(requestPath, request); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	exit := RunPrepublication(context.Background(), []string{requestPath, filepath.Join(t.TempDir(), "response.json")}, io.Discard, &stderr, func(managedpackpromotion.Publisher) Promoter {
		t.Fatal("module factory must not be called")
		return nil
	})
	if exit != 1 || !strings.Contains(stderr.String(), "protocol root") {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
}

type workerPromoter struct {
	publisher managedpackpromotion.Publisher
	promote   func(context.Context, managedpackpromotion.Request, managedpackpromotion.Publisher) (managedpackpromotion.Result, error)
}

func (promoter *workerPromoter) Promote(ctx context.Context, request managedpackpromotion.Request) (managedpackpromotion.Result, error) {
	return promoter.promote(ctx, request, promoter.publisher)
}

type publisherFunc func(context.Context, managedpackpromotion.Candidate) (managedpackpromotion.Publication, error)

func (publish publisherFunc) Publish(ctx context.Context, candidate managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
	return publish(ctx, candidate)
}

func candidateRepository(t *testing.T) (string, managedpackpromotion.Candidate) {
	t.Helper()
	root := testRepository(t)
	base := strings.TrimSpace(runTestCommand(t, root, "git", "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "candidate.txt"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, root, "git", "add", "candidate.txt")
	runTestCommand(t, root, "git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "candidate")
	head := strings.TrimSpace(runTestCommand(t, root, "git", "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runTestCommand(t, root, "git", "show", "-s", "--format=%T", head))
	candidate := validCandidate(root)
	candidate.BaseSHA, candidate.HeadSHA, candidate.ResultTreeSHA = base, head, tree
	return root, candidate
}
