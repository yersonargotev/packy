package localprojection

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestFingerprintRejectionRestoresEarlierChanges(t *testing.T) {
	for _, rejection := range []string{"mismatch", "read error"} {
		t.Run(rejection, func(t *testing.T) {
			root := t.TempDir()
			replaced := filepath.Join(root, "replaced")
			created := filepath.Join(root, "created")
			deleted := filepath.Join(root, "deleted")
			rejected := filepath.Join(root, "rejected")
			later := filepath.Join(root, "later")
			for path, content := range map[string]string{replaced: "old replacement", deleted: "old deletion", later: "later content"} {
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			change, detail := fingerprintRejectionFixture(t, rejected, rejection)
			err := ApplyFilesystemChanges([]FilesystemChange{
				{ID: "replace", Target: replaced, RegularFile: true, FileContent: []byte("new replacement")},
				{ID: "create", Target: created, RegularFile: true, FileContent: []byte("new creation")},
				{ID: "delete", Target: deleted, Delete: true},
				change,
				{ID: "later", Target: later, Delete: true},
			})
			assertFingerprintRejection(t, err, detail)
			for path, want := range map[string]string{replaced: "old replacement", deleted: "old deletion", later: "later content"} {
				if data, readErr := os.ReadFile(path); readErr != nil || string(data) != want {
					t.Errorf("%s = %q, %v; want %q", filepath.Base(path), data, readErr, want)
				}
			}
			if _, statErr := os.Lstat(created); !os.IsNotExist(statErr) {
				t.Errorf("created target remains: %v", statErr)
			}
			assertRejectedTarget(t, rejected, rejection)
			entries, readErr := os.ReadDir(root)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".packy-batch-stage-") {
					t.Errorf("transaction artifact remains: %s", entry.Name())
				}
			}
		})
	}
}

func TestFingerprintRejectionReportsRollbackFailure(t *testing.T) {
	for _, rejection := range []string{"mismatch", "read error"} {
		t.Run(rejection, func(t *testing.T) {
			root := t.TempDir()
			first := filepath.Join(root, "first")
			rejected := filepath.Join(root, "rejected")
			if err := os.WriteFile(first, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			change, detail := fingerprintRejectionFixture(t, rejected, rejection)
			injected := errors.New("injected restore failure")
			originalRename := renameTreePath
			t.Cleanup(func() { renameTreePath = originalRename })
			var backup string
			renameTreePath = func(oldPath, newPath string) error {
				if strings.HasSuffix(oldPath, ".backup") && newPath == first {
					backup = oldPath
					return injected
				}
				return originalRename(oldPath, newPath)
			}
			err := ApplyFilesystemChanges([]FilesystemChange{
				{ID: "first", Target: first, RegularFile: true, FileContent: []byte("replacement")},
				change,
			})
			assertFingerprintRejection(t, err, detail)
			if err == nil || !strings.Contains(err.Error(), "rollback failed") || !errors.Is(err, injected) {
				t.Errorf("error = %v, want wrapped rollback failure", err)
			}
			if data, readErr := os.ReadFile(backup); readErr != nil || string(data) != "original" {
				t.Errorf("recovery backup = %q, %v; want original", data, readErr)
			}
			assertRejectedTarget(t, rejected, rejection)
		})
	}
}

func fingerprintRejectionFixture(t *testing.T, target, rejection string) (FilesystemChange, string) {
	t.Helper()
	change := FilesystemChange{ID: "rejected", Target: target, Delete: true, ExpectedTargetFingerprint: "stale"}
	if rejection == "mismatch" {
		if err := os.WriteFile(target, []byte("rejected content"), 0o600); err != nil {
			t.Fatal(err)
		}
		return change, "target changed after preflight"
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(target, "link")); err != nil {
		t.Fatal(err)
	}
	change.ExactTreeTarget = true
	return change, "composite tree contains non-regular path"
}

func assertFingerprintRejection(t *testing.T, err error, detail string) {
	t.Helper()
	var actionErr capabilitypack.ProjectionActionError
	if err == nil || !errors.As(err, &actionErr) || actionErr.ID != "rejected" || !strings.Contains(err.Error(), detail) {
		t.Errorf("error = %v, want rejected action and %q", err, detail)
	}
}

func assertRejectedTarget(t *testing.T, target, rejection string) {
	t.Helper()
	if rejection == "mismatch" {
		if data, err := os.ReadFile(target); err != nil || string(data) != "rejected content" {
			t.Errorf("rejected target = %q, %v", data, err)
		}
		return
	}
	if link, err := os.Readlink(filepath.Join(target, "link")); err != nil || link != "missing" {
		t.Errorf("rejected tree link = %q, %v", link, err)
	}
}
