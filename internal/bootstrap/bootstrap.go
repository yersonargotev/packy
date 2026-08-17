package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

const DefaultRepositoryURL = "https://github.com/yersonargotev/packy.git"

const cleanupTimeout = 5 * time.Second

// BootstrapOptions describes how packy init prepares the package-installed
// Source of Truth checkout. It lives outside command construction so the CLI
// remains an adapter around source bootstrapping behavior.
type BootstrapOptions struct {
	InstalledSource InstalledSource
	RepositoryURL   string
	RepositoryRef   string
	HomeDir         string
	ConfigHome      string
	ReportProgress  func(string) error
}

type BootstrapResult struct {
	Cloned  bool
	Updated bool
}

func EnsureInstalledSource(ctx context.Context, opts BootstrapOptions) (BootstrapResult, error) {
	if err := ctx.Err(); err != nil {
		return BootstrapResult{}, err
	}
	if strings.TrimSpace(opts.InstalledSource.Root()) == "" {
		return BootstrapResult{}, errors.New("installed source root is required")
	}
	if strings.TrimSpace(opts.RepositoryURL) == "" {
		opts.RepositoryURL = DefaultRepositoryURL
	}

	result := BootstrapResult{}
	validationErr := validateInstalledSource(ctx, opts.InstalledSource.Root())
	if validationErr == nil {
		updated, err := ensureInstalledSourceRef(ctx, opts)
		if err != nil {
			return BootstrapResult{}, err
		}
		result.Updated = updated
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return BootstrapResult{}, err
	}

	info, err := os.Stat(opts.InstalledSource.Root())
	var emptyDestination os.FileInfo
	switch {
	case err == nil && !info.IsDir():
		return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a directory: %s", opts.InstalledSource.Root())
	case err == nil:
		empty, err := dirEmpty(opts.InstalledSource.Root())
		if err != nil {
			return BootstrapResult{}, err
		}
		if !empty {
			return BootstrapResult{}, fmt.Errorf("Installed Source path exists but is not a valid Packy checkout: %s. Move it aside or pass --source-root", opts.InstalledSource.Root())
		}
		emptyDestination = info
	case !os.IsNotExist(err):
		return BootstrapResult{}, fmt.Errorf("inspect Installed Source: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return BootstrapResult{}, fmt.Errorf("git is required to clone the Packy Source of Truth into %s", opts.InstalledSource.Root())
	}
	if err := cloneInstalledSource(ctx, opts, emptyDestination); err != nil {
		return BootstrapResult{}, err
	}
	result.Cloned = true
	return result, nil
}

func ensureInstalledSourceRef(ctx context.Context, opts BootstrapOptions) (bool, error) {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" {
		return false, nil
	}
	matches, err := repositoryRefMatches(ctx, opts, fmt.Sprintf("cannot update it to %s", ref))
	if err != nil {
		return false, err
	}
	if matches {
		return false, nil
	}
	if dirty, err := repositoryDirty(ctx, opts); err != nil {
		return false, err
	} else if dirty {
		return false, fmt.Errorf("Installed Source at %s has local changes; refusing to update to %s. Commit/stash them, move it aside, or pass --source-root", opts.InstalledSource.Root(), ref)
	}
	if err := reportProgress(opts, fmt.Sprintf("updating Installed Source at %s to %s", opts.InstalledSource.Root(), ref)); err != nil {
		return false, err
	}
	if err := fetchInstalledSourceRef(ctx, opts, ref); err != nil {
		return false, fmt.Errorf("update Installed Source to %s: %w", ref, err)
	}
	if err := validateFetchedInstalledSource(ctx, opts); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, err := gitOutput(ctx, opts, "checkout", "--detach", "FETCH_HEAD"); err != nil {
		return false, fmt.Errorf("checkout Installed Source ref %s: %w", ref, err)
	}
	return true, nil
}

