package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue520LegacyGlobalStateIsUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	legacy := `{"schema_version":5,"revision":1,"activations":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewFileActivationStore(path).LoadSnapshot(context.Background(), SurfaceCodex)
	if err == nil || !strings.Contains(err.Error(), "unsupported legacy capability-pack state") {
		t.Fatalf("legacy state error = %v", err)
	}
}

func TestIssue520GlobalReceiptPersistsOnlyCurrentOwnershipFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	store := NewFileActivationStore(path)
	explicit := true
	state := ActivationState{
		Intent:    ActivationIntent{PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}}, Explicit: &explicit},
		Ownership: []ProjectionOwnership{{ID: "path:/tmp/guide", ProjectionID: "skill:guide", Target: "/tmp/guide", Fingerprint: "digest", PackID: "app", Surface: SurfaceCodex}},
	}
	state.Intents = []ActivationIntent{state.Intent}
	if _, err := store.SaveSnapshot(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	projection := document["receipts"].([]any)[0].(map[string]any)["projections"].([]any)[0].(map[string]any)
	for _, retired := range []string{"physical_id", "contributors", "adapter_provenance", "authorities", "mode", "file_mode", "command", "args", "discoverable_by"} {
		if _, exists := projection[retired]; exists {
			t.Fatalf("installed receipt retained %q: %s", retired, data)
		}
	}
}
