package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

func TestCheckedInCatalogDerivesCurrentAddyManifest(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := Discover(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := catalog.ShowDetail("addy")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Pack.Version != "1.0.0" || detail.Pack.Description != "Addy agent skills" || !detail.Pack.Selectable ||
		detail.Pack.manifestVersion != manifestSchemaV4 ||
		len(detail.Pack.Surfaces) != 3 || detail.Pack.Surfaces[0] != SurfaceClaude ||
		detail.Pack.Surfaces[1] != SurfaceCodex || detail.Pack.Surfaces[2] != SurfaceOpenCode ||
		len(detail.HistoricalVersions) != 0 || len(detail.UpdateRoutes) != 0 {
		t.Fatalf("Addy current contract = %#v", detail)
	}
}

func TestCheckedInEngramTwoPublishesExactThreeSurfaceContract(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := Discover(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show("engram")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "1.0.0" || len(pack.Surfaces) != 3 || pack.Surfaces[0] != SurfaceClaude {
		t.Fatalf("Engram catalog contract = %#v", pack)
	}
	contract := LifecycleContractFor(pack, SurfaceClaude, nil)
	if contract.Compatibility != CompatibilityDegraded || len(contract.Exclusions) != 1 || contract.Exclusions[0].Code != "generic-lifecycle-unsupported" {
		t.Fatalf("Engram Claude lifecycle contract = %#v", contract)
	}
	if got := pack.Resources[1].SurfaceExclusions; len(got) != 1 || got[0].Surface != SurfaceClaude || got[0].Mode != "optional" || got[0].Code != "generic-lifecycle-unsupported" {
		t.Fatalf("Engram Claude lifecycle outcome = %#v", got)
	}
	mcp := pack.Resources[2]
	if mcp.Command != "engram" || strings.Join(mcp.Args, "\x00") != "mcp\x00--tools=agent" || len(mcp.Bindings) != 3 {
		t.Fatalf("Engram MCP contract = %#v", mcp)
	}
}

func TestDiscoverWaitsForCompleteBundleTransaction(t *testing.T) {
	bundle := writeCatalogFixture(t)
	repository := filepath.Dir(bundle)
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Discover(bundle)
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

type blockingBundleAdapter struct {
	bundleRoot string
	entered    chan string
	release    chan struct{}
}

func (a *blockingBundleAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	data, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.FromSlash(transition.Desired.Resources[0].Source)))
	if err != nil {
		return SurfaceInspection{}, err
	}
	a.entered <- string(data)
	<-a.release
	return SurfaceInspection{}, nil
}

func (*blockingBundleAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
}

func TestPreviewHoldsBundleTransactionThroughAdapterReads(t *testing.T) {
	bundle := writeCatalogFixture(t)
	catalog, err := Discover(bundle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &blockingBundleAdapter{bundleRoot: bundle, entered: make(chan string, 1), release: make(chan struct{})}
	facade := NewFacade(catalog, WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	previewDone := make(chan error, 1)
	go func() {
		_, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
		previewDone <- err
	}()
	select {
	case got := <-adapter.entered:
		if got != "matty" {
			t.Fatalf("adapter read %q, want one complete old bundle generation", got)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter did not inspect the bundle")
	}

	transaction := make(chan *bundletransaction.Guard, 1)
	transactionErr := make(chan error, 1)
	go func() {
		guard, err := bundletransaction.Acquire(context.Background(), filepath.Dir(bundle))
		if err != nil {
			transactionErr <- err
			return
		}
		transaction <- guard
	}()
	select {
	case guard := <-transaction:
		guard.Release()
		t.Fatal("bundle transaction started before the adapter completed its observation")
	case err := <-transactionErr:
		t.Fatal(err)
	case <-time.After(40 * time.Millisecond):
	}

	close(adapter.release)
	if err := <-previewDone; err != nil {
		t.Fatal(err)
	}
	select {
	case guard := <-transaction:
		if err := guard.Release(); err != nil {
			t.Fatal(err)
		}
	case err := <-transactionErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("bundle transaction did not resume after the complete observation")
	}
}

func TestDeferredCatalogRefreshesAfterBundleSwap(t *testing.T) {
	bundle := writeCatalogFixture(t)
	catalog, err := DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Dir(bundle)
	stage := filepath.Join(repository, "bundle-stage")
	for _, path := range []string{
		"instructions/addy.md",
		"instructions/argote-engineering.md",
		"instructions/argote-spanish.md",
		"instructions/engram.md",
		"instructions/matty.md",
		"skills/espera-que/SKILL.md",
		"packs/addy/pack.json",
		"packs/argote/pack.json",
		"packs/engram/pack.json",
		"packs/matty/pack.json",
	} {
		data, err := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if path == "packs/matty/pack.json" {
			data = []byte(strings.Replace(string(data), `"version":"1.0.0"`, `"version":"2.0.0"`, 1))
		}
		target := filepath.Join(stage, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(repository, "bundle-backup")
	if err := os.Rename(bundle, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, bundle); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}

	pack, err := catalog.Show("matty")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "2.0.0" {
		t.Fatalf("Show version=%s, want complete new generation 2.0.0", pack.Version)
	}
	packs, err := catalog.ListCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if packs[3].ID != "matty" || packs[3].Version != "2.0.0" {
		t.Fatalf("ListCurrent packs=%+v, want complete new generation", packs)
	}
}

func TestDiscoverLoadsInitialStrictCatalog(t *testing.T) {
	bundleRoot := writeCatalogFixture(t)
	catalog, err := Discover(bundleRoot)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	packs := catalog.List()
	if len(packs) != 4 || packs[0].ID != "addy" || packs[1].ID != "argote" || packs[2].ID != "engram" || packs[3].ID != "matty" {
		t.Fatalf("packs = %#v", packs)
	}
	argote, err := catalog.Show("argote")
	if err != nil {
		t.Fatal(err)
	}
	if got := argote.ResourceCounts(); got != (ResourceCounts{Skills: 1, Instructions: 2}) {
		t.Fatalf("argote counts = %#v", got)
	}
	engram, err := catalog.Show("engram")
	if err != nil {
		t.Fatal(err)
	}
	if got := engram.ResourceCounts(); got != (ResourceCounts{Instructions: 1, MCPServers: 1, Lifecycles: 1}) {
		t.Fatalf("counts = %#v", got)
	}
	if strings.Join(engram.Requires.Tools, ",") != "engram" {
		t.Fatalf("tools = %v", engram.Requires.Tools)
	}
	if _, err := catalog.Show("web"); err == nil || !strings.Contains(err.Error(), "pack list") {
		t.Fatalf("unknown error = %v", err)
	}
}

func TestDiscoverRejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"unknown field", func(m map[string]any) { m["host_config"] = true }, "unknown field"},
		{"invalid id", func(m map[string]any) { m["id"] = "Engram" }, "lowercase kebab-case"},
		{"invalid version", func(m map[string]any) { m["version"] = "1" }, "SemVer"},
		{"invalid prerelease version", func(m map[string]any) { m["version"] = "1.0.0-01" }, "SemVer"},
		{"invalid external requirement", func(m map[string]any) { m["external_requirements"] = []any{"not a tool"} }, "external_requirements"},
		{"unknown resource", func(m map[string]any) {
			m["resources"] = []any{map[string]any{"kind": "config", "id": "bad", "requires": []any{}, "conflicts": []any{}, "bindings": []any{}, "surface_exclusions": []any{}}}
		}, "unsupported resource kind"},
		{"duplicate resource", func(m map[string]any) { r := m["resources"].([]any); m["resources"] = append(r, r[0]) }, "duplicate resource"},
		{"traversing source", func(m map[string]any) {
			m["resources"] = []any{map[string]any{"kind": "instruction", "id": "bad-source", "source": "../outside", "requires": []any{}, "conflicts": []any{}, "bindings": []any{}, "surface_exclusions": []any{}}}
		}, "escapes the bundle root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCatalogFixture(t)
			path := filepath.Join(root, "packs", "engram", "pack.json")
			data, _ := os.ReadFile(path)
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			tt.mutate(manifest)
			encoded, _ := json.Marshal(manifest)
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Discover(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateSurfacesRejectsUnsupportedSurface(t *testing.T) {
	root := writeCatalogFixture(t)
	path := filepath.Join(root, "packs", "addy", "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"surfaces":["claude","codex","opencode"]`, `"surfaces":["codex","mobile"]`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("error = %v", err)
	}
}

func writeCatalogFixture(t *testing.T) string {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle")
	skillRoot := filepath.Join(bundle, "skills")
	instructionRoot := filepath.Join(bundle, "instructions")
	for _, dir := range []string{skillRoot, instructionRoot, filepath.Join(skillRoot, "espera-que"), filepath.Join(bundle, "packs", "addy"), filepath.Join(bundle, "packs", "argote"), filepath.Join(bundle, "packs", "engram"), filepath.Join(bundle, "packs", "matty")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "engram.md"), []byte("engram"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "addy.md"), []byte("addy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "matty.md"), []byte("matty"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "argote-engineering.md"), []byte("engineering"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "argote-spanish.md"), []byte("spanish"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "espera-que", "SKILL.md"), []byte("---\nname: espera-que\n---\n\nRepitch.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	addy := currentCatalogFixtureManifest("addy", "Addy", "instructions/addy.md", []string{})
	argote := `{"id":"argote","version":"1.0.0","description":"Argote","selectable":true,"surfaces":["claude","codex","opencode"],"external_requirements":[],"resources":[{"kind":"instruction","id":"engineering-principles","source":"instructions/argote-engineering.md","requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"instruction","name":"engineering-principles","invocation":"engineering-principles","mode":"native","sharing":"shared"},{"surface":"codex","projection":"instruction","name":"engineering-principles","invocation":"engineering-principles","mode":"native","sharing":"shared"},{"surface":"opencode","projection":"instruction","name":"engineering-principles","invocation":"engineering-principles","mode":"native","sharing":"shared"}],"surface_exclusions":[]},{"kind":"instruction","id":"neutral-spanish","source":"instructions/argote-spanish.md","requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"instruction","name":"neutral-spanish","invocation":"neutral-spanish","mode":"native","sharing":"shared"},{"surface":"codex","projection":"instruction","name":"neutral-spanish","invocation":"neutral-spanish","mode":"native","sharing":"shared"},{"surface":"opencode","projection":"instruction","name":"neutral-spanish","invocation":"neutral-spanish","mode":"native","sharing":"shared"}],"surface_exclusions":[]},{"kind":"skill","id":"espera-que","source":"skills/espera-que","requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"skill","name":"espera-que","invocation":"/espera-que","mode":"native","sharing":"exclusive"},{"surface":"codex","projection":"skill","name":"espera-que","invocation":"$espera-que","mode":"native","sharing":"exclusive"},{"surface":"opencode","projection":"skill","name":"espera-que","invocation":"espera-que","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}],"exclusions":[]}`
	engram := `{"id":"engram","version":"1.0.0","description":"Engram","selectable":true,"surfaces":["claude","codex","opencode"],"external_requirements":["engram"],"resources":[{"kind":"instruction","id":"engram-memory","source":"instructions/engram.md","requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"instruction","name":"engram-memory","invocation":"engram-memory","mode":"native","sharing":"shared"},{"surface":"codex","projection":"instruction","name":"engram-memory","invocation":"engram-memory","mode":"native","sharing":"shared"},{"surface":"opencode","projection":"instruction","name":"engram-memory","invocation":"engram-memory","mode":"native","sharing":"shared"}],"surface_exclusions":[]},{"kind":"lifecycle","id":"engram-memory","requires":[],"conflicts":[],"bindings":[{"surface":"codex","projection":"lifecycle","name":"engram-memory","invocation":"engram-memory","mode":"native","sharing":"exclusive"},{"surface":"opencode","projection":"lifecycle","name":"engram-memory","invocation":"engram-memory","mode":"native","sharing":"exclusive"}],"surface_exclusions":[{"surface":"claude","mode":"optional","code":"generic-lifecycle-unsupported","reason":"unsupported"}]},{"kind":"mcp_server","id":"engram","command":"engram","args":["mcp"],"requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"mcp_server","name":"engram","invocation":"engram","mode":"native","sharing":"exclusive"},{"surface":"codex","projection":"mcp_server","name":"engram","invocation":"engram","mode":"native","sharing":"exclusive"},{"surface":"opencode","projection":"mcp_server","name":"engram","invocation":"engram","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}],"exclusions":[]}`
	matty := currentCatalogFixtureManifest("matty", "Matty", "instructions/matty.md", []string{})
	for name, data := range map[string]string{"addy": addy, "argote": argote, "engram": engram, "matty": matty} {
		if err := os.WriteFile(filepath.Join(bundle, "packs", name, "pack.json"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return bundle
}

func currentCatalogFixtureManifest(id, description, source string, requirements []string) string {
	external, _ := json.Marshal(requirements)
	return `{"id":"` + id + `","version":"1.0.0","description":"` + description + `","selectable":true,"surfaces":["claude","codex","opencode"],"external_requirements":` + string(external) + `,"resources":[{"kind":"instruction","id":"` + id + `-guidance","source":"` + source + `","requires":[],"conflicts":[],"bindings":[{"surface":"claude","projection":"instruction","name":"` + id + `-guidance","invocation":"` + id + `-guidance","mode":"native","sharing":"shared"},{"surface":"codex","projection":"instruction","name":"` + id + `-guidance","invocation":"` + id + `-guidance","mode":"native","sharing":"shared"},{"surface":"opencode","projection":"instruction","name":"` + id + `-guidance","invocation":"` + id + `-guidance","mode":"native","sharing":"shared"}],"surface_exclusions":[]}],"exclusions":[]}`
}