func validateFetchedInstalledSource(ctx context.Context, opts BootstrapOptions) (err error) {
	validationRoot, err := os.MkdirTemp(filepath.Dir(opts.InstalledSource.Root()), ".packy-validate.*")
	if err != nil {
		return fmt.Errorf("create Installed Source validation directory: %w", err)
	}
	if err := os.Remove(validationRoot); err != nil {
		return fmt.Errorf("prepare Installed Source validation worktree: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := newCleanupContext(ctx)
		defer cancel()
		_, gitCleanupErr := gitOutput(cleanupCtx, opts, "worktree", "remove", "--force", validationRoot)
		removeErr := os.RemoveAll(validationRoot)
		var cleanupErr error
		if gitCleanupErr != nil {
			cleanupErr = fmt.Errorf("remove validation worktree: %w", gitCleanupErr)
		}
		if removeErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove validation directory: %w", removeErr))
		}
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("clean up Installed Source validation worktree: %w", cleanupErr)
			if err == nil {
				err = cleanupErr
			} else {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if _, err := gitOutput(ctx, opts, "worktree", "add", "--detach", validationRoot, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("prepare fetched Installed Source for validation: %w", err)
	}

	if err := validateInstalledSource(ctx, validationRoot); err != nil {
		return fmt.Errorf("fetched Installed Source has an invalid skill bundle: %w", err)
	}
	return nil
}

func newCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
}

func fetchInstalledSourceRef(ctx context.Context, opts BootstrapOptions, ref string) error {
	if strings.HasPrefix(ref, "v") {
		tagRef := "refs/tags/" + ref
		if _, err := gitOutput(ctx, opts, "fetch", "--depth", "1", "origin", tagRef+":"+tagRef); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return err
		}
	}
	_, err := gitOutput(ctx, opts, "fetch", "--depth", "1", "origin", ref)
	return err
}

func ValidateInstalledSourceRef(ctx context.Context, opts BootstrapOptions) error {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" {
		return nil
	}
	if strings.TrimSpace(opts.InstalledSource.Root()) == "" {
		return errors.New("installed source root is required")
	}
	if err := validateInstalledSource(ctx, opts.InstalledSource.Root()); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("default Installed Source is missing or invalid at %s; run packy init to initialize it", skillbundle.InstalledSourceRoot(opts.InstalledSource))
	}
	matches, err := repositoryRefMatchesReadOnly(opts, fmt.Sprintf("run packy init to align it with %s", ref))
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("default Installed Source at %s is stale for Packy %s; run packy init to align it before managing capability packs", opts.InstalledSource.Root(), ref)
	}
	return nil
}

// repositoryRefMatchesReadOnly validates the already-installed checkout
// without invoking Git. Lifecycle Preview uses this path so dry-runs never
// execute commands; mutating bootstrap operations continue to use Git itself.
func repositoryRefMatchesReadOnly(opts BootstrapOptions, missingGitReason string) (bool, error) {
	repository, err := git.PlainOpenWithOptions(opts.InstalledSource.Root(), &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return false, fmt.Errorf("Installed Source at %s is not a git checkout; %s. Move it aside or pass --source-root", opts.InstalledSource.Root(), missingGitReason)
		}
		return false, fmt.Errorf("inspect Installed Source git metadata: %w", err)
	}
	head, err := repository.Head()
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source HEAD: %w", err)
	}
	target, err := repository.ResolveRevision(plumbing.Revision(opts.RepositoryRef + "^{commit}"))
	if err != nil {
		return false, nil
	}
	return head.Hash() == *target, nil
}

func repositoryRefMatches(ctx context.Context, opts BootstrapOptions, missingGitReason string) (bool, error) {
	if _, err := os.Stat(filepath.Join(opts.InstalledSource.Root(), ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("Installed Source at %s is not a git checkout; %s. Move it aside or pass --source-root", opts.InstalledSource.Root(), missingGitReason)
		}
		return false, fmt.Errorf("inspect Installed Source git metadata: %w", err)
	}
	return repositoryAtRef(ctx, opts)
}

