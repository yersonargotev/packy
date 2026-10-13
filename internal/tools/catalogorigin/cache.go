package catalogorigin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogtransaction"
)

const retention = 7 * 24 * time.Hour
const maxEntries = 8

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var entryName = regexp.MustCompile(`^[0-9a-f]{64}$`)

type seal struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Digest     string `json:"digest"`
}

func (r *Resolver) resolve(ctx context.Context, origin cataloglayout.Origin) (string, error) {
	if !repositoryName.MatchString(origin.Repository) || !fullCommit.MatchString(origin.Commit) {
		return "", errors.New("origin requires an exact owner/repository and full lowercase commit")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate origin cache: %w", err)
	}
	root := filepath.Join(cache, "packy", "catalog-origins-v1")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err := realDirectory(root); err != nil {
		return "", err
	}
	guard, err := catalogtransaction.Acquire(ctx, root)
	if err != nil {
		return "", err
	}
	defer guard.Release()
	if err := prune(ctx, root); err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(origin.Repository+"\x00"+origin.Commit)))
	entry := filepath.Join(root, key)
	if _, err := os.Lstat(entry); errors.Is(err, os.ErrNotExist) {
		if err := r.acquire(ctx, origin, root, entry); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	target, err := os.MkdirTemp(r.temporary, "checkout-")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(target)
		}
	}()
	if err := copyVerified(ctx, entry, target, origin); err != nil {
		return "", fmt.Errorf("origin cache entry %s is invalid; remove this entry while authoring commands are idle and retry: %w", entry, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now := time.Now()
	if err := os.Chtimes(entry, now, now); err != nil {
		return "", err
	}
	if err := prune(ctx, root); err != nil {
		return "", err
	}
	success = true
	return target, nil
}

func (r *Resolver) acquire(ctx context.Context, origin cataloglayout.Origin, root, entry string) error {
	stage, err := os.MkdirTemp(root, "stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	checkout := filepath.Join(stage, "checkout")
	repository, err := git.PlainCloneContext(ctx, checkout, false, &git.CloneOptions{
		URL: "https://github.com/" + origin.Repository + ".git", Tags: git.AllTags, CABundle: r.caBundle, Progress: io.Discard,
	})
	if err != nil {
		return fmt.Errorf("clone public origin %s: %w", origin.Repository, err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(origin.Commit)}); err != nil {
		return fmt.Errorf("checkout origin commit %s: %w", origin.Commit, err)
	}
	digest, err := copyTree(ctx, checkout, "")
	if err != nil {
		return err
	}
	data, err := json.Marshal(seal{origin.Repository, origin.Commit, digest})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "seal.json"), data, 0600); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(stage, entry)
}

func copyVerified(ctx context.Context, entry, target string, origin cataloglayout.Origin) error {
	if err := realDirectory(entry); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(entry, "seal.json"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return errors.New("invalid origin seal")
	}
	data, err := os.ReadFile(filepath.Join(entry, "seal.json"))
	if err != nil {
		return err
	}
	var expected seal
	if err := json.Unmarshal(data, &expected); err != nil {
		return err
	}
	if expected.Repository != origin.Repository || expected.Commit != origin.Commit {
		return errors.New("origin seal identity mismatch")
	}
	digest, err := copyTree(ctx, filepath.Join(entry, "checkout"), target)
	if err != nil {
		return err
	}
	if digest != expected.Digest {
		return errors.New("origin content digest mismatch")
	}
	return nil
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("origin directory %s is not a real directory", path)
	}
	return nil
}

// prune runs only under the cache-wide lock, so stages are abandoned and private
// consumer copies remain valid even when a retained entry is evicted.
func prune(ctx context.Context, root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type retained struct {
		name string
		used time.Time
	}
	var kept []retained
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(root, entry.Name())
		if strings.HasPrefix(entry.Name(), "stage-") {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		if !entryName.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) > retention {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		kept = append(kept, retained{entry.Name(), info.ModTime()})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].used.After(kept[j].used) })
	for i := maxEntries; i < len(kept); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(root, kept[i].name)); err != nil {
			return err
		}
	}
	return nil
}
