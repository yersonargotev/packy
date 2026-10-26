package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIssue876ControlledCheckRejectsRetiredIdentityBeforeMutation(t *testing.T) {
	for _, scope := range []ControlledCheckScope{ControlledCheckGlobal, ControlledCheckProject} {
		t.Run(string(scope), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "state")
			store := NewFileControlledCheckStore(home)
			identity := issue876ControlledCheckIdentity(scope)
			if _, err := store.Status(context.Background(), identity); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
				t.Fatalf("retired status identity: %v", err)
			}
			if _, err := store.Record(context.Background(), identity, ReadinessTrue, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
				t.Fatalf("retired recorded identity: %v", err)
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatalf("retired check created state or lock directory: %v", err)
			}
		})
	}
}

func TestIssue876ControlledCheckRejectsRetiredPersistedEvidence(t *testing.T) {
	home := t.TempDir()
	identity := issue876ControlledCheckIdentity(ControlledCheckGlobal)
	evidence := controlledCheckEvidence{
		Identity: identity, ValidityIdentity: controlledCheckValidity(identity), Result: ReadinessTrue, ObservedAt: "2026-10-25T00:00:00Z",
	}
	data, err := json.Marshal(controlledCheckDocument{SchemaVersion: controlledCheckSchemaVersion, Entries: []controlledCheckEvidence{evidence}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "controlled-checks.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileControlledCheckStore(home)
	if _, err := store.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("retired persisted evidence: %v", err)
	}
	identity.Surface = SurfaceAgents
	if _, err := store.Status(context.Background(), identity); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("current surface silently ignored retired evidence: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatalf("read-only rejection modified persisted evidence: %v", err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != "controlled-checks.json" {
		t.Fatalf("read-only rejection created state or lock files: %v, %v", entries, err)
	}
}

func issue876ControlledCheckIdentity(scope ControlledCheckScope) ControlledCheckIdentity {
	identity := ControlledCheckIdentity{
		Pack: "reviewed", PackVersion: "1.0.0", Scope: scope, Surface: Surface("opencode"), Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}},
		ProjectionRevision: "projection-v1", AdapterVersion: "retired/v1", HostVersion: "1.0.0",
	}
	if scope == ControlledCheckProject {
		identity.ProjectDigest = strings.Repeat("a", 64)
	}
	return identity
}
