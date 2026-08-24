package managedpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepositoryIntegrityAcceptsTheCurrentManagedPackCatalog(t *testing.T) {
	err := ValidateRepositoryIntegrity(context.Background(), filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateRepositoryIntegrityRejectsAdmissionEntriesOutsideTheRegistryModel(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, repositoryFixture)
		want string
	}{
		{
			name: "unregistered pack directory",
			edit: func(t *testing.T, fixture repositoryFixture) {
				record := fixture.record
				record.PackID = "other"
				if _, err := WriteAdmissionRecord(filepath.Join(fixture.root, "managed-packs", "admissions"), record); err != nil {
					t.Fatal(err)
				}
			},
			want: `unexpected Pack Admission Record root entry "other"`,
		},
		{
			name: "extra root file",
			edit: func(t *testing.T, fixture repositoryFixture) {
				writeFile(t, filepath.Join(fixture.root, "managed-packs", "admissions", "README.md"), "extra\n", 0o644)
			},
			want: `unexpected Pack Admission Record root entry "README.md"`,
		},
		{
			name: "extra pack entry",
			edit: func(t *testing.T, fixture repositoryFixture) {
				if err := os.Mkdir(filepath.Join(fixture.root, "managed-packs", "admissions", "example", "extra"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "must be a regular file",
		},
		{
			name: "record path identity",
			edit: func(t *testing.T, fixture repositoryFixture) {
				oldPath := filepath.Join(fixture.root, "managed-packs", "admissions", "example", "1.0.0.json")
				newPath := filepath.Join(fixture.root, "managed-packs", "admissions", "example", "2.0.0.json")
				if err := os.Rename(oldPath, newPath); err != nil {
					t.Fatal(err)
				}
			},
			want: "does not match record identity example@1.0.0",
		},
		{
			name: "record project identity",
			edit: func(t *testing.T, fixture repositoryFixture) {
				record := fixture.record
				record.Project = "owner/other"
				overwriteAdmission(t, fixture, record)
			},
			want: `project "owner/other" does not match registry project "owner/example"`,
		},
		{
			name: "malformed historical record",
			edit: func(t *testing.T, fixture repositoryFixture) {
				writeFile(t, filepath.Join(fixture.root, "managed-packs", "admissions", "example", "0.9.0.json"), "{}\n", 0o644)
			},
			want: "release_immutable is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeRepositoryFixture(t)
			test.edit(t, fixture)
			assertRepositoryIntegrityError(t, fixture.root, test.want)
		})
	}
}

func TestValidateRepositoryIntegrityRequiresRegistryAndBundleIdentitiesToMatch(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, repositoryFixture)
		want string
	}{
		{
			name: "registered Pack missing from bundle",
			edit: func(t *testing.T, fixture repositoryFixture) {
				if err := os.RemoveAll(filepath.Join(fixture.root, "bundle", "packs", "example")); err != nil {
					t.Fatal(err)
				}
			},
			want: `registered Pack "example" is missing`,
		},
		{
			name: "unregistered Pack in bundle",
			edit: func(t *testing.T, fixture repositoryFixture) {
				if err := os.Mkdir(filepath.Join(fixture.root, "bundle", "packs", "other"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: `bundled Pack "other" is not registered`,
		},
		{
			name: "manifest identity differs from directory",
			edit: func(t *testing.T, fixture repositoryFixture) {
				mutateManifest(t, filepath.Join(fixture.root, "bundle", "packs", "example"), func(manifest map[string]any) {
					manifest["id"] = "other"
				})
			},
			want: `does not match manifest identity "other"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeRepositoryFixture(t)
			test.edit(t, fixture)
			assertRepositoryIntegrityError(t, fixture.root, test.want)
		})
	}
}

func TestValidateRepositoryIntegrityRequiresTheExactCurrentAdmissionAndClosure(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, repositoryFixture)
		want string
	}{
		{
			name: "current version has no admission",
			edit: func(t *testing.T, fixture repositoryFixture) {
				mutateManifest(t, filepath.Join(fixture.root, "bundle", "packs", "example"), func(manifest map[string]any) {
					manifest["version"] = "1.0.1"
				})
			},
			want: "example@1.0.1 has no exact Pack Admission Record",
		},
		{
			name: "manifest bytes drift",
			edit: func(t *testing.T, fixture repositoryFixture) {
				mutateManifest(t, filepath.Join(fixture.root, "bundle", "packs", "example"), func(manifest map[string]any) {
					manifest["description"] = "Drifted description"
				})
			},
			want: "manifest digest does not match",
		},
		{
			name: "closure bytes drift",
			edit: func(t *testing.T, fixture repositoryFixture) {
				writeFile(t, filepath.Join(fixture.root, "bundle", "instructions", "guidance.md"), "drifted\n", 0o644)
			},
			want: "closure digest does not match",
		},
		{
			name: "closure mode drift",
			edit: func(t *testing.T, fixture repositoryFixture) {
				if err := os.Chmod(filepath.Join(fixture.root, "bundle", "instructions", "guidance.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "closure digest does not match",
		},
		{
			name: "admission file path set drift",
			edit: func(t *testing.T, fixture repositoryFixture) {
				record := fixture.record
				record.Files = append(record.Files, FileRecord{
					Path: "skills/zz-extra/SKILL.md", Mode: "100644", SHA256: strings.Repeat("a", 64),
				})
				record.ClosureSHA256 = fixtureClosureDigest(record.Files)
				overwriteAdmission(t, fixture, record)
			},
			want: "closure digest does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeRepositoryFixture(t)
			test.edit(t, fixture)
			assertRepositoryIntegrityError(t, fixture.root, test.want)
		})
	}
}

func TestValidateRepositoryIntegrityRejectsEveryRetiredGenericSourcePath(t *testing.T) {
	for _, relative := range retiredRepositoryPaths {
		t.Run(relative, func(t *testing.T) {
			fixture := writeRepositoryFixture(t)
			path := filepath.Join(fixture.root, filepath.FromSlash(relative))
			if filepath.Ext(path) == "" {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, path, "{}\n", 0o644)
			}
			assertRepositoryIntegrityError(t, fixture.root, fmt.Sprintf("retired generic Pack Source path %q", relative))
		})
	}
}

type repositoryFixture struct {
	root   string
	record AdmissionRecord
}

func writeRepositoryFixture(t *testing.T) repositoryFixture {
	t.Helper()
	repositoryRoot := t.TempDir()
	projectRoot := t.TempDir()
	writeFile(t, filepath.Join(projectRoot, "pack.json"), integrityFixtureManifest, 0o644)
	writeFile(t, filepath.Join(projectRoot, "instructions", "guidance.md"), "managed guidance\n", 0o644)

	validation, err := ValidateProject(context.Background(), projectRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := MaterializeClosure(context.Background(), projectRoot, filepath.Join(repositoryRoot, "bundle"), validation); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repositoryRoot, "managed-packs", "registry.json"), `{
  "schema_version": 1,
  "packs": [
    {"pack_id": "example", "project": "owner/example"}
  ]
}
`, 0o644)

	record := validAdmissionRecord()
	record.Project = "owner/example"
	record.ManifestSHA256 = validation.ManifestSHA256
	record.ClosureSHA256 = validation.ClosureSHA256
	record.Files = validation.Files
	if _, err := WriteAdmissionRecord(filepath.Join(repositoryRoot, "managed-packs", "admissions"), record); err != nil {
		t.Fatal(err)
	}
	return repositoryFixture{root: repositoryRoot, record: record}
}

const integrityFixtureManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [
    {
      "kind": "instruction",
      "id": "guidance",
      "source": "instructions/guidance.md",
      "description": "Explains the reviewed guidance",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "instruction",
          "name": "guidance",
          "invocation": "guidance",
          "mode": "native",
          "sharing": "shared",
          "capabilities": [
            {
              "type": "project-instruction",
              "project_instruction": {
                "id": "guidance",
                "source": "instructions/guidance.md"
              }
            }
          ]
        }
      ],
      "surface_exclusions": []
    }
  ]
}
`

func overwriteAdmission(t *testing.T, fixture repositoryFixture, record AdmissionRecord) {
	t.Helper()
	data, err := MarshalAdmissionRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(fixture.root, "managed-packs", "admissions", "example", "1.0.0.json"), string(data), 0o644)
}

func assertRepositoryIntegrityError(t *testing.T, root, want string) {
	t.Helper()
	err := ValidateRepositoryIntegrity(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func fixtureClosureDigest(files []FileRecord) string {
	digest := sha256.New()
	for _, file := range files {
		fmt.Fprintf(digest, "%s\x00%s\x00%s\n", file.Path, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(digest.Sum(nil))
}
