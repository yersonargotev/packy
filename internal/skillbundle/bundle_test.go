package skillbundle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

func TestDiscoverWaitsForCompleteBundleTransaction(t *testing.T) {
	repository := t.TempDir()
	source := filepath.Join(repository, "bundle", "skills")
	writeValidSkillSource(t, source)
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Discover(context.Background(), source, t.TempDir(), "")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Discover completed outside the shared lock: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Discover did not resume after the bundle transaction")
	}
}

func TestBundleRootOwnsPhysicalSourceLayout(t *testing.T) {
	want := filepath.Join(t.TempDir(), "bundle")
	if got := BundleRoot(filepath.Join(want, "skills")); got != want {
		t.Fatalf("BundleRoot = %q, want %q", got, want)
	}
}

func TestDiscoverReportsMissingSourceWithHint(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "bundle", "skills")
	_, err := Discover(context.Background(), missing, t.TempDir(), "run packy init to initialize it")
	if err == nil {
		t.Fatal("expected missing source error")
	}
	var missingErr MissingSourceError
	if !errors.As(err, &missingErr) {
		t.Fatalf("error = %T %v, want MissingSourceError", err, err)
	}
	if missingErr.Path != missing {
		t.Fatalf("MissingSourceError.Path = %q, want %q", missingErr.Path, missing)
	}
	for _, want := range []string{missing, "run packy init"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestDiscoverValidatesRepresentativeSkillResources(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "bundle", "skills")
	writeValidSkillSource(t, sourceRoot)
	linkDir := t.TempDir()

	skills, err := Discover(context.Background(), sourceRoot, linkDir, "")
	if err != nil {
		t.Fatalf("Discover valid source: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("Discover returned %d skills, want 3", len(skills))
	}
	for _, skill := range skills {
		if !filepath.IsAbs(skill.SourcePath) || skill.LinkPath != filepath.Join(linkDir, skill.Name) {
			t.Fatalf("invalid discovered skill: %#v", skill)
		}
	}
}

func TestDiscoverRejectsMalformedSkillResources(t *testing.T) {
	tests := []struct {
		name        string
		breakSource func(*testing.T, string)
		want        string
	}{
		{
			name:        "missing required group",
			breakSource: func(t *testing.T, root string) { t.Helper(); os.RemoveAll(filepath.Join(root, "productivity")) },
			want:        "discover productivity skills",
		},
		{
			name: "skill missing manifest",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				os.Remove(filepath.Join(root, "engineering", "ask-matt", "SKILL.md"))
			},
			want: "missing SKILL.md",
		},
		{
			name: "selected skill path is not a directory",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, "in-progress", "loop-me")
				os.RemoveAll(path)
				if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "is not a directory",
		},
		{
			name: "source root is not a directory",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(root, []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "source path is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := filepath.Join(t.TempDir(), "bundle", "skills")
			writeValidSkillSource(t, sourceRoot)
			tt.breakSource(t, sourceRoot)
			_, err := Discover(context.Background(), sourceRoot, t.TempDir(), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Discover error = %v, want containing %q", err, tt.want)
			}
			var malformed MalformedSourceError
			if !errors.As(err, &malformed) || malformed.Path != sourceRoot {
				t.Fatalf("Discover error = %T %v, want MalformedSourceError for %s", err, err, sourceRoot)
			}
		})
	}
}

func writeValidSkillSource(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"engineering/ask-matt/SKILL.md",
		"productivity/grilling/SKILL.md",
		"in-progress/loop-me/SKILL.md",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
