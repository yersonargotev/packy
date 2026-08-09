package toolbin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPATHResolverObservesAnyDeclaredToolWithoutExecutingIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic-helper")
	if err := os.WriteFile(path, []byte("not executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	resolver := NewPATHResolver(func(name string) (string, error) {
		calls = append(calls, name)
		return path, nil
	})

	resolution, err := resolver.Resolve(context.Background(), "synthetic-helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "synthetic-helper" {
		t.Fatalf("PATH lookups = %#v", calls)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Available || resolution.Tool != "synthetic-helper" || resolution.Path != path || resolution.ResolvedPath != resolvedPath || resolution.Origin != "path" || resolution.Precondition == "" {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestPATHResolverReportsMissingToolAsCurrentNegativeEvidence(t *testing.T) {
	resolver := NewPATHResolver(func(name string) (string, error) {
		if name != "another-helper" {
			t.Fatalf("lookup = %q", name)
		}
		return "", errors.New("not found")
	})

	resolution, err := resolver.Resolve(context.Background(), "another-helper")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Available || resolution.Tool != "another-helper" || resolution.Origin != "path" || resolution.Precondition == "" {
		t.Fatalf("resolution = %#v", resolution)
	}
}
