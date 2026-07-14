package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/matty/internal/workstation"
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
		{name: "default", wantRoot: filepath.Join(home, ".local", "share", "matty")},
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
