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
