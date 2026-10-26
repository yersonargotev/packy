package capabilitypack_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue877GlobalReceiptRejectsClientWithoutNativeRole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	receipt := []byte(`{"schema_version":1,"revision":1,"receipts":[{"pack":{"id":"shared","version":"1.0.0"},"surface":"agents","selection":{"mode":"all","roots":[],"client":"codex"},"resources":[{"kind":"skill","id":"guide"}],"projections":[]}]}`)
	if err := os.WriteFile(path, receipt, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := capabilitypack.NewFileActivationStore(path).LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents)
	if err == nil || !strings.Contains(err.Error(), "client selection requires an installed native role") {
		t.Fatalf("receipt assigned an unselected runtime client: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(receipt) {
		t.Fatalf("rejected receipt changed: %q %v", got, err)
	}
}
