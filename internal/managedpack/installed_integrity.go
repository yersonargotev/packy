package managedpack

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ValidateInstalledRepositoryIntegrity binds the installed catalog's admission
// authority and current manifest selection to HEAD and validates its complete
// declared closures. The caller holds the bundle transaction through validation
// and catalog consumption.
// Unrelated worktree files and repository-maintenance policy are outside this check.
func ValidateInstalledRepositoryIntegrity(ctx context.Context, repositoryRoot string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository, err := git.PlainOpenWithOptions(repositoryRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return fmt.Errorf("open installed admission authority: %w", err)
	}
	head, err := repository.Head()
	if err != nil {
		return fmt.Errorf("read installed admission authority HEAD: %w", err)
	}
	commit, err := repository.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("read installed admission authority commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("read installed admission authority tree: %w", err)
	}
	expected := make(map[string]bool)
	err = tree.Files().ForEach(func(file *object.File) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.Name != "managed-packs/registry.json" && !strings.HasPrefix(file.Name, "managed-packs/admissions/") && !strings.HasPrefix(file.Name, "bundle/packs/") {
			return nil
		}
		expected[file.Name] = true
		if err := rejectSymlinkComponents(ctx, repositoryRoot, file.Name); err != nil {
			return err
		}
		path := filepath.Join(repositoryRoot, filepath.FromSlash(file.Name))
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || canonicalMode(info.Mode()) != fmt.Sprintf("%o", file.Mode) {
			return fmt.Errorf("installed admission authority %q mode differs from HEAD", file.Name)
		}
		if info.Size() != file.Size || info.Size() > maxIndexedFileBytes {
			return fmt.Errorf("installed admission authority %q bytes differ from HEAD", file.Name)
		}
		current, err := readFileBounded(ctx, path, info)
		if err != nil {
			return err
		}
		committed, err := file.Contents()
		if err != nil {
			return err
		}
		if !bytes.Equal(current, []byte(committed)) {
			return fmt.Errorf("installed admission authority %q bytes differ from HEAD", file.Name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !expected["managed-packs/registry.json"] {
		return fmt.Errorf("installed Managed Pack registry is absent from HEAD")
	}
	if err := rejectSymlinkComponents(ctx, repositoryRoot, "managed-packs/admissions"); err != nil {
		return err
	}
	err = filepath.WalkDir(filepath.Join(repositoryRoot, "managed-packs", "admissions"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		if !expected[filepath.ToSlash(relative)] {
			return fmt.Errorf("installed admission authority %q is absent from HEAD", relative)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := rejectSymlinkComponents(ctx, repositoryRoot, "bundle/packs"); err != nil {
		return err
	}
	return validateRepositoryCatalogIntegrity(ctx, repositoryRoot)
}
