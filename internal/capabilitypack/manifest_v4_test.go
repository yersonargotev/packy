package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validManifestV4 = `{
  "schema_version": 4,
  "id": "example",
  "version": "1.0.0",
  "surfaces": ["codex"],
  "provides": [],
  "requires": {"capabilities": [], "tools": []},
  "conflicts": [],
  "resources": [{
    "kind": "skill",
    "id": "example",
    "source": "skills/example.md",
    "requires": [],
    "bindings": [{
      "surface": "codex",
      "projection": "skill",
      "name": "example",
      "invocation": "$example",
      "mode": "native",
      "sharing": "exclusive"
    }],
    "surface_exclusions": [],
    "runtime_modes": [{
      "id": "local",
      "role": "primary",
      "requirements": [{"kind": "tool", "id": "node", "version": ">=20.0.0"}],
      "authorities": [{"kind": "filesystem_read", "scope": "consumer_project"}],
      "effects": [],
      "fallback": {"kind": "none"},
      "on_unavailable": "fail_before_effects"
    }]
  }],
  "contract": {"exclusions": []}
}`

func writeManifestV4(t *testing.T, manifest string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pack.json")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPortableManifestV4ExposesCanonicalRuntimeModes(t *testing.T) {
	pack, err := LoadPortableManifest(writeManifestV4(t, validManifestV4), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Resources) != 1 || len(pack.Resources[0].RuntimeModes) != 1 {
		t.Fatalf("runtime modes not preserved: %#v", pack.Resources)
	}
	mode := pack.Resources[0].RuntimeModes[0]
	if mode.Requirements[0].Version != ">=20.0.0" || mode.OnUnavailable != "fail_before_effects" {
		t.Fatalf("runtime contract changed: %#v", mode)
	}
}

func TestLoadPortableManifestV4AcceptsResourceLocalVerifiedFallback(t *testing.T) {
	manifest := strings.Replace(validManifestV4,
		`"fallback": {"kind": "none"},`,
		`"fallback": {"kind": "mode", "mode": "local-fallback"},`,
		1,
	)
	manifest = strings.Replace(manifest,
		`    }]
  }],`,
		`    }, {
      "id": "local-fallback",
      "role": "fallback_only",
      "requirements": [],
      "authorities": [{"kind": "filesystem_read", "scope": "consumer_project"}],
      "effects": [],
      "fallback": {"kind": "none"},
      "on_unavailable": "fail_before_effects"
    }]
  }],`,
		1,
	)
	pack, err := LoadPortableManifest(writeManifestV4(t, manifest), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.Resources[0].RuntimeModes[0].Fallback.Mode; got != "local-fallback" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestLoadPortableManifestV4RejectsOneFactNegativeTwins(t *testing.T) {
	tests := map[string]string{
		"optional modes":               strings.Replace(validManifestV4, `"contract": {"exclusions": []}`, `"contract": {"exclusions": [], "optional_modes": []}`, 1),
		"null modes":                   strings.Replace(validManifestV4, `"runtime_modes": [{`, `"runtime_modes": null, "ignored": [{`, 1),
		"unknown role":                 strings.Replace(validManifestV4, `"role": "primary"`, `"role": "secondary"`, 1),
		"unknown requirement":          strings.Replace(validManifestV4, `"kind": "tool"`, `"kind": "credential"`, 1),
		"version on authentication":    strings.Replace(validManifestV4, `"kind": "tool"`, `"kind": "authentication"`, 1),
		"unnormalized version":         strings.Replace(validManifestV4, `">=20.0.0"`, `">= 20.0.0"`, 1),
		"null tool version":            strings.Replace(validManifestV4, `">=20.0.0"`, `null`, 1),
		"duplicate requirement":        strings.Replace(validManifestV4, `[{"kind": "tool", "id": "node", "version": ">=20.0.0"}]`, `[{"kind": "tool", "id": "node", "version": ">=20.0.0"},{"kind": "tool", "id": "node", "version": ">=20.0.0"}]`, 1),
		"unknown authority":            strings.Replace(validManifestV4, `"filesystem_read"`, `"shell"`, 1),
		"wrong authority scope":        strings.Replace(validManifestV4, `"consumer_project"`, `"planet"`, 1),
		"known but illegal scope":      strings.Replace(validManifestV4, `"consumer_project"`, `"remote_git"`, 1),
		"unknown effect":               strings.Replace(validManifestV4, `"effects": []`, `"effects": [{"kind":"shell","scope":"workstation"}]`, 1),
		"known effect wrong scope":     strings.Replace(validManifestV4, `"effects": []`, `"effects": [{"kind":"upload","scope":"consumer_project"}]`, 1),
		"fallback mode with no target": strings.Replace(validManifestV4, `{"kind": "none"}`, `{"kind": "mode", "mode": "missing"}`, 1),
		"unsafe unavailable policy":    strings.Replace(validManifestV4, `"fail_before_effects"`, `"continue"`, 1),
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadPortableManifest(writeManifestV4(t, manifest), t.TempDir()); err == nil {
				t.Fatal("expected strict v4 rejection")
			}
		})
	}
}

func TestLoadPortableManifestV4PreservesV3ResourceShapes(t *testing.T) {
	bundle, path, manifest := writeManifestV3Fixture(t)
	manifest["schema_version"] = 4
	delete(manifest["contract"].(map[string]any), "optional_modes")
	resource(manifest, "agent", "helper")["runtime_modes"] = []any{map[string]any{
		"id": "assist", "role": "primary", "requirements": []any{},
		"authorities": []any{}, "effects": []any{},
		"fallback": map[string]any{"kind": "none"}, "on_unavailable": "fail_before_effects",
	}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := LoadPortableManifest(path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range pack.Resources {
		if candidate.Kind == "lifecycle" && candidate.Bindings[0].Hook != nil {
			return
		}
	}
	t.Fatal("v4 did not preserve the v3 lifecycle hook shape")
}

func TestLoadPortableManifestV3StillForbidsRuntimeModes(t *testing.T) {
	legacy := strings.Replace(validManifestV4, `"schema_version": 4`, `"schema_version": 3`, 1)
	if _, err := LoadPortableManifest(writeManifestV4(t, legacy), t.TempDir()); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("v3 runtime_modes must remain unknown, got %v", err)
	}
}

func TestRuntimeEvidenceIsTriStateDeterministicAndSecretSafe(t *testing.T) {
	valid := `{"requirements":[{"kind":"authentication","id":"vercel","state":"unavailable","reason":"not_found","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"},{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}],"authorities":[{"kind":"network","scope":"vercel_project","state":"unverified","reason":"stale","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}]}`
	if _, err := DecodeRuntimeEvidence([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for name, encoded := range map[string]string{
		"unknown state":   strings.Replace(valid, `"available"`, `"maybe"`, 1),
		"unknown reason":  strings.Replace(valid, `"verified"`, `"present"`, 1),
		"bad time":        strings.Replace(valid, `"2026-07-25T12:00:00Z"`, `"today"`, 1),
		"sensitive field": strings.Replace(valid, `"observer_revision":"observer-v1"`, `"observer_revision":"observer-v1","token":"secret"`, 1),
		"duplicate fact":  strings.Replace(valid, `],"authorities"`, `,{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}],"authorities"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRuntimeEvidence([]byte(encoded)); err == nil {
				t.Fatal("expected evidence rejection")
			}
		})
	}
}
