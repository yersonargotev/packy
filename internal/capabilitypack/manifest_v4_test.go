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
    "provides_capabilities": [],
    "requires_capabilities": [],
    "requires_tools": [],
    "capability_conflicts": [],
    "conflicts": [],
    "notices": [],
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

func TestEncodePortableManifestV4IsDeterministicAndRoundTrips(t *testing.T) {
	pack, err := LoadPortableManifest(writeManifestV4(t, validManifestV4), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := EncodePortableManifestV4(pack)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodePortableManifestV4(pack)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || len(first) == 0 || first[len(first)-1] != '\n' || strings.HasSuffix(string(first), "\n\n") {
		t.Fatalf("producer output is not canonical and deterministic:\n%s", first)
	}
	path := writeManifestV4(t, string(first))
	roundTrip, err := LoadPortableManifest(path, t.TempDir())
	if err != nil {
		t.Fatalf("producer emitted a manifest rejected by the runtime validator: %v", err)
	}
	if roundTrip.Resources[0].RuntimeModes[0].Role != RuntimeModePrimary {
		t.Fatalf("round-trip changed runtime mode: %#v", roundTrip.Resources[0].RuntimeModes[0])
	}
	if strings.Contains(string(first), "optional_modes") {
		t.Fatal("v4 producer dual-wrote optional_modes")
	}
}

func TestLoadPortableManifestV4AcceptsResourceLocalVerifiedFallback(t *testing.T) {
	manifest := validFallbackManifestV4()
	pack, err := LoadPortableManifest(writeManifestV4(t, manifest), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.Resources[0].RuntimeModes[0].Fallback.Mode; got != "local-fallback" {
		t.Fatalf("fallback = %q", got)
	}
}

func validFallbackManifestV4() string {
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
	return manifest
}

func TestLoadPortableManifestV4RejectsInvalidFallbackGraphsAndModeOrder(t *testing.T) {
	mutate := func(edit func([]any)) string {
		var manifest map[string]any
		if err := json.Unmarshal([]byte(validFallbackManifestV4()), &manifest); err != nil {
			t.Fatal(err)
		}
		modes := manifest["resources"].([]any)[0].(map[string]any)["runtime_modes"].([]any)
		edit(modes)
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	unsorted := mutate(func(modes []any) { modes[0], modes[1] = modes[1], modes[0] })
	invalidRoleEdge := mutate(func(modes []any) {
		modes[1].(map[string]any)["role"] = "primary"
	})
	cycle := mutate(func(modes []any) {
		modes[1].(map[string]any)["fallback"] = map[string]any{"kind": "mode", "mode": "local"}
	})

	for name, test := range map[string]struct {
		manifest string
		want     string
	}{
		"unsorted modes":    {unsorted, "runtime_modes must be sorted"},
		"invalid role edge": {invalidRoleEdge, "from a primary mode"},
		"fallback cycle":    {cycle, "must be acyclic"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadPortableManifest(writeManifestV4(t, test.manifest), t.TempDir()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
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
		"unsorted requirements":        strings.Replace(validManifestV4, `[{"kind": "tool", "id": "node", "version": ">=20.0.0"}]`, `[{"kind": "tool", "id": "node", "version": ">=20.0.0"},{"kind": "authentication", "id": "vercel"}]`, 1),
		"unknown authority":            strings.Replace(validManifestV4, `"filesystem_read"`, `"shell"`, 1),
		"duplicate authority":          strings.Replace(validManifestV4, `[{"kind": "filesystem_read", "scope": "consumer_project"}]`, `[{"kind": "filesystem_read", "scope": "consumer_project"},{"kind": "filesystem_read", "scope": "consumer_project"}]`, 1),
		"unsorted authorities":         strings.Replace(validManifestV4, `[{"kind": "filesystem_read", "scope": "consumer_project"}]`, `[{"kind":"network","scope":"vercel_project"},{"kind":"filesystem_read","scope":"consumer_project"}]`, 1),
		"wrong authority scope":        strings.Replace(validManifestV4, `"consumer_project"`, `"planet"`, 1),
		"known but illegal scope":      strings.Replace(validManifestV4, `"consumer_project"`, `"remote_git"`, 1),
		"unknown effect":               strings.Replace(validManifestV4, `"effects": []`, `"effects": [{"kind":"shell","scope":"workstation"}]`, 1),
		"duplicate effect":             strings.Replace(validManifestV4, `"effects": []`, `"effects": [{"kind":"upload","scope":"deployment_payload"},{"kind":"upload","scope":"deployment_payload"}]`, 1),
		"unsorted effects":             strings.Replace(validManifestV4, `"effects": []`, `"effects": [{"kind":"upload","scope":"deployment_payload"},{"kind":"preview_deployment","scope":"vercel_project"}]`, 1),
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
	for _, encoded := range manifest["resources"].([]any) {
		resource := encoded.(map[string]any)
		resource["provides_capabilities"] = []any{}
		resource["requires_capabilities"] = []any{}
		resource["requires_tools"] = []any{}
		resource["capability_conflicts"] = []any{}
		if resource["kind"] != "notice" {
			resource["notices"] = []any{}
			resource["conflicts"] = []any{}
		}
	}
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
	encoded, err := EncodePortableManifestV4(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err = LoadPortableManifest(path, bundle)
	if err != nil {
		t.Fatalf("canonical producer lost a v3 resource field: %v", err)
	}
	for _, candidate := range pack.Resources {
		if candidate.Kind == "lifecycle" && candidate.Bindings[0].Hook != nil {
			return
		}
	}
	t.Fatal("v4 did not preserve the v3 lifecycle hook shape")
}

func TestLoadPortableManifestV4ValidatesNoticeAssociations(t *testing.T) {
	withNotice := strings.Replace(validManifestV4,
		`  "resources": [{`,
		`  "resources": [{"kind":"notice","id":"mit","source":"NOTICE","license":"MIT","attribution":"Example","requires":[],"provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[],"bindings":[],"surface_exclusions":[]},{`,
		1,
	)
	tests := map[string]string{
		"missing":         strings.Replace(validManifestV4, `"notices": []`, `"notices": ["notice:missing"]`, 1),
		"wrong kind":      strings.Replace(validManifestV4, `"notices": []`, `"notices": ["skill:example"]`, 1),
		"duplicate":       strings.Replace(withNotice, `"notices": []`, `"notices": ["notice:mit","notice:mit"]`, 1),
		"notice requires": strings.Replace(withNotice, `"requires":[]`, `"requires":["skill:example"]`, 1),
		"unsorted": strings.Replace(
			strings.Replace(withNotice, `"id":"mit"`, `"id":"apache"`, 1),
			`"notices": []`, `"notices": ["notice:mit","notice:apache"]`, 1),
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadPortableManifest(writeManifestV4(t, manifest), t.TempDir()); err == nil {
				t.Fatal("expected invalid notice association")
			}
		})
	}
}

func TestEncodePortableManifestV4RejectsNoticeOwnedAssociations(t *testing.T) {
	pack, err := LoadPortableManifest(writeManifestV4(t, validManifestV4), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pack.Resources = append(pack.Resources, Resource{
		Kind: "notice", ID: "mit", Source: "NOTICE", Requires: []string{},
		Notices: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{},
		License: "MIT", Attribution: "Example", ProvidesCapabilities: []string{},
		RequiresCapabilities: []string{}, RequiresTools: []string{}, CapabilityConflicts: []string{},
	})
	if _, err := EncodePortableManifestV4(pack); err == nil || !strings.Contains(err.Error(), "forbidden for notice resources") {
		t.Fatalf("notice-owned associations error = %v", err)
	}
}

func TestLoadPortableManifestV3StillForbidsRuntimeModes(t *testing.T) {
	legacy := strings.Replace(validManifestV4, `"schema_version": 4`, `"schema_version": 3`, 1)
	if _, err := LoadPortableManifest(writeManifestV4(t, legacy), t.TempDir()); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("v3 runtime_modes must remain unknown, got %v", err)
	}
}

func TestLoadPortableManifestV3StillForbidsNotices(t *testing.T) {
	resource := []byte(`{"kind":"instruction","id":"guide","source":"guide.md","requires":[],"notices":[],"bindings":[],"surface_exclusions":[]}`)
	if _, err := decodeResource(resource, manifestSchemaV3); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("v3 notices must remain unknown, got %v", err)
	}
}

func TestLoadPortableManifestV4ValidatesSamePackResourceConflicts(t *testing.T) {
	manifestWith := func(resources []map[string]any) string {
		var manifest map[string]any
		if err := json.Unmarshal([]byte(validManifestV4), &manifest); err != nil {
			t.Fatal(err)
		}
		encoded := make([]any, len(resources))
		for i := range resources {
			encoded[i] = resources[i]
		}
		manifest["resources"] = encoded
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	resource := func(kind, id string, requires, conflicts []string) map[string]any {
		if requires == nil {
			requires = []string{}
		}
		if conflicts == nil {
			conflicts = []string{}
		}
		return map[string]any{
			"kind": kind, "id": id, "source": id + ".md",
			"requires": requires, "conflicts": conflicts, "notices": []string{},
			"provides_capabilities": []string{}, "requires_capabilities": []string{},
			"requires_tools": []string{}, "capability_conflicts": []string{},
			"bindings": []any{}, "surface_exclusions": []any{},
		}
	}

	valid := manifestWith([]map[string]any{
		resource("asset", "alpha", nil, []string{"asset:beta"}),
		resource("asset", "beta", nil, []string{"asset:alpha"}),
	})
	if _, err := LoadPortableManifest(writeManifestV4(t, valid), t.TempDir()); err != nil {
		t.Fatalf("symmetric conflict rejected: %v", err)
	}
	mutateExample := func(edit func(map[string]any)) string {
		var manifest map[string]any
		if err := json.Unmarshal([]byte(validManifestV4), &manifest); err != nil {
			t.Fatal(err)
		}
		edit(manifest["resources"].([]any)[0].(map[string]any))
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	tests := map[string]struct {
		manifest string
		want     string
	}{
		"missing": {
			mutateExample(func(resource map[string]any) { delete(resource, "conflicts") }),
			"conflicts is a required non-null array",
		},
		"null": {
			mutateExample(func(resource map[string]any) { resource["conflicts"] = nil }),
			"conflicts is a required non-null array",
		},
		"self": {
			mutateExample(func(resource map[string]any) { resource["conflicts"] = []string{"skill:example"} }),
			`resource "skill:example" must not conflict with itself`,
		},
		"missing target": {
			mutateExample(func(resource map[string]any) { resource["conflicts"] = []string{"asset:missing"} }),
			`conflict "asset:missing" does not exist`,
		},
		"asymmetric": {
			manifestWith([]map[string]any{
				resource("asset", "alpha", nil, []string{"asset:beta"}),
				resource("asset", "beta", nil, []string{}),
			}),
			`between "asset:alpha" and "asset:beta" must be symmetric`,
		},
		"direct dependency": {
			manifestWith([]map[string]any{
				resource("asset", "alpha", []string{"asset:beta"}, []string{"asset:beta"}),
				resource("asset", "beta", nil, []string{"asset:alpha"}),
			}),
			`"asset:alpha" must not conflict with mandatory dependency "asset:beta"`,
		},
		"transitive dependency": {
			manifestWith([]map[string]any{
				resource("asset", "alpha", []string{"asset:beta"}, []string{"asset:charlie"}),
				resource("asset", "beta", []string{"asset:charlie"}, []string{}),
				resource("asset", "charlie", nil, []string{"asset:alpha"}),
			}),
			`"asset:alpha" must not conflict with mandatory dependency "asset:charlie"`,
		},
		"sibling dependencies": {
			manifestWith([]map[string]any{
				resource("asset", "alpha", nil, []string{"asset:beta"}),
				resource("asset", "beta", nil, []string{"asset:alpha"}),
				resource("asset", "root", []string{"asset:alpha", "asset:beta"}, nil),
			}),
			`resource "asset:root" mandatory dependency closure contains conflicting resources "asset:alpha" and "asset:beta"`,
		},
		"unsorted": {
			manifestWith([]map[string]any{
				resource("asset", "alpha", nil, []string{"asset:charlie", "asset:beta"}),
				resource("asset", "beta", nil, []string{"asset:alpha"}),
				resource("asset", "charlie", nil, []string{"asset:alpha"}),
			}),
			"conflicts must be a sorted set",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadPortableManifest(writeManifestV4(t, test.manifest), t.TempDir()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadPortableManifestV4ForbidsNoticeConflictsAndV3StillRejectsField(t *testing.T) {
	noticeV4 := `{"kind":"notice","id":"mit","source":"NOTICE","license":"MIT","attribution":"Example","requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]}`
	if _, err := decodeResource([]byte(noticeV4), manifestSchemaV4); err == nil || !strings.Contains(err.Error(), "forbidden for notice resources") {
		t.Fatalf("v4 notice conflicts error = %v", err)
	}
	resourceV3 := []byte(`{"kind":"instruction","id":"guide","source":"guide.md","requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]}`)
	if _, err := decodeResource(resourceV3, manifestSchemaV3); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("v3 conflicts must remain unknown, got %v", err)
	}
}

func TestRuntimeEvidenceIsTriStateDeterministicAndSecretSafe(t *testing.T) {
	valid := `{"requirements":[{"kind":"authentication","id":"vercel","state":"unavailable","reason":"not_found","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1","redacted_identity":"vercel-user"},{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}],"authorities":[{"kind":"network","scope":"vercel_project","state":"unverified","reason":"stale","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}]}`
	if _, err := DecodeRuntimeEvidence([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	secretAuthority := `{"requirements":[],"authorities":[{"kind":"secret_use","scope":"vercel_account","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1","redacted_identity":"vercel-token"}]}`
	if _, err := DecodeRuntimeEvidence([]byte(secretAuthority)); err != nil {
		t.Fatalf("sanitized secret authority identity must be admissible: %v", err)
	}
	for name, encoded := range map[string]string{
		"unknown state":             strings.Replace(valid, `"available"`, `"maybe"`, 1),
		"unknown reason":            strings.Replace(valid, `"verified"`, `"present"`, 1),
		"bad time":                  strings.Replace(valid, `"2026-07-25T12:00:00Z"`, `"today"`, 1),
		"sensitive field":           strings.Replace(valid, `"observer_revision":"observer-v1"`, `"observer_revision":"observer-v1","token":"secret"`, 1),
		"fingerprint field":         strings.Replace(valid, `"observer_revision":"observer-v1"`, `"observer_revision":"observer-v1","fingerprint":"recoverable"`, 1),
		"legacy identity":           strings.Replace(valid, `"redacted_identity":"vercel-user"`, `"identity":"vercel-user"`, 1),
		"invalid redacted identity": strings.Replace(valid, `"redacted_identity":"vercel-user"`, `"redacted_identity":"Vercel User"`, 1),
		"duplicate fact":            strings.Replace(valid, `],"authorities"`, `,{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}],"authorities"`, 1),
		"unsorted requirements":     strings.Replace(valid, `{"kind":"authentication","id":"vercel","state":"unavailable","reason":"not_found","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1","redacted_identity":"vercel-user"},{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}`, `{"kind":"tool","id":"node","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"},{"kind":"authentication","id":"vercel","state":"unavailable","reason":"not_found","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1","redacted_identity":"vercel-user"}`, 1),
		"unsorted authorities":      strings.Replace(valid, `{"kind":"network","scope":"vercel_project","state":"unverified","reason":"stale","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}`, `{"kind":"network","scope":"vercel_project","state":"unverified","reason":"stale","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"},{"kind":"filesystem_read","scope":"consumer_project","state":"available","reason":"verified","observed_at":"2026-07-25T12:00:00Z","observer_revision":"observer-v1"}`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRuntimeEvidence([]byte(encoded)); err == nil {
				t.Fatal("expected evidence rejection")
			}
		})
	}
}
