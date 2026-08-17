package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/workstation"
)

func TestResolveInstalledSource(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	absoluteRoot := filepath.Join(t.TempDir(), "installed")
	snapshot, err := workstation.Resolve(workstation.Inputs{Home: home, CurrentDirectory: cwd}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		explicitRoot string
		wantRoot     string
	}{
		{name: "default", wantRoot: filepath.Join(home, ".local", "share", "packy")},
		{name: "absolute override", explicitRoot: absoluteRoot, wantRoot: absoluteRoot},
		{name: "relative override", explicitRoot: filepath.Join("relative", "installed"), wantRoot: filepath.Join(cwd, "relative", "installed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed, err := ResolveInstalledSource(snapshot, tt.explicitRoot)
			if err != nil {
				t.Fatalf("ResolveInstalledSource: %v", err)
			}
			if installed.Root() != tt.wantRoot {
				t.Fatalf("Root = %q; want %q", installed.Root(), tt.wantRoot)
			}
			if installed.BundleRoot() != filepath.Join(tt.wantRoot, "bundle") {
				t.Fatalf("BundleRoot = %q", installed.BundleRoot())
			}
		})
	}
}

func TestResolveInstalledSourceNeedsCurrentDirectoryOnlyForRelativeRoot(t *testing.T) {
	home := t.TempDir()
	wantErr := errors.New("cwd unavailable")
	snapshot, err := workstation.Resolve(workstation.Inputs{
		Home:                home,
		CurrentDirectoryErr: wantErr,
	}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}

	absoluteRoot := filepath.Join(t.TempDir(), "installed")
	installed, err := ResolveInstalledSource(snapshot, absoluteRoot)
	if err != nil || installed.Root() != absoluteRoot {
		t.Fatalf("absolute root = %q, %v", installed.Root(), err)
	}
	if _, err := ResolveInstalledSource(snapshot, filepath.Join("relative", "installed")); !errors.Is(err, wantErr) {
		t.Fatalf("relative root error = %v; want %v", err, wantErr)
	}
}

func TestInstalledSourceAtOwnsCheckoutAndBundleLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	installed := InstalledSourceAt(root)
	if installed.Root() != root {
		t.Fatalf("Root = %q, want %q", installed.Root(), root)
	}
	if installed.BundleRoot() != filepath.Join(root, "bundle") {
		t.Fatalf("BundleRoot = %q", installed.BundleRoot())
	}
}

func TestValidateInstalledSourceRefUsesDescriptor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "descriptor-source")
	err := ValidateInstalledSourceRef(context.Background(), BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryRef:   "v1.2.3",
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(root, "bundle", "skills")) {
		t.Fatalf("ValidateInstalledSourceRef error = %v, want descriptor path", err)
	}
}

func TestEnsureInstalledSourceCancelsGitCloneWithoutInstallingPartialSource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := EnsureInstalledSource(ctx, BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         t.TempDir(),
		ConfigHome:      t.TempDir(),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EnsureInstalledSource error = %v; want context deadline", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("canceled clone installed source: %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".packy-clone.") {
			t.Fatalf("canceled clone left temporary directory %s", entry.Name())
		}
	}
}

func TestEnsureInstalledSourceCloneFailurePreservesExistingEmptySource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	if err := os.Mkdir(root, 0o711); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o711); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\necho 'simulated clone failure' >&2\nexit 23\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := EnsureInstalledSource(context.Background(), BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         home,
		ConfigHome:      configHome,
	})
	if err == nil || !strings.Contains(err.Error(), "simulated clone failure") {
		t.Fatalf("EnsureInstalledSource error = %v; want clone failure", err)
	}
	info, statErr := os.Stat(root)
	if statErr != nil {
		t.Fatalf("stat preserved Installed Source: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatalf("preserved Installed Source mode = %v; want directory", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o711 {
		t.Fatalf("preserved Installed Source permissions = %o; want 711", got)
	}
	if empty, readErr := dirEmpty(root); readErr != nil || !empty {
		t.Fatalf("preserved Installed Source empty = %t, %v; want true", empty, readErr)
	}
	assertNoBootstrapDirectories(t, parent)
}

