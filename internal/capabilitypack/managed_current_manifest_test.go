package capabilitypack

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadCurrentManifestLoadsMaterializedManagedPackWithoutChangingManifest(t *testing.T) {
	bundleRoot := t.TempDir()
	manifestPath, manifestBytes := writeManagedCurrentManifestFixture(t, bundleRoot)

	pack, err := LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, manifestBytes) {
		t.Fatalf("managed manifest bytes changed during load\nbefore: %s\nafter:  %s", manifestBytes, after)
	}
	if pack.ID != "managed-loader" || pack.Version != "2.3.4" || pack.Description != "Managed loader fixture" || !pack.Selectable {
		t.Fatalf("Pack identity and selection semantics = %#v", pack)
	}
	if got, want := pack.Surfaces, []Surface{SurfaceCodex}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces = %#v, want %#v", got, want)
	}
	if got, want := pack.ReadinessObligations, []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}; !reflect.DeepEqual(got, want) {
		t.Fatalf("readiness obligations = %#v, want %#v", got, want)
	}
	if got, want := pack.Requires.Tools, []string{"example-tool"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("external requirements = %#v, want %#v", got, want)
	}
	if got, want := pack.Resources, []Resource{
		{
			Kind: "instruction", ID: "guide", Source: "instructions/managed-loader.md",
			Description: "Projects the managed guidance", Requires: []string{}, Conflicts: []string{},
			RequiresTools: []string{}, Notices: []string{"notice:upstream-license"},
			Bindings: []Binding{{
				Surface: SurfaceCodex, Projection: "instruction", Name: "guide", Invocation: "guide",
				Mode: "native", Sharing: "shared", Capabilities: []SurfaceCapability{{
					Type:               SurfaceCapabilityProjectInstruction,
					ProjectInstruction: &ProjectInstructionCapability{ID: "guide", Source: "instructions/managed-loader.md"},
				}},
			}},
			SurfaceExclusions: []SurfaceExclusion{},
		},
		{
			Kind: "notice", ID: "upstream-license", Source: "notices/upstream-license",
			Description: "Preserves the upstream license", License: "MIT", Attribution: "Example Authors",
			Requires: []string{}, Conflicts: []string{}, RequiresTools: []string{}, Bindings: []Binding{},
			SurfaceExclusions: []SurfaceExclusion{},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resources = %#v, want %#v", got, want)
	}
}

func TestLoadCurrentManifestRejectsInvalidManagedPackWire(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "wrong schema",
			mutate: func(manifest string) string {
				return strings.Replace(manifest, `"schema_version": 1`, `"schema_version": 2`, 1)
			},
			wantErr: "schema_version must be 1",
		},
		{
			name: "legacy exclusions",
			mutate: func(manifest string) string {
				return strings.Replace(manifest, `  "origins": [`, "  \"exclusions\": [],\n  \"origins\": [", 1)
			},
			wantErr: `unknown field "exclusions"`,
		},
		{
			name: "legacy source reference",
			mutate: func(manifest string) string {
				return strings.Replace(manifest, `  "origins": [`, "  \"source_reference\": {\"repository\": \"example/legacy\", \"revision\": \"v1\"},\n  \"origins\": [", 1)
			},
			wantErr: `unknown field "source_reference"`,
		},
		{
			name: "unknown origin field",
			mutate: func(manifest string) string {
				return strings.Replace(manifest, `      "revision": "v2.3.4"`, "      \"revision\": \"v2.3.4\",\n      \"branch\": \"main\"", 1)
			},
			wantErr: `unknown field "branch"`,
		},
		{
			name: "unknown resource origin field",
			mutate: func(manifest string) string {
				return strings.Replace(manifest, `        "relationship": "adapted"`, "        \"relationship\": \"adapted\",\n        \"subpath\": \"extra\"", 1)
			},
			wantErr: `unknown field "subpath"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundleRoot := t.TempDir()
			manifestPath, manifestBytes := writeManagedCurrentManifestFixture(t, bundleRoot)
			invalid := test.mutate(string(manifestBytes))
			if invalid == string(manifestBytes) {
				t.Fatal("fixture mutation did not change the manifest")
			}
			if err := os.WriteFile(manifestPath, []byte(invalid), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := LoadCurrentManifest(manifestPath, bundleRoot, false)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("managed manifest error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadCurrentManifestRejectsManifestWithoutManagedSchemaVersion(t *testing.T) {
	bundleRoot := t.TempDir()
	manifestPath := filepath.Join(bundleRoot, "packs", "missing-schema", "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"id":"missing-schema"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCurrentManifest(manifestPath, bundleRoot, true)
	if err == nil || !strings.Contains(err.Error(), "Managed Pack schema_version is required") {
		t.Fatalf("manifest error = %v, want missing Managed Pack schema_version", err)
	}
}

func writeManagedCurrentManifestFixture(t *testing.T, bundleRoot string) (string, []byte) {
	t.Helper()
	manifestPath := filepath.Join(bundleRoot, "packs", "managed-loader", "pack.json")
	for path, content := range map[string]string{
		filepath.Join(bundleRoot, "instructions", "managed-loader.md"): "# Managed guidance\n",
		filepath.Join(bundleRoot, "notices", "upstream-license"):       "MIT License\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
  "schema_version": 1,
  "id": "managed-loader",
  "version": "2.3.4",
  "description": "Managed loader fixture",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": ["example-tool"],
  "origins": [
    {
      "id": "upstream",
      "repository": "example/upstream",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "revision": "v2.3.4"
    }
  ],
  "resources": [
    {
      "kind": "instruction",
      "id": "guide",
      "source": "instructions/managed-loader.md",
      "description": "Projects the managed guidance",
      "requires": [],
      "conflicts": [],
      "notices": ["notice:upstream-license"],
      "origin": {
        "id": "upstream",
        "path": "guidance.md",
        "relationship": "adapted"
      },
      "bindings": [
        {
          "surface": "codex",
          "projection": "instruction",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": [
            {
              "type": "project-instruction",
              "project_instruction": {
                "id": "guide",
                "source": "instructions/managed-loader.md"
              }
            }
          ]
        }
      ],
      "surface_exclusions": []
    },
    {
      "kind": "notice",
      "id": "upstream-license",
      "source": "notices/upstream-license",
      "description": "Preserves the upstream license",
      "license": "MIT",
      "attribution": "Example Authors",
      "requires": [],
      "conflicts": [],
      "bindings": [],
      "surface_exclusions": []
    }
  ]
}
`)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, manifest
}
