// Package catalogorigin resolves exact public GitHub origins for Catalog
// Project validation commands.
package catalogorigin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/managedpack"
)

// Resolver owns temporary exact-origin checkouts for one command invocation.
type Resolver struct {
	temporary string
	resolved  map[string]string
}

// New creates an empty origin resolver.
func New() (*Resolver, error) {
	temporary, err := os.MkdirTemp("", "packy-catalog-origins-")
	if err != nil {
		return nil, fmt.Errorf("create temporary origin directory: %w", err)
	}
	return &Resolver{temporary: temporary, resolved: map[string]string{}}, nil
}

// Close removes the resolver's owned temporary checkouts.
func (r *Resolver) Close() error {
	if r == nil || r.temporary == "" {
		return nil
	}
	return os.RemoveAll(r.temporary)
}

// Resolve checks out one exact public GitHub origin commit.
func (r *Resolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
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