func TestEnsureInstalledSourcePrepublicationFailuresPreserveExistingEmptySource(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(*testing.T, string) BootstrapOptions
		wantError string
	}{
		{
			name: "missing Git",
			prepare: func(t *testing.T, root string) BootstrapOptions {
				t.Setenv("PATH", t.TempDir())
				return BootstrapOptions{InstalledSource: InstalledSourceAt(root)}
			},
			wantError: "git is required",
		},
		{
			name: "progress reporting",
			prepare: func(t *testing.T, root string) BootstrapOptions {
				bin := t.TempDir()
				writeSuccessfulCloneGit(t, bin, "")
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				return BootstrapOptions{
					InstalledSource: InstalledSourceAt(root),
					ReportProgress: func(string) error {
						return errors.New("simulated progress failure")
					},
				}
			},
			wantError: "simulated progress failure",
		},
		{
			name: "staged source validation",
			prepare: func(t *testing.T, root string) BootstrapOptions {
				bin := t.TempDir()
				git := filepath.Join(bin, "git")
				script := `#!/bin/sh
for argument in "$@"; do
  destination="$argument"
done
mkdir -p "$destination/bundle"
`
				if err := os.WriteFile(git, []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				return BootstrapOptions{InstalledSource: InstalledSourceAt(root)}
			},
			wantError: "invalid skill bundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "installed")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			configHome := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", configHome)
			opts := tt.prepare(t, root)
			opts.HomeDir = home
			opts.ConfigHome = configHome

			_, err := EnsureInstalledSource(context.Background(), opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("EnsureInstalledSource error = %v; want %q", err, tt.wantError)
			}
			if empty, readErr := dirEmpty(root); readErr != nil || !empty {
				t.Fatalf("preserved Installed Source empty = %t, %v; want true", empty, readErr)
			}
			assertNoBootstrapDirectories(t, parent)
		})
	}
}

func TestEnsureInstalledSourceCancellationPreservesExistingEmptySource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := EnsureInstalledSource(ctx, BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         home,
		ConfigHome:      configHome,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EnsureInstalledSource error = %v; want context deadline", err)
	}
	if empty, readErr := dirEmpty(root); readErr != nil || !empty {
		t.Fatalf("preserved Installed Source empty = %t, %v; want true", empty, readErr)
	}
	assertNoBootstrapDirectories(t, parent)
}

func TestEnsureInstalledSourcePublishesCloneOverExistingEmptySource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	writeSuccessfulCloneGit(t, bin, "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	result, err := EnsureInstalledSource(context.Background(), BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         home,
		ConfigHome:      configHome,
	})
	if err != nil {
		t.Fatalf("EnsureInstalledSource: %v", err)
	}
	if !result.Cloned {
		t.Fatalf("EnsureInstalledSource result = %+v; want cloned", result)
	}
	published, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(original, published) {
		t.Fatal("successful clone retained the consumed empty directory")
	}
	if err := validateInstalledSource(context.Background(), root); err != nil {
		t.Fatalf("published Installed Source: %v", err)
	}
	assertNoBootstrapDirectories(t, parent)
}

func TestEnsureInstalledSourceDoesNotConsumeDestinationChangedDuringClone(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	writeSuccessfulCloneGit(t, bin, `touch "$PACKY_SOURCE_ROOT/concurrent"`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PACKY_SOURCE_ROOT", root)
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	_, err := EnsureInstalledSource(context.Background(), BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         home,
		ConfigHome:      configHome,
	})
	if err == nil || !strings.Contains(err.Error(), "changed during initialization") {
		t.Fatalf("EnsureInstalledSource error = %v; want concurrent destination change", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "concurrent")); statErr != nil {
		t.Fatalf("concurrent Installed Source content: %v", statErr)
	}
	assertNoBootstrapDirectories(t, parent)
}