func repositoryAtRef(ctx context.Context, opts BootstrapOptions) (bool, error) {
	head, err := gitOutput(ctx, opts, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source HEAD: %w", err)
	}
	target, err := gitOutput(ctx, opts, "rev-parse", "--verify", opts.RepositoryRef+"^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(head) == strings.TrimSpace(target), nil
}

func repositoryDirty(ctx context.Context, opts BootstrapOptions) (bool, error) {
	status, err := gitOutput(ctx, opts, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("inspect Installed Source status: %w", err)
	}
	return strings.TrimSpace(status) != "", nil
}

func cloneInstalledSource(ctx context.Context, opts BootstrapOptions, emptyDestination os.FileInfo) (err error) {
	parent := filepath.Dir(opts.InstalledSource.Root())
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Installed Source parent: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, ".packy-clone.*")
	if err != nil {
		return fmt.Errorf("create temporary clone directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(tmp); cleanupErr != nil {
			cleanupErr = fmt.Errorf("clean up temporary Installed Source clone: %w", cleanupErr)
			if err == nil {
				err = cleanupErr
			} else {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()

	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(opts.RepositoryRef) != "" {
		args = append(args, "--branch", opts.RepositoryRef)
	}
	args = append(args, opts.RepositoryURL, tmp)
	if err := reportProgress(opts, fmt.Sprintf("cloning Installed Source into %s", opts.InstalledSource.Root())); err != nil {
		return err
	}
	if _, err := runGit(ctx, opts, args...); err != nil {
		return fmt.Errorf("clone Packy Source of Truth: %w", err)
	}
	if err := validateInstalledSource(ctx, tmp); err != nil {
		return fmt.Errorf("cloned Packy Source of Truth has an invalid skill bundle: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishClonedInstalledSource(tmp, opts.InstalledSource.Root(), emptyDestination, renameInstalledSourceNoReplace); err != nil {
		return err
	}
	return nil
}

func publishClonedInstalledSource(tmp, root string, emptyDestination os.FileInfo, rename func(string, string) error) error {
	if emptyDestination != nil {
		if err := consumeEmptyInstalledSource(root, emptyDestination); err != nil {
			return err
		}
	}
	if err := rename(tmp, root); err != nil {
		installErr := fmt.Errorf("install cloned Packy Source of Truth: %w", err)
		if emptyDestination == nil {
			return installErr
		}
		if restoreErr := restoreEmptyInstalledSource(root, emptyDestination.Mode().Perm()); restoreErr != nil {
			return errors.Join(installErr, restoreErr)
		}
		return installErr
	}
	return nil
}

func consumeEmptyInstalledSource(root string, expected os.FileInfo) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("revalidate empty Installed Source directory: %w", err)
	}
	if !info.IsDir() || !os.SameFile(expected, info) {
		return fmt.Errorf("Installed Source path changed during initialization: %s", root)
	}
	empty, err := dirEmpty(root)
	if err != nil {
		return err
	}
	if !empty {
		return fmt.Errorf("Installed Source path changed during initialization: %s", root)
	}
	if err := os.Remove(root); err != nil {
		return fmt.Errorf("remove empty Installed Source directory for publication: %w", err)
	}
	return nil
}

func restoreEmptyInstalledSource(root string, mode os.FileMode) error {
	if err := os.Mkdir(root, mode); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("restore empty Installed Source directory: %w", err)
	}
	if err := os.Chmod(root, mode); err != nil {
		return fmt.Errorf("restore empty Installed Source directory permissions: %w", err)
	}
	return nil
}

func reportProgress(opts BootstrapOptions, message string) error {
	if opts.ReportProgress == nil {
		return nil
	}
	if err := opts.ReportProgress(message); err != nil {
		return fmt.Errorf("report initialization progress: %w", err)
	}
	return nil
}

func gitOutput(ctx context.Context, opts BootstrapOptions, args ...string) (string, error) {
	return runGit(ctx, opts, append([]string{"-C", opts.InstalledSource.Root()}, args...)...)
}

func runGit(ctx context.Context, opts BootstrapOptions, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnv(opts)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", fmt.Errorf("git %s canceled: %w\n%s", strings.Join(args, " "), contextErr, strings.TrimSpace(string(output)))
		}
		return "", fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func gitEnv(opts BootstrapOptions) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_CONFIG_NOSYSTEM=1")
	if opts.HomeDir != "" {
		env = append(env, "HOME="+opts.HomeDir)
	}
	if opts.ConfigHome != "" {
		env = append(env, "XDG_CONFIG_HOME="+opts.ConfigHome)
	}
	return env
}

func validateInstalledSource(ctx context.Context, dir string) error {
	return skillbundle.ValidateSource(ctx, skillbundle.SourceRoot(dir), "")
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read Installed Source directory: %w", err)
	}
	return len(entries) == 0, nil
}
