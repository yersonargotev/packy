// Command catalogvalidate validates an inert Catalog Project and, when given a
// baseline, enforces independent Pack version changes.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/managedpack"
)

type resolver struct {
	temporary string
	resolved  map[string]string
}

func (r *resolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
	key := origin.Repository + "\x00" + origin.Commit
	if root := r.resolved[key]; root != "" {
		return root, nil
	}
	target := filepath.Join(r.temporary, fmt.Sprintf("%x", sha256.Sum256([]byte(key))))
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
	r.resolved[key] = target
	return target, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("catalogvalidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", ".", "Catalog Project root")
	baseline := flags.String("baseline", "", "baseline Catalog Project root for Pack version validation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "catalogvalidate accepts flags only")
		return 2
	}

	temporary, err := os.MkdirTemp("", "packy-catalog-origins-")
	if err != nil {
		fmt.Fprintf(stderr, "create temporary origin directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(temporary)

	validation, err := managedpack.ValidateCatalogProject(
		context.Background(), *project, *baseline,
		&resolver{temporary: temporary, resolved: map[string]string{}},
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "validated Catalog Project packs=%d fitness_rows=%d\n", len(validation.Packs), validation.FitnessRows)
	for _, pack := range validation.Packs {
		fmt.Fprintf(stdout, "%s@%s manifest_sha256=%s closure_sha256=%s files=%d\n",
			pack.Manifest.ID, pack.Manifest.Version, pack.ManifestSHA256, pack.ClosureSHA256, len(pack.Files))
	}
	return 0
}