func TestEnsureInstalledSourceDoesNotConsumeReplacedEmptyDestination(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	writeSuccessfulCloneGit(t, bin, `rmdir "$PACKY_SOURCE_ROOT"
mkdir "$PACKY_SOURCE_ROOT"`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PACKY_SOURCE_ROOT", root)
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	_, err := EnsureInstalledSource(context.Background(), BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryURL:   "https://example.invalid/packy.git",
		HomeDir:         home,
		ConfigHome:      configHome,
	})
	if err == nil || !strings.Contains(err.Error(), "changed during initialization") {
		t.Fatalf("EnsureInstalledSource error = %v; want replaced destination error", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("replacement Installed Source: %v", statErr)
	}
	assertNoBootstrapDirectories(t, parent)
}

func TestPublishClonedInstalledSourceRestoresConsumedDirectoryAfterRenameFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	if err := os.Mkdir(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(filepath.Dir(root), ".packy-clone.fixture")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	renameErr := errors.New("simulated rename failure")
	renameCalls := 0

	err = publishClonedInstalledSource(tmp, root, emptyDestination, func(source, destination string) error {
		renameCalls++
		if renameCalls == 2 {
			return renameErr
		}
		return renameInstalledSourceNoReplace(source, destination)
	})
	if !errors.Is(err, renameErr) {
		t.Fatalf("publishClonedInstalledSource error = %v; want rename failure", err)
	}
	info, statErr := os.Stat(root)
	if statErr != nil {
		t.Fatalf("restored Installed Source: %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o777 {
		t.Fatalf("restored Installed Source permissions = %o; want 777", got)
	}
	if empty, readErr := dirEmpty(root); readErr != nil || !empty {
		t.Fatalf("restored Installed Source empty = %t, %v; want true", empty, readErr)
	}
}

func TestPublishClonedInstalledSourceDoesNotOverwriteConcurrentReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(filepath.Dir(root), ".packy-clone.fixture")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	var replacement os.FileInfo
	renameCalls := 0

	err = publishClonedInstalledSource(tmp, root, emptyDestination, func(source, destination string) error {
		renameCalls++
		if renameCalls == 2 {
			if mkdirErr := os.Mkdir(destination, 0o750); mkdirErr != nil {
				t.Fatal(mkdirErr)
			}
			var statErr error
			replacement, statErr = os.Stat(destination)
			if statErr != nil {
				t.Fatal(statErr)
			}
		}
		return renameInstalledSourceNoReplace(source, destination)
	})
	if err == nil {
		t.Fatal("publishClonedInstalledSource succeeded over concurrent replacement")
	}
	preserved, statErr := os.Stat(root)
	if statErr != nil {
		t.Fatalf("concurrent Installed Source replacement: %v", statErr)
	}
	if !os.SameFile(replacement, preserved) {
		t.Fatal("concurrent Installed Source replacement was overwritten")
	}
}

func TestPublishClonedInstalledSourceRestoresDestinationReplacedBeforeHold(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(filepath.Dir(root), ".packy-clone.fixture")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	var replacement os.FileInfo
	renameCalls := 0

	err = publishClonedInstalledSource(tmp, root, emptyDestination, func(source, destination string) error {
		renameCalls++
		if renameCalls == 1 {
			if removeErr := os.Remove(source); removeErr != nil {
				t.Fatal(removeErr)
			}
			if mkdirErr := os.Mkdir(source, 0o750); mkdirErr != nil {
				t.Fatal(mkdirErr)
			}
			var statErr error
			replacement, statErr = os.Stat(source)
			if statErr != nil {
				t.Fatal(statErr)
			}
		}
		return renameInstalledSourceNoReplace(source, destination)
	})
	if err == nil || !strings.Contains(err.Error(), "changed during initialization") {
		t.Fatalf("publishClonedInstalledSource error = %v; want concurrent destination change", err)
	}
	preserved, statErr := os.Stat(root)
	if statErr != nil {
		t.Fatalf("restored concurrent Installed Source replacement: %v", statErr)
	}
	if !os.SameFile(replacement, preserved) {
		t.Fatal("concurrent Installed Source replacement was not restored")
	}
}

func TestPublishClonedInstalledSourceJoinsRestorationFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	root := filepath.Join(parent, "installed")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(parent, ".packy-clone.fixture")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	renameErr := errors.New("simulated rename failure")
	restoreErr := errors.New("simulated restoration failure")
	renameCalls := 0

	err = publishClonedInstalledSource(tmp, root, emptyDestination, func(source, destination string) error {
		renameCalls++
		switch renameCalls {
		case 2:
			return renameErr
		case 3:
			return restoreErr
		default:
			return renameInstalledSourceNoReplace(source, destination)
		}
	})
	if !errors.Is(err, renameErr) || !errors.Is(err, restoreErr) || !strings.Contains(err.Error(), "restore consumed empty Installed Source directory") {
		t.Fatalf("publishClonedInstalledSource error = %v; want rename and restoration failures", err)
	}
}

