package authorityphase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const stagedNoChangeReason = "sealed candidate staged for publication authority"

type Promoter interface {
	Promote(context.Context, managedpackpromotion.Request) (managedpackpromotion.Result, error)
}

// RunPrepublication executes the complete high-level promotion module with a
// terminal Publisher that can only stage its already sealed Candidate.
func RunPrepublication(ctx context.Context, args []string, _ io.Writer, stderr io.Writer, factory func(managedpackpromotion.Publisher) Promoter) int {
	if len(args) != 2 || factory == nil {
		fmt.Fprintln(stderr, "invalid prepublication worker invocation")
		return 1
	}
	root, err := workerProtocolRoot(args[0], args[1])
	if err != nil {
		fmt.Fprintf(stderr, "validate prepublication protocol root: %v\n", err)
		return 1
	}
	request, err := readPrepareRequest(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "read prepublication request: %v\n", err)
		return 1
	}
	if !pathWithin(root, request.Request.RepositoryRoot) {
		fmt.Fprintln(stderr, "prepublication repository is outside the protocol root")
		return 1
	}
	stager := &stagingPublisher{destination: filepath.Join(root, candidateDirectoryName)}
	result, err := factory(stager).Promote(ctx, request.Request)
	if err != nil {
		fmt.Fprintf(stderr, "prepare promotion candidate: %v\n", err)
		return 1
	}
	response := prepareResponse{}
	if stager.candidate != nil {
		if result.Status != managedpackpromotion.StatusNoChange || result.Reason != stagedNoChangeReason {
			fmt.Fprintln(stderr, "promotion module did not acknowledge the staged candidate")
			return 1
		}
		response.Status, response.Candidate = prepareCandidate, *stager.candidate
	} else {
		if err := validateTerminalResult(result); err != nil {
			fmt.Fprintf(stderr, "validate prepublication result: %v\n", err)
			return 1
		}
		response.Status, response.Result = prepareResult, result
	}
	if err := writePrepareResponse(args[1], request, response); err != nil {
		fmt.Fprintf(stderr, "write prepublication response: %v\n", err)
		return 1
	}
	return 0
}

// RunPublication invokes only the mutation Publisher with a sealed Candidate.
func RunPublication(ctx context.Context, args []string, _ io.Writer, stderr io.Writer, publisher managedpackpromotion.Publisher) int {
	if len(args) != 2 || publisher == nil {
		fmt.Fprintln(stderr, "invalid publication worker invocation")
		return 1
	}
	root, err := workerProtocolRoot(args[0], args[1])
	if err != nil {
		fmt.Fprintf(stderr, "validate publication protocol root: %v\n", err)
		return 1
	}
	request, err := readPublishRequest(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "read publication request: %v\n", err)
		return 1
	}
	if err := validateStagedCandidate(root, request.Candidate.Coordinate, request.Candidate); err != nil {
		fmt.Fprintf(stderr, "validate sealed publication candidate: %v\n", err)
		return 1
	}
	publication, err := publisher.Publish(ctx, request.Candidate)
	var result managedpackpromotion.Result
	if err != nil {
		var rejection *managedpackpromotion.RejectionError
		if !errors.As(err, &rejection) {
			fmt.Fprintf(stderr, "publish sealed candidate: %v\n", err)
			return 1
		}
		result = managedpackpromotion.Result{Status: managedpackpromotion.StatusRejected, Rejection: &managedpackpromotion.Rejection{Gate: rejection.Gate, Reason: rejection.Reason}}
	} else {
		result, err = resultForPublication(publication)
		if err != nil {
			fmt.Fprintf(stderr, "validate publication: %v\n", err)
			return 1
		}
	}
	if err := writePublishResponse(args[1], request, publishResponse{Result: result}); err != nil {
		fmt.Fprintf(stderr, "write publication response: %v\n", err)
		return 1
	}
	return 0
}

type stagingPublisher struct {
	destination string
	candidate   *managedpackpromotion.Candidate
}

func (publisher *stagingPublisher) Publish(ctx context.Context, candidate managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
	if publisher.candidate != nil {
		return managedpackpromotion.Publication{}, errors.New("promotion module attempted to stage more than one candidate")
	}
	if err := stageCandidate(ctx, candidate, publisher.destination); err != nil {
		return managedpackpromotion.Publication{}, err
	}
	candidate.RepositoryRoot = publisher.destination
	publisher.candidate = &candidate
	return managedpackpromotion.Publication{NoChangeReason: stagedNoChangeReason}, nil
}

func stageCandidate(ctx context.Context, candidate managedpackpromotion.Candidate, destination string) error {
	if strings.TrimSpace(candidate.RepositoryRoot) == "" || !filepath.IsAbs(candidate.RepositoryRoot) {
		return errors.New("candidate repository root must be absolute")
	}
	environment := os.Environ()
	remote, err := commandText(ctx, candidate.RepositoryRoot, environment, "git", "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("resolve staged candidate origin: %w", err)
	}
	remote, err = sanitizeRemote(remote)
	if err != nil {
		return err
	}
	if _, err := commandText(ctx, "", environment, "git", "clone", "--local", "--no-hardlinks", "--no-checkout", candidate.RepositoryRoot, destination); err != nil {
		return fmt.Errorf("stage sealed candidate repository: %w", err)
	}
	if _, err := commandText(ctx, destination, environment, "git", "remote", "set-url", "origin", remote); err != nil {
		return fmt.Errorf("restore staged candidate origin: %w", err)
	}
	identity, err := commandText(ctx, destination, environment, "git", "show", "-s", "--format=%H%x00%T%x00%P", candidate.HeadSHA)
	if err != nil {
		return fmt.Errorf("verify staged candidate identity: %w", err)
	}
	if identity != candidate.HeadSHA+"\x00"+candidate.ResultTreeSHA+"\x00"+candidate.BaseSHA {
		return errors.New("staged repository does not contain the sealed candidate commit")
	}
	return nil
}

func workerProtocolRoot(requestPath, responsePath string) (string, error) {
	if !filepath.IsAbs(requestPath) || !filepath.IsAbs(responsePath) || filepath.Clean(requestPath) != requestPath || filepath.Clean(responsePath) != responsePath {
		return "", errors.New("protocol paths must be absolute and clean")
	}
	root := filepath.Dir(requestPath)
	if filepath.Dir(responsePath) != root || requestPath == responsePath {
		return "", errors.New("request and response must share one protocol root")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("protocol root must be a real directory")
	}
	return root, nil
}

var _ managedpackpromotion.Publisher = (*stagingPublisher)(nil)
