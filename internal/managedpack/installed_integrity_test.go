package managedpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestInstalledRepositoryIntegrityBindsAuthorityToHEAD(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, repositoryFixture)
		want string
	}{
		{name: "current"},
		{name: "unrelated untracked files", edit: func(t *testing.T, f repositoryFixture) {
			writeFile(t, filepath.Join(f.root, "bundle", "sources.json"), "unrelated", 0o644)
			writeFile(t, filepath.Join(f.root, "notes.txt"), "unrelated", 0o644)
		}},
		{name: "coherent manifest and admission tampering", edit: func(t *testing.T, f repositoryFixture) {
			path := filepath.Join(f.root, "bundle", "packs", "example", "pack.json")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			modified := strings.Replace(string(original), "Example Managed Pack", "Unreviewed Managed Pack", 1)
			writeFile(t, path, modified, 0o644)
			digest := sha256.Sum256([]byte(modified))
			f.record.ManifestSHA256 = hex.EncodeToString(digest[:])
			for i := range f.record.Files {
				if f.record.Files[i].Path == "pack.json" {
					f.record.Files[i].SHA256 = f.record.ManifestSHA256
				}
			}
			f.record.ClosureSHA256 = fixtureClosureDigest(f.record.Files)
			overwriteAdmission(t, f, f.record)
			if err := ValidateRepositoryIntegrity(context.Background(), f.root); err != nil {
				t.Fatalf("tampering must remain internally consistent: %v", err)
			}
		}, want: "differ from HEAD"},
		{name: "coherent registry and admission tampering", edit: func(t *testing.T, f repositoryFixture) {
			writeFile(t, filepath.Join(f.root, "managed-packs", "registry.json"), `{"schema_version":1,"packs":[{"pack_id":"example","project":"owner/unreviewed"}]}`, 0o644)
			f.record.Project = "owner/unreviewed"
			overwriteAdmission(t, f, f.record)
			if err := ValidateRepositoryIntegrity(context.Background(), f.root); err != nil {
				t.Fatalf("tampering must remain internally consistent: %v", err)
			}
		}, want: "differ"},
		{name: "registry mode", edit: func(t *testing.T, f repositoryFixture) {
			if err := os.Chmod(filepath.Join(f.root, "managed-packs", "registry.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "mode differs"},
		{name: "authority directory symlink", edit: func(t *testing.T, f repositoryFixture) {
			source := filepath.Join(f.root, "managed-packs", "admissions")
			target := filepath.Join(f.root, "moved-admissions")
			if err := os.Rename(source, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, source); err != nil {
				t.Fatal(err)
			}
		}, want: "symlink"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := writeRepositoryFixture(t)
			commitInstalledIntegrityFixture(t, f.root)
			if test.edit != nil {
				test.edit(t, f)
			}
			err := ValidateInstalledRepositoryIntegrity(context.Background(), f.root)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func commitInstalledIntegrityFixture(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	_, err = worktree.Commit("Admit reviewed catalog", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInstalledRepositoryIntegrityPinsCurrentAdmissionSelection(t *testing.T) {
	f := writeRepositoryFixture(t)
	path := filepath.Join(f.root, "bundle", "packs", "example", "pack.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	newer := strings.Replace(string(original), `"version": "1.0.0"`, `"version": "1.0.1"`, 1)
	record := f.record
	record.PackVersion = "1.0.1"
	record.Tag = "pack-v1.0.1"
	digest := sha256.Sum256([]byte(newer))
	record.ManifestSHA256 = hex.EncodeToString(digest[:])
	for i := range record.Files {
		if record.Files[i].Path == "pack.json" {
			record.Files[i].SHA256 = record.ManifestSHA256
		}
	}
	record.ClosureSHA256 = fixtureClosureDigest(record.Files)
	if _, err := WriteAdmissionRecord(filepath.Join(f.root, "managed-packs", "admissions"), record); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, newer, 0o644)
	commitInstalledIntegrityFixture(t, f.root)
	writeFile(t, path, string(original), 0o644)
	if err := ValidateRepositoryIntegrity(context.Background(), f.root); err != nil {
		t.Fatalf("previously admitted version must remain internally consistent: %v", err)
	}
	if err := ValidateInstalledRepositoryIntegrity(context.Background(), f.root); err == nil || !strings.Contains(err.Error(), "HEAD") {
		t.Fatalf("rollback to historical admission error = %v", err)
	}
}
