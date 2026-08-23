// managedpackvalidate validates a public Managed Pack Project before release.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/managedpack"
)

type originFlags map[string]string

func (values originFlags) String() string { return "" }

func (values originFlags) Set(value string) error {
	id, root, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(root) == "" {
		return fmt.Errorf("origin must be <id>=<local-root>")
	}
	if _, exists := values[id]; exists {
		return fmt.Errorf("origin %q was provided more than once", id)
	}
	values[id] = root
	return nil
}

type resolver struct {
	local     map[string]string
	temporary string
}

func (r resolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
	if root := r.local[origin.ID]; root != "" {
		return root, nil
	}
	target := filepath.Join(r.temporary, origin.ID)
	repository, err := git.PlainCloneContext(ctx, target, false, &git.CloneOptions{
		URL:      "https://github.com/" + origin.Repository + ".git",
		Tags:     git.AllTags,
		Progress: io.Discard,
	})
	if err != nil {
		return "", fmt.Errorf("clone public origin %s: %w", origin.Repository, err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return "", fmt.Errorf("open origin worktree: %w", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(origin.Commit)}); err != nil {
		return "", fmt.Errorf("checkout origin commit %s: %w", origin.Commit, err)
	}
	return target, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("managedpackvalidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", ".", "Managed Pack Project root")
	localOrigins := originFlags{}
	flags.Var(localOrigins, "origin", "use a local origin checkout as <id>=<root>; repeatable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "managedpackvalidate accepts flags only")
		return 2
	}
	temporary, err := os.MkdirTemp("", "packy-managed-origins-")
	if err != nil {
		fmt.Fprintf(stderr, "create temporary origin directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(temporary)

	result, err := managedpack.Preflight(context.Background(), *project, resolver{local: localOrigins, temporary: temporary})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	validation := result.Validation
	fmt.Fprintf(stdout, "validated %s@%s manifest_sha256=%s closure_sha256=%s files=%d fitness_rows=%d\n",
		validation.Manifest.ID, validation.Manifest.Version, validation.ManifestSHA256, validation.ClosureSHA256,
		len(validation.Files), len(result.Fitness.Rows))
	return 0
}
