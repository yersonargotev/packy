// Command promotepack is Packy's repository-private Managed Pack Promotion
// adapter. It is intentionally outside cmd/ and public release artifacts.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/githubacquisition"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/githubproposal"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/offlinevalidation"
	"github.com/yersonargotev/packy/internal/managedpackpromotion/repositorycandidate"
	"github.com/yersonargotev/packy/internal/packsync/githubsource"
)

const usage = "usage: promotepack <pack-id>@<version>"

type promoter interface {
	Promote(context.Context, managedpackpromotion.Request) (managedpackpromotion.Result, error)
}

type dependencies struct {
	executable       func() (string, error)
	repositoryRoot   func(context.Context) (string, error)
	newPromoter      func(string) promoter
	runOfflineWorker func([]string, io.Writer, io.Writer) int
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, productionDependencies()))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	if len(args) > 0 && args[0] == offlinevalidation.ModeArgument {
		if deps.runOfflineWorker == nil {
			fmt.Fprintln(stderr, "promotepack offline worker dependency is incomplete")
			return 1
		}
		return deps.runOfflineWorker(args[1:], stdout, stderr)
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	coordinate, err := managedpackpromotion.ParseCoordinate(args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if deps.executable == nil || deps.repositoryRoot == nil || deps.newPromoter == nil {
		fmt.Fprintln(stderr, "promotepack dependencies are incomplete")
		return 1
	}
	executable, err := deps.executable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve promotepack executable: %v\n", err)
		return 1
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		fmt.Fprintf(stderr, "resolve absolute promotepack executable: %v\n", err)
		return 1
	}
	repositoryRoot, err := deps.repositoryRoot(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "resolve Packy repository root: %v\n", err)
		return 1
	}
	result, err := deps.newPromoter(filepath.Clean(executable)).Promote(ctx, managedpackpromotion.Request{
		RepositoryRoot: repositoryRoot,
		Coordinate:     coordinate,
	})
	if err != nil {
		fmt.Fprintf(stderr, "promote %s: %v\n", coordinate, err)
		return 1
	}
	return renderResult(result, stdout, stderr)
}

func renderResult(result managedpackpromotion.Result, stdout, stderr io.Writer) int {
	switch result.Status {
	case managedpackpromotion.StatusNoChange:
		if strings.TrimSpace(result.Reason) == "" {
			fmt.Fprintln(stderr, "promotion returned no-change without a reason")
			return 1
		}
		fmt.Fprintf(stdout, "no-change %s\n", result.Reason)
		return 0
	case managedpackpromotion.StatusRejected:
		if result.Rejection == nil || result.Rejection.Gate == "" || strings.TrimSpace(result.Rejection.Reason) == "" {
			fmt.Fprintln(stderr, "promotion returned an incomplete rejection")
			return 1
		}
		fmt.Fprintf(stderr, "rejected gate=%s reason=%s\n", result.Rejection.Gate, result.Rejection.Reason)
		return 1
	case managedpackpromotion.StatusProposal:
		if result.Proposal == nil || result.Proposal.Number <= 0 || result.Proposal.URL == "" || result.Proposal.Branch == "" || result.Proposal.HeadSHA == "" {
			fmt.Fprintln(stderr, "promotion returned an incomplete proposal")
			return 1
		}
		fmt.Fprintf(stdout, "proposal %s branch=%s head=%s\n", result.Proposal.URL, result.Proposal.Branch, result.Proposal.HeadSHA)
		return 0
	default:
		fmt.Fprintf(stderr, "promotion returned unsupported status %q\n", result.Status)
		return 1
	}
}

func productionDependencies() dependencies {
	return dependencies{
		executable:       os.Executable,
		runOfflineWorker: offlinevalidation.Run,
		repositoryRoot: func(ctx context.Context) (string, error) {
			command := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
			output, err := command.Output()
			if err != nil {
				return "", err
			}
			root := strings.TrimSpace(string(output))
			if root == "" || !filepath.IsAbs(root) {
				return "", errors.New("git returned no absolute worktree root")
			}
			return filepath.Clean(root), nil
		},
		newPromoter: func(executable string) promoter {
			publicClient := &http.Client{Timeout: 2 * time.Minute}
			return managedpackpromotion.NewModule(
				githubacquisition.New(githubsource.New(publicClient)),
				offlinevalidation.New(executable),
				repositorycandidate.New(),
				githubproposal.NewCLI(),
			)
		},
	}
}