func writeSuccessfulCloneGit(t *testing.T, bin, beforeExit string) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "source")
	writeValidInstalledSource(t, fixture)
	t.Setenv("PACKY_TEST_VALID_SOURCE", fixture)
	script := `#!/bin/sh
for argument in "$@"; do
  destination="$argument"
done
cp -R "$PACKY_TEST_VALID_SOURCE/." "$destination"
` + beforeExit + "\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertNoBootstrapDirectories(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".packy-clone.") || strings.HasPrefix(entry.Name(), ".packy-consumed.") {
			t.Fatalf("bootstrap left temporary directory %s", entry.Name())
		}
	}
}

func TestEnsureInstalledSourceDoesNotCheckoutAfterCanceledFetch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	writeValidInstalledSource(t, root)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	checkoutMarker := filepath.Join(t.TempDir(), "checkout")
	git := filepath.Join(bin, "git")
	script := `#!/bin/sh
shift 2
case "$1" in
  rev-parse)
    case "$3" in
      HEAD) echo old ;;
      *) echo new ;;
    esac
    ;;
  status) exit 0 ;;
  fetch) exec sleep 300 ;;
  checkout) touch "$PACKY_CHECKOUT_MARKER" ;;
esac
`
	if err := os.WriteFile(git, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PACKY_CHECKOUT_MARKER", checkoutMarker)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := EnsureInstalledSource(ctx, BootstrapOptions{InstalledSource: InstalledSourceAt(root), RepositoryRef: "v2.0.0"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EnsureInstalledSource error = %v; want context deadline", err)
	}
	if _, err := os.Lstat(checkoutMarker); !os.IsNotExist(err) {
		t.Fatalf("canceled fetch proceeded to checkout: %v", err)
	}
}

func TestValidationWorktreeCleanupIsIndependentBoundedAndVisible(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	cleanupCtx, cancelCleanup := newCleanupContext(parent)
	defer cancelCleanup()
	if cleanupCtx.Err() != nil {
		t.Fatalf("cleanup context inherited cancellation: %v", cleanupCtx.Err())
	}
	if _, ok := cleanupCtx.Deadline(); !ok {
		t.Fatal("cleanup context has no deadline")
	}

	root := filepath.Join(t.TempDir(), "installed")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	script := `#!/bin/sh
shift 2
if [ "$1" = worktree ] && [ "$2" = add ]; then
  validation_root="$4"
  for skill in engineering/ask-matt productivity/grilling in-progress/loop-me; do
    mkdir -p "$validation_root/bundle/skills/$skill"
    printf '%s\n' '---' 'name: fixture' '---' > "$validation_root/bundle/skills/$skill/SKILL.md"
  done
  exit 0
fi
echo 'simulated cleanup failure' >&2
exit 23
`
	if err := os.WriteFile(git, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := validateFetchedInstalledSource(context.Background(), BootstrapOptions{InstalledSource: InstalledSourceAt(root)})
	if err == nil || !strings.Contains(err.Error(), "clean up Installed Source validation worktree") || !strings.Contains(err.Error(), "simulated cleanup failure") {
		t.Fatalf("cleanup error = %v; want visible Git cleanup diagnostic", err)
	}
}

func TestCanceledValidationWorktreeAddPreservesCancellationAndCleanupFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	script := `#!/bin/sh
shift 2
if [ "$1" = worktree ] && [ "$2" = add ]; then
  exec sleep 300
fi
echo 'cleanup after canceled add failed' >&2
exit 24
`
	if err := os.WriteFile(git, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := validateFetchedInstalledSource(ctx, BootstrapOptions{InstalledSource: InstalledSourceAt(root)})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("validation error = %v; want original context deadline", err)
	}
	if !strings.Contains(err.Error(), "cleanup after canceled add failed") {
		t.Fatalf("validation error = %v; want joined cleanup diagnostic", err)
	}
	entries, readErr := os.ReadDir(filepath.Dir(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".packy-validate.") {
			t.Fatalf("canceled validation left temporary directory %s", entry.Name())
		}
	}
}

func writeValidInstalledSource(t *testing.T, root string) {
	t.Helper()
	for _, relative := range []string{
		"bundle/skills/engineering/ask-matt/SKILL.md",
		"bundle/skills/productivity/grilling/SKILL.md",
		"bundle/skills/in-progress/loop-me/SKILL.md",
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBootstrapOptionsHasOneInstalledSourceLocation(t *testing.T) {
	typeOfOptions := reflect.TypeOf(BootstrapOptions{})
	if _, found := typeOfOptions.FieldByName("SourceRoot"); found {
		t.Fatal("BootstrapOptions retains legacy SourceRoot beside InstalledSource")
	}
}
